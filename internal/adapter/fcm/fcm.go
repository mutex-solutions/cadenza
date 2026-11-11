// Package fcm sends notifications through Firebase Cloud Messaging HTTP v1
// (provider "fcm") with the self-hoster's own service account. It uses only
// the standard library: it signs the OAuth2 JWT itself, exchanges it for an
// access token and caches that token until it expires.
package fcm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/notify"
)

// DefaultEndpoint is the FCM API base URL.
const DefaultEndpoint = "https://fcm.googleapis.com"

// Provider is the device provider name this sender handles.
const Provider = "fcm"

var _ notify.Sender = (*Sender)(nil)

// Config holds the settings for a Sender.
type Config struct {
	// ServiceAccountJSON is the key file downloaded from the Firebase console.
	ServiceAccountJSON []byte
	// Endpoint replaces DefaultEndpoint, e.g. an httptest server.
	Endpoint string
	// TokenURL replaces the service account's token_uri, e.g. an httptest
	// server. The JWT audience stays token_uri.
	TokenURL string
	// HTTPClient defaults to a client with a 15 second timeout.
	HTTPClient *http.Client
	// Clock defaults to kernel.SystemClock.
	Clock kernel.Clock
}

// Sender is a notify.Sender for FCM registration tokens.
type Sender struct {
	project  string
	endpoint string
	client   *http.Client
	tokens   *tokenSource
}

// New parses the service account and returns a Sender. It makes no requests.
func New(cfg Config) (*Sender, error) {
	sa, key, err := parseServiceAccount(cfg.ServiceAccountJSON)
	if err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	clock := cfg.Clock
	if clock == nil {
		clock = kernel.SystemClock{}
	}
	tokenURL := cfg.TokenURL
	if tokenURL == "" {
		tokenURL = sa.TokenURI
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Sender{
		project:  sa.ProjectID,
		endpoint: endpoint,
		client:   client,
		tokens:   &tokenSource{sa: sa, key: key, tokenURL: tokenURL, client: client, clock: clock},
	}, nil
}

// Provider returns "fcm".
func (s *Sender) Provider() string { return Provider }

type message struct {
	Token        string            `json:"token"`
	Notification notification      `json:"notification"`
	Data         map[string]string `json:"data,omitempty"`
	Android      android           `json:"android"`
}

type notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type android struct {
	Priority string `json:"priority"`
}

// apiError is FCM's error body.
type apiError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
		Details []struct {
			Type      string `json:"@type"`
			ErrorCode string `json:"errorCode"`
		} `json:"details"`
	} `json:"error"`
}

// unregistered reports whether the error says the token is no longer valid.
func (e apiError) unregistered() bool {
	for _, d := range e.Error.Details {
		if d.ErrorCode == "UNREGISTERED" {
			return true
		}
	}
	return e.Error.Status == "UNREGISTERED"
}

// Send delivers msg to one FCM registration token, given as a JSON string.
// A 404 or UNREGISTERED answer returns Result{Gone: true} and no error.
func (s *Sender) Send(ctx context.Context, token json.RawMessage, msg notify.Message) (notify.Result, error) {
	var tok string
	if err := json.Unmarshal(token, &tok); err != nil || tok == "" {
		return notify.Result{}, kernel.Invalid("fcm token is not a non-empty JSON string").WithFix("send the FCM registration token as a JSON string")
	}
	body, err := json.Marshal(map[string]message{"message": {
		Token:        tok,
		Notification: notification{Title: msg.Title, Body: msg.Body},
		Data:         messageData(msg),
		Android:      android{Priority: "high"},
	}})
	if err != nil {
		return notify.Result{}, kernel.Internal(err, "encode fcm message")
	}
	access, err := s.tokens.Token(ctx)
	if err != nil {
		return notify.Result{}, err
	}
	u := s.endpoint + "/v1/projects/" + url.PathEscape(s.project) + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return notify.Result{}, kernel.Internal(err, "build fcm request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+access)
	resp, err := s.client.Do(req)
	if err != nil {
		e := kernel.Unavailable("fcm is unreachable")
		e.Err = err
		return notify.Result{}, e
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusOK {
		var ok struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(data, &ok)
		return notify.Result{ProviderID: ok.Name}, nil
	}
	var ae apiError
	_ = json.Unmarshal(data, &ae)
	if resp.StatusCode == http.StatusNotFound || ae.unregistered() {
		return notify.Result{Gone: true}, nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		s.tokens.Invalidate()
	}
	if ae.Error.Status != "" {
		return notify.Result{}, kernel.Unavailable("fcm returned %d %s: %s", resp.StatusCode, ae.Error.Status, ae.Error.Message)
	}
	return notify.Result{}, kernel.Unavailable("fcm returned %d: %s", resp.StatusCode, truncate(data))
}

// messageData is the data map the app receives: msg.Data plus thread_id and url.
func messageData(msg notify.Message) map[string]string {
	d := make(map[string]string, len(msg.Data)+2)
	for k, v := range msg.Data {
		d[k] = v
	}
	if msg.ThreadID != "" {
		d["thread_id"] = msg.ThreadID
	}
	if msg.URL != "" {
		d["url"] = msg.URL
	}
	if len(d) == 0 {
		return nil
	}
	return d
}

func truncate(b []byte) string {
	if len(b) > 200 {
		b = b[:200]
	}
	return string(b)
}
