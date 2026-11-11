// Package webpush sends notifications to browsers with the Web Push
// protocol, signed with the server's VAPID key. It implements notify.Sender
// for the "webpush" provider.
package webpush

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	wp "github.com/SherClockHolmes/webpush-go"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/notify"
)

// Provider is the provider name devices register with.
const Provider = "webpush"

// DefaultTTL is how long a push service keeps an undelivered message.
const DefaultTTL = 24 * time.Hour

var _ notify.Sender = (*Sender)(nil)

// Config holds the server's VAPID identity.
type Config struct {
	// VAPIDPublicKey and VAPIDPrivateKey are base64url keys, as made by
	// GenerateVAPIDKeys. The public key is also given to browsers.
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	// Subscriber is a contact for push services: "mailto:you@example.com"
	// or an https URL. A bare e-mail address is also accepted.
	Subscriber string
	// TTL is how long push services keep a message for an offline browser.
	// Zero means DefaultTTL.
	TTL time.Duration
	// HTTPClient sends the requests. Nil means a client with a 30s timeout.
	HTTPClient *http.Client
}

// Sender delivers notify.Messages to browser push subscriptions.
type Sender struct {
	cfg Config
}

// New checks cfg and returns a Sender.
func New(cfg Config) (*Sender, error) {
	if cfg.VAPIDPublicKey == "" || cfg.VAPIDPrivateKey == "" {
		return nil, kernel.Invalid("web push needs VAPID keys").WithFix("generate them with webpush.GenerateVAPIDKeys")
	}
	if cfg.Subscriber == "" {
		return nil, kernel.Invalid("web push needs a subscriber contact").WithFix("set it to mailto:you@example.com")
	}
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultTTL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Sender{cfg: cfg}, nil
}

// GenerateVAPIDKeys returns a new base64url VAPID key pair. The server makes
// one on first start and keeps it; changing it breaks every subscription.
func GenerateVAPIDKeys() (publicKey, privateKey string, err error) {
	privateKey, publicKey, err = wp.GenerateVAPIDKeys()
	if err != nil {
		return "", "", kernel.Internal(err, "could not generate VAPID keys")
	}
	return publicKey, privateKey, nil
}

// Provider returns "webpush".
func (s *Sender) Provider() string { return Provider }

// Send encrypts msg as JSON and posts it to the subscription's endpoint.
// token is the browser's PushSubscription JSON:
// {"endpoint": "...", "keys": {"p256dh": "...", "auth": "..."}}.
// A 404 or 410 from the push service means the subscription is gone.
func (s *Sender) Send(ctx context.Context, token json.RawMessage, msg notify.Message) (notify.Result, error) {
	sub, err := parseToken(token)
	if err != nil {
		return notify.Result{}, err
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return notify.Result{}, kernel.Internal(err, "could not encode push message")
	}
	resp, err := wp.SendNotificationWithContext(ctx, payload, sub, &wp.Options{
		HTTPClient:      s.cfg.HTTPClient,
		Subscriber:      strings.TrimPrefix(s.cfg.Subscriber, "mailto:"),
		TTL:             int(s.cfg.TTL / time.Second),
		Urgency:         wp.UrgencyHigh,
		VAPIDPublicKey:  s.cfg.VAPIDPublicKey,
		VAPIDPrivateKey: s.cfg.VAPIDPrivateKey,
	})
	if err != nil {
		e := kernel.Unavailable("web push send failed")
		e.Err = err
		return notify.Result{}, e
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return notify.Result{Gone: true}, nil
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return notify.Result{ProviderID: resp.Header.Get("Location")}, nil
	default:
		e := kernel.Unavailable("web push service answered %d", resp.StatusCode)
		e.Err = fmt.Errorf("%s", strings.TrimSpace(string(body)))
		return notify.Result{}, e
	}
}

// parseToken decodes and checks a PushSubscription. The endpoint must be
// https so the server only posts to real push services over TLS.
func parseToken(token json.RawMessage) (*wp.Subscription, error) {
	var sub wp.Subscription
	if err := json.Unmarshal(token, &sub); err != nil {
		return nil, kernel.Invalid("web push token is not a PushSubscription JSON object").
			WithFix(`send {"endpoint":"https://...","keys":{"p256dh":"...","auth":"..."}}`)
	}
	u, err := url.Parse(sub.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, kernel.Invalid("web push endpoint %q is not an https URL", sub.Endpoint)
	}
	if sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		return nil, kernel.Invalid("web push token is missing keys.p256dh or keys.auth")
	}
	return &sub, nil
}
