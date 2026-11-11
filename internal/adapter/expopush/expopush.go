// Package expopush sends notifications through the Expo push service
// (provider "expo"). It is a small hand-written client for
// POST /--/api/v2/push/send and /--/api/v2/push/getReceipts.
package expopush

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/notify"
)

// DefaultBaseURL is Expo's public push service.
const DefaultBaseURL = "https://exp.host"

// Provider is the device provider name this sender handles.
const Provider = "expo"

var _ notify.Sender = (*Sender)(nil)

// tokenRe matches an Expo push token, e.g. ExponentPushToken[xxxxxxxx].
var tokenRe = regexp.MustCompile(`^(ExponentPushToken|ExpoPushToken)\[[A-Za-z0-9_\-]+\]$`)

// Config holds the settings for a Sender. Only the zero value is needed for
// real use; the rest is for self-hosters' own Expo tokens and for tests.
type Config struct {
	// BaseURL replaces DefaultBaseURL, e.g. an httptest server.
	BaseURL string
	// AccessToken is the self-hoster's own Expo access token, sent as a
	// Bearer token when set. Needed only if "enhanced push security" is on.
	AccessToken string
	// HTTPClient defaults to a client with a 15 second timeout.
	HTTPClient *http.Client
	// MaxRetries is how many times a 429, 5xx or network failure is retried.
	// Zero means 3; negative means no retries.
	MaxRetries int
	// Sleep waits between retries. Defaults to a context-aware timer.
	// Tests replace it to avoid real waiting.
	Sleep func(ctx context.Context, d time.Duration) error
}

// Sender is a notify.Sender for Expo push tokens.
type Sender struct {
	base    string
	token   string
	client  *http.Client
	retries int
	sleep   func(ctx context.Context, d time.Duration) error
}

// New returns a Sender with defaults filled in.
func New(cfg Config) *Sender {
	s := &Sender{
		base:    strings.TrimRight(cfg.BaseURL, "/"),
		token:   cfg.AccessToken,
		client:  cfg.HTTPClient,
		retries: cfg.MaxRetries,
		sleep:   cfg.Sleep,
	}
	if s.base == "" {
		s.base = DefaultBaseURL
	}
	if s.client == nil {
		s.client = &http.Client{Timeout: 15 * time.Second}
	}
	if s.retries == 0 {
		s.retries = 3
	} else if s.retries < 0 {
		s.retries = 0
	}
	if s.sleep == nil {
		s.sleep = sleepCtx
	}
	return s
}

// Provider returns "expo".
func (s *Sender) Provider() string { return Provider }

// pushMessage is one entry of the send request body.
type pushMessage struct {
	To        string            `json:"to"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Data      map[string]string `json:"data,omitempty"`
	Sound     string            `json:"sound"`
	Priority  string            `json:"priority"`
	ChannelID string            `json:"channelId"`
}

// ticket is Expo's answer for one message.
type ticket struct {
	Status  string  `json:"status"` // "ok" or "error"
	ID      string  `json:"id"`
	Message string  `json:"message"`
	Details details `json:"details"`
}

type details struct {
	Error string `json:"error"`
}

// ParseToken checks that raw is a JSON string holding an Expo push token and
// returns the token.
func ParseToken(raw json.RawMessage) (string, error) {
	var tok string
	if err := json.Unmarshal(raw, &tok); err != nil {
		return "", kernel.Invalid("expo token is not a JSON string").WithFix(`send the token as "ExponentPushToken[...]"`)
	}
	if !tokenRe.MatchString(tok) {
		return "", kernel.Invalid("expo token %q is not an Expo push token", tok).WithFix(`send the token as "ExponentPushToken[...]"`)
	}
	return tok, nil
}

// Send delivers msg to one Expo push token. A DeviceNotRegistered ticket
// returns Result{Gone: true} and no error.
func (s *Sender) Send(ctx context.Context, token json.RawMessage, msg notify.Message) (notify.Result, error) {
	tok, err := ParseToken(token)
	if err != nil {
		return notify.Result{}, err
	}
	body, err := json.Marshal([]pushMessage{{
		To:        tok,
		Title:     msg.Title,
		Body:      msg.Body,
		Data:      messageData(msg),
		Sound:     "default",
		Priority:  "high",
		ChannelID: "default",
	}})
	if err != nil {
		return notify.Result{}, kernel.Internal(err, "encode expo message")
	}
	var resp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := s.post(ctx, "/--/api/v2/push/send", body, &resp); err != nil {
		return notify.Result{}, err
	}
	if len(resp.Errors) > 0 {
		return notify.Result{}, kernel.Unavailable("expo rejected the request: %s %s", resp.Errors[0].Code, resp.Errors[0].Message)
	}
	t, err := firstTicket(resp.Data)
	if err != nil {
		return notify.Result{}, err
	}
	switch {
	case t.Status == "ok":
		return notify.Result{ProviderID: t.ID}, nil
	case t.Details.Error == "DeviceNotRegistered":
		return notify.Result{Gone: true}, nil
	default:
		return notify.Result{}, kernel.Unavailable("expo send failed: %s %s", t.Details.Error, t.Message)
	}
}

// firstTicket reads the ticket from data, which Expo sends as an array for
// array requests and as an object for single-message requests.
func firstTicket(data json.RawMessage) (ticket, error) {
	var list []ticket
	if err := json.Unmarshal(data, &list); err == nil {
		if len(list) == 0 {
			return ticket{}, kernel.Unavailable("expo returned no ticket")
		}
		return list[0], nil
	}
	var t ticket
	if err := json.Unmarshal(data, &t); err != nil || t.Status == "" {
		return ticket{}, kernel.Unavailable("expo returned an unreadable ticket")
	}
	return t, nil
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

// post sends body to path and decodes a 200 answer into out. It retries 429,
// 5xx and network failures with doubling backoff starting at one second.
func (s *Sender) post(ctx context.Context, path string, body []byte, out any) error {
	backoff := time.Second
	var lastErr error
	for attempt := 0; ; attempt++ {
		retry, err := s.postOnce(ctx, path, body, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry || attempt >= s.retries || ctx.Err() != nil {
			return lastErr
		}
		if err := s.sleep(ctx, backoff); err != nil {
			return kernel.Unavailable("expo push interrupted: %v", err)
		}
		backoff *= 2
	}
}

// postOnce makes one request. retry reports whether a failure is worth retrying.
func (s *Sender) postOnce(ctx context.Context, path string, body []byte, out any) (retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+path, bytes.NewReader(body))
	if err != nil {
		return false, kernel.Internal(err, "build expo request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		e := kernel.Unavailable("expo push is unreachable")
		e.Err = err
		return true, e
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		e := kernel.Unavailable("read expo response")
		e.Err = err
		return true, e
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return true, kernel.Unavailable("expo push returned %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		// Expo puts request-level errors in the body with a 4xx status.
		if json.Unmarshal(data, out) == nil && bytes.Contains(data, []byte(`"errors"`)) {
			return false, nil
		}
		return false, kernel.Unavailable("expo push returned %d: %s", resp.StatusCode, truncate(data))
	}
	if err := json.Unmarshal(data, out); err != nil {
		e := kernel.Unavailable("expo returned unreadable JSON")
		e.Err = err
		return false, e
	}
	return false, nil
}

func truncate(b []byte) string {
	if len(b) > 200 {
		b = b[:200]
	}
	return string(b)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait: %w", ctx.Err())
	}
}
