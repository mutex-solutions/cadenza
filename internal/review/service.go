package review

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/mutex-solutions/cadenza/internal/eventlog"
	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Service implements review's use cases.
type Service struct {
	store     Store
	files     Files
	templates Templates
	render    Renderer
	clock     kernel.Clock
	pub       Publisher
	baseURL   string // public URL of the web app, for links in outputs
}

// Deps are the Service's collaborators.
type Deps struct {
	Store     Store
	Files     Files
	Templates Templates
	Renderer  Renderer
	Clock     kernel.Clock
	Publisher Publisher
	BaseURL   string
}

func NewService(d Deps) *Service {
	return &Service{store: d.Store, files: d.Files, templates: d.Templates, render: d.Renderer,
		clock: d.Clock, pub: d.Publisher, baseURL: strings.TrimRight(d.BaseURL, "/")}
}

func (s *Service) threadURL(id string) string { return s.baseURL + "/t/" + id }

// tx runs fn in a transaction and publishes the events it recorded after commit.
func (s *Service) tx(ctx context.Context, fn func(r Repo, rec *recorder) error) error {
	rec := &recorder{}
	err := s.store.InTx(ctx, func(r Repo) error {
		rec.repo = r
		return fn(r, rec)
	})
	if err == nil {
		s.pub.Publish(rec.events...)
	}
	return err
}

// recorder appends events inside a transaction and remembers them for publishing.
type recorder struct {
	repo   Repo
	events []eventlog.Event
}

func (rc *recorder) add(ctx context.Context, e eventlog.Event) (int64, error) {
	if err := rc.repo.AppendEvent(ctx, &e); err != nil {
		return 0, err
	}
	rc.events = append(rc.events, e)
	return e.Seq, nil
}

// checkText validates a required or optional text field's length.
func checkText(field, v string, required bool, max int) error {
	if required && strings.TrimSpace(v) == "" {
		return kernel.Invalid("%s is required", field)
	}
	if utf8.RuneCountInString(v) > max {
		return kernel.Invalid("%s is too long (%d characters; the limit is %d)", field, utf8.RuneCountInString(v), max)
	}
	return nil
}
