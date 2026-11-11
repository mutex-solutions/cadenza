package diskblob

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/media"
)

// Handler serves signed links: PUT stores a file, GET and HEAD read one
// (with Range support). Mount it at PathPrefix, e.g.
// mux.Handle(diskblob.PathPrefix, store.Handler()). It answers 403 for a bad
// or expired signature, 400 for bytes that do not match the link, 413 for
// too many bytes and 404 for a missing file.
func (s *Store) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dir, sha := path.Split(r.URL.Path)
		if !strings.HasSuffix(dir, PathPrefix) || !media.ValidSHA256(sha) {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodPut:
			s.put(w, r, sha)
		case http.MethodGet, http.MethodHead:
			s.get(w, r, sha)
		default:
			w.Header().Set("Allow", "GET, HEAD, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (s *Store) put(w http.ResponseWriter, r *http.Request, sha string) {
	l, err := s.parse(r, sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct != l.typ {
		http.Error(w, "Content-Type must be "+l.typ, http.StatusForbidden)
		return
	}
	if l.size > s.maxBytes || r.ContentLength > l.size {
		http.Error(w, "file is larger than the link allows", http.StatusRequestEntityTooLarge)
		return
	}
	if r.ContentLength >= 0 && r.ContentLength != l.size {
		http.Error(w, "Content-Length does not match the signed size", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(s.path(sha)); err == nil {
		w.WriteHeader(http.StatusOK) // content addressed: already stored
		return
	}
	body := http.MaxBytesReader(w, r.Body, l.size)
	code, msg := s.store(body, sha, l.size)
	if code != http.StatusOK {
		http.Error(w, msg, code)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// store streams body to a temp file while hashing it, then moves it into
// place only when the size and SHA-256 match.
func (s *Store) store(body io.Reader, sha string, size int64) (int, string) {
	tmp, err := os.CreateTemp(filepath.Join(s.dir, "tmp"), "put-*")
	if err != nil {
		return http.StatusInternalServerError, "cannot create temp file"
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), body)
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		tmp.Close()
		return http.StatusRequestEntityTooLarge, "file is larger than the link allows"
	case err != nil:
		tmp.Close()
		return http.StatusBadRequest, "reading upload failed"
	case n != size:
		tmp.Close()
		return http.StatusBadRequest, "upload size does not match the signed size"
	case hex.EncodeToString(h.Sum(nil)) != sha:
		tmp.Close()
		return http.StatusBadRequest, "upload SHA-256 does not match"
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return http.StatusInternalServerError, "writing file failed"
	}
	if err := tmp.Close(); err != nil {
		return http.StatusInternalServerError, "writing file failed"
	}
	final := s.path(sha)
	if err := os.MkdirAll(filepath.Dir(final), 0o750); err != nil {
		return http.StatusInternalServerError, "cannot create folder"
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return http.StatusInternalServerError, "storing file failed"
	}
	return http.StatusOK, ""
}

func (s *Store) get(w http.ResponseWriter, r *http.Request, sha string) {
	l, err := s.parse(r, sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	f, err := os.Open(s.path(sha))
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "cannot open file", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, "cannot read file", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", l.typ)
	h.Set("Content-Disposition", disposition(l.typ, l.name))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, max-age=86400, immutable")
	if l.typ == media.TypeHTML {
		// Pages are served from the server's own origin, so they must not
		// run with its cookies or storage: sandbox gives them an opaque origin.
		h.Set("Content-Security-Policy", "sandbox allow-scripts allow-popups")
	}
	http.ServeContent(w, r, "", fi.ModTime(), f)
}
