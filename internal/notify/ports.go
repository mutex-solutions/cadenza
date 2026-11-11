// Package notify decides when the owner is told something (plan D14), sends
// it through every registered device's provider, and logs each send.
package notify

import (
	"context"
	"encoding/json"
)

// Message is what a notification shows. Title is
// "<agent> · <machine> — <agent's title>"; nothing from the work itself.
type Message struct {
	Title    string            `json:"title"`
	Body     string            `json:"body"`
	ThreadID string            `json:"thread_id,omitempty"`
	URL      string            `json:"url,omitempty"` // path in the app, e.g. /t/t_81
	Data     map[string]string `json:"data,omitempty"`
}

// Result of one send.
type Result struct {
	// Gone means the device token is no longer valid (unsubscribed,
	// uninstalled); the device should be disabled.
	Gone bool
	// ProviderID is the provider's id for the send, e.g. an Expo ticket id.
	ProviderID string
}

// Sender delivers to one provider. Implementations: adapter/webpush,
// adapter/expopush, adapter/fcm. Token is the provider-specific device token
// as registered (spec.DeviceInput.Token).
type Sender interface {
	Provider() string // "webpush", "expo", "fcm"
	Send(ctx context.Context, token json.RawMessage, msg Message) (Result, error)
}
