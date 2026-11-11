package notify

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Device is a phone or browser that receives notifications.
type Device struct {
	ID         string
	Owner      kernel.OwnerID
	Provider   string
	Token      json.RawMessage
	Name       string
	CreatedAt  time.Time
	LastOKAt   time.Time
	DisabledAt *time.Time
}

// Entry is one logged send (plan C7: every send is logged).
type Entry struct {
	ID        string
	Owner     kernel.OwnerID
	DeviceID  string
	Provider  string
	Title     string
	Body      string
	ThreadID  string
	Status    string // sent, failed, skipped
	Error     string
	CreatedAt time.Time
}

// Entry statuses.
const (
	StatusSent   = "sent"
	StatusFailed = "failed"
)

// Repo stores devices and the send log.
type Repo interface {
	CreateDevice(ctx context.Context, d *Device) error
	GetDevice(ctx context.Context, owner kernel.OwnerID, id string) (*Device, error)
	GetDeviceByToken(ctx context.Context, owner kernel.OwnerID, provider string, token json.RawMessage) (*Device, error)
	UpdateDevice(ctx context.Context, d *Device) error
	ListActiveDevices(ctx context.Context, owner kernel.OwnerID) ([]Device, error)
	CreateNotification(ctx context.Context, e *Entry) error
	ListNotifications(ctx context.Context, owner kernel.OwnerID, limit int) ([]Entry, error)
	// LastSentForThread is when a notification about the thread last went out.
	LastSentForThread(ctx context.Context, owner kernel.OwnerID, threadID string) (time.Time, error)
}
