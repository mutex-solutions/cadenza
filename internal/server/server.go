package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mutex-solutions/cadenza/internal/access"
	"github.com/mutex-solutions/cadenza/internal/adapter/diskblob"
	"github.com/mutex-solutions/cadenza/internal/adapter/expopush"
	"github.com/mutex-solutions/cadenza/internal/adapter/fcm"
	"github.com/mutex-solutions/cadenza/internal/adapter/markdown"
	"github.com/mutex-solutions/cadenza/internal/adapter/s3"
	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore"
	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore/postgres"
	"github.com/mutex-solutions/cadenza/internal/adapter/sqlstore/sqlite"
	"github.com/mutex-solutions/cadenza/internal/adapter/webpush"
	"github.com/mutex-solutions/cadenza/internal/catalog"
	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/files"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/notify"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/server/httpapi"
	"github.com/mutex-solutions/cadenza/internal/server/mcphttp"
	"github.com/mutex-solutions/cadenza/internal/server/webapp"
)

// store is what both database engines provide.
type store interface {
	Access() access.Store
	Review() review.Store
	Files() files.Repo
	Catalog() catalog.Repo
	Notify() notify.Repo
	Events() eventlog.Reader
	Settings() sqlstore.Settings
	Ping(ctx context.Context) error
	Close() error
}

// Server is a running Cadenza.
type Server struct {
	cfg     Config
	log     *slog.Logger
	store   store
	bus     *eventlog.Bus
	notify  *notify.Service
	handler http.Handler
}

// New opens storage and builds every service. Close releases them.
func New(ctx context.Context, cfg Config, version string, log *slog.Logger) (*Server, error) {
	st, err := openStore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, log: log, store: st, bus: eventlog.NewBus()}
	if err := s.build(ctx, version); err != nil {
		st.Close()
		return nil, err
	}
	return s, nil
}

func openStore(ctx context.Context, cfg Config) (store, error) {
	if path, ok := strings.CutPrefix(cfg.Database, "sqlite:"); ok {
		if path != ":memory:" {
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return nil, fmt.Errorf("create database folder: %w", err)
			}
		}
		return sqlite.Open(ctx, path)
	}
	return postgres.Open(ctx, cfg.Database)
}

func (s *Server) build(ctx context.Context, version string) error {
	clock := kernel.SystemClock{}
	blobs, blobHandler, err := s.blobStore(ctx)
	if err != nil {
		return err
	}
	senders, vapidPublic, err := s.senders(ctx)
	if err != nil {
		return err
	}
	md := markdown.New()
	accessSvc := access.NewService(s.store.Access(), clock, s.bus)
	filesSvc := files.NewService(s.store.Files(), blobs, clock)
	catalogSvc := catalog.NewService(s.store.Catalog(), md, clock)
	reviewSvc := review.NewService(review.Deps{Store: s.store.Review(), Files: filesSvc, Templates: catalogSvc, Renderer: md,
		Clock: clock, Publisher: s.bus, BaseURL: s.cfg.PublicURL})
	s.notify = notify.NewService(s.store.Notify(), senders, reviewSvc, clock, s.log, vapidPublic)

	api := httpapi.New(httpapi.Deps{Access: accessSvc, Review: reviewSvc, Files: filesSvc, Catalog: catalogSvc, Notify: s.notify,
		Events: s.store.Events(), Bus: s.bus, Clock: clock, Log: s.log, Version: version, MinCLIVersion: s.cfg.MinCLIVersion,
		SecureCookies: s.cfg.Secure()})
	mcp := mcphttp.New(mcphttp.Deps{Access: accessSvc, Review: reviewSvc, Files: filesSvc, Catalog: catalogSvc, Version: version})

	mux := http.NewServeMux()
	mux.Handle("/v1/", api)
	mux.Handle("/mcp", mcp)
	if blobHandler != nil {
		mux.Handle(diskblob.PathPrefix, blobHandler)
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.store.Ping(r.Context()); err != nil {
			http.Error(w, "database unreachable", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok\n"))
	})
	mux.Handle("/", webapp.Handler())
	s.handler = s.secure(mux)
	return nil
}

// blobStore returns S3, or local disk in trial mode (with its HTTP handler).
func (s *Server) blobStore(ctx context.Context) (files.BlobStore, http.Handler, error) {
	if !s.cfg.Trial() {
		st, err := s3.New(s3.Config{Endpoint: s.cfg.S3.Endpoint, PublicEndpoint: s.cfg.S3.PublicEndpoint, Region: s.cfg.S3.Region,
			Bucket: s.cfg.S3.Bucket, AccessKey: s.cfg.S3.AccessKey, SecretKey: s.cfg.S3.SecretKey, ChecksumMode: s.cfg.S3.Checksum})
		if err != nil {
			return nil, nil, fmt.Errorf("object storage: %w", err)
		}
		return st, nil, nil
	}
	secret, err := s.secret(ctx, "blob_signing_secret")
	if err != nil {
		return nil, nil, err
	}
	key, _ := hex.DecodeString(secret)
	st, err := diskblob.New(diskblob.Config{Dir: filepath.Join(s.cfg.DataDir, "blobs"), BaseURL: s.cfg.PublicURL, Secret: key})
	if err != nil {
		return nil, nil, fmt.Errorf("local file storage: %w", err)
	}
	s.log.Info("trial mode: files are stored on this server's disk; set CADENZA_S3_BUCKET for real use", "dir", filepath.Join(s.cfg.DataDir, "blobs"))
	return st, st.Handler(), nil
}

// senders builds the push providers: Web Push always, Expo and FCM when the
// self-hoster configured their own credentials.
func (s *Server) senders(ctx context.Context) ([]notify.Sender, string, error) {
	pub, err := s.settingOr(ctx, "vapid_public", "")
	if err != nil {
		return nil, "", err
	}
	priv, err := s.settingOr(ctx, "vapid_private", "")
	if err != nil {
		return nil, "", err
	}
	if pub == "" || priv == "" {
		if pub, priv, err = webpush.GenerateVAPIDKeys(); err != nil {
			return nil, "", fmt.Errorf("generate Web Push keys: %w", err)
		}
		st := s.store.Settings()
		if err := st.SetSetting(ctx, "vapid_public", pub); err != nil {
			return nil, "", err
		}
		if err := st.SetSetting(ctx, "vapid_private", priv); err != nil {
			return nil, "", err
		}
	}
	wp, err := webpush.New(webpush.Config{VAPIDPublicKey: pub, VAPIDPrivateKey: priv, Subscriber: s.cfg.VAPIDSubject})
	if err != nil {
		return nil, "", fmt.Errorf("web push: %w", err)
	}
	senders := []notify.Sender{wp}
	if s.cfg.ExpoEnabled {
		senders = append(senders, expopush.New(expopush.Config{AccessToken: s.cfg.ExpoAccessToken}))
	}
	if s.cfg.FCMServiceAccount != "" {
		b, err := os.ReadFile(s.cfg.FCMServiceAccount)
		if err != nil {
			return nil, "", fmt.Errorf("read CADENZA_FCM_SERVICE_ACCOUNT_FILE: %w", err)
		}
		f, err := fcm.New(fcm.Config{ServiceAccountJSON: b})
		if err != nil {
			return nil, "", fmt.Errorf("fcm: %w", err)
		}
		senders = append(senders, f)
	}
	return senders, pub, nil
}

func (s *Server) settingOr(ctx context.Context, key, def string) (string, error) {
	v, err := s.store.Settings().GetSetting(ctx, key)
	if kernel.Is(err, kernel.KindNotFound) {
		return def, nil
	}
	return v, err
}

// secret returns a stored random secret, creating it on first start.
func (s *Server) secret(ctx context.Context, key string) (string, error) {
	v, err := s.settingOr(ctx, key, "")
	if err != nil || v != "" {
		return v, err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	v = hex.EncodeToString(b)
	return v, s.store.Settings().SetSetting(ctx, key, v)
}

// secure adds security headers to every response.
func (s *Server) secure(next http.Handler) http.Handler {
	csp := s.contentSecurityPolicy()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		if !strings.HasPrefix(r.URL.Path, diskblob.PathPrefix) {
			h.Set("Content-Security-Policy", csp)
		}
		if s.cfg.Secure() {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

// contentSecurityPolicy lets the app load files from object storage and run
// agent webpages only inside sandboxed frames.
func (s *Server) contentSecurityPolicy() string {
	storage := ""
	for _, e := range []string{s.cfg.S3.PublicEndpoint, s.cfg.S3.Endpoint} {
		if u, err := url.Parse(e); err == nil && u.Host != "" {
			storage += " " + u.Scheme + "://" + u.Host
			if s.cfg.S3.Bucket != "" {
				storage += " " + u.Scheme + "://*." + u.Host // virtual-host style buckets
			}
			break
		}
	}
	return "default-src 'self'; script-src 'self' 'unsafe-eval' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob:" + storage + "; media-src 'self' blob:" + storage + "; connect-src 'self'" + storage + "; " +
		"frame-src 'self' blob: data:; worker-src 'self' blob:; font-src 'self' data:; object-src 'none'; base-uri 'none'; " +
		"frame-ancestors 'none'; form-action 'self'"
}

// Handler is the server's HTTP handler (for tests).
func (s *Server) Handler() http.Handler { return s.handler }

// Run serves until ctx ends, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{Addr: s.cfg.Addr, Handler: s.handler, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: time.Minute, IdleTimeout: 2 * time.Minute}
	go s.notify.Run(ctx, s.bus)
	go s.pruneEvents(ctx)
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	s.log.Info("cadenza server listening", "addr", s.cfg.Addr, "public_url", s.cfg.PublicURL)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// pruneEvents deletes events older than the retention window once a day.
func (s *Server) pruneEvents(ctx context.Context) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		if n, err := s.store.Events().DeleteEventsBefore(ctx, time.Now().Add(-eventlog.Retention)); err != nil {
			s.log.Warn("pruning events failed", "error", err)
		} else if n > 0 {
			s.log.Info("pruned old events", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ResetPassword sets the owner password from the command line.
func (s *Server) ResetPassword(ctx context.Context, password string) error {
	return access.NewService(s.store.Access(), kernel.SystemClock{}, s.bus).ResetPassword(ctx, password)
}

// Close releases storage.
func (s *Server) Close() error { return s.store.Close() }
