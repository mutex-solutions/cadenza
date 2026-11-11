// Package catalog is the template catalog: the built-in templates plus
// custom ones made by agents or the owner. Custom template versions are
// immutable and kept forever, so old posts always render (plan gap G-14).
package catalog

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Template sources.
const (
	SourceBuiltin = "builtin"
	SourceAgent   = "agent"
	SourceOwner   = "owner"
)

// Stored is one version of a custom template.
type Stored struct {
	Owner      kernel.OwnerID
	Name       string
	Version    int
	Source     string
	Definition json.RawMessage
	CreatedAt  time.Time
}

// Repo stores custom templates.
type Repo interface {
	CreateTemplate(ctx context.Context, t *Stored) error
	GetTemplate(ctx context.Context, owner kernel.OwnerID, name string, version int) (*Stored, error)
	LatestTemplate(ctx context.Context, owner kernel.OwnerID, name string) (*Stored, error)
	ListLatestTemplates(ctx context.Context, owner kernel.OwnerID) ([]Stored, error)
}
