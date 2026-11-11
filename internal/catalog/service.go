package catalog

import (
	"context"
	"encoding/json"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/review"
	"github.com/mutex-solutions/cadenza/internal/spec"
	"github.com/mutex-solutions/cadenza/internal/template"
)

// Service lists, shows, creates and checks templates. It implements
// review.Templates.
type Service struct {
	repo   Repo
	render template.Renderer
	clock  kernel.Clock
}

func NewService(repo Repo, render template.Renderer, clock kernel.Clock) *Service {
	return &Service{repo: repo, render: render, clock: clock}
}

var _ review.Templates = (*Service)(nil)

// Lookup finds a template: built-ins first (their names are reserved), then
// the owner's custom templates. Version 0 means the latest.
func (s *Service) Lookup(ctx context.Context, owner kernel.OwnerID, name string, version int) (template.Definition, string, error) {
	if d, ok := template.Builtin(name); ok {
		if version != 0 && version != template.BuiltinVersion {
			return template.Definition{}, "", kernel.NotFound("template %s has no version %d", name, version).WithFix("leave the version out to use the latest")
		}
		return d, SourceBuiltin, nil
	}
	var st *Stored
	var err error
	if version == 0 {
		st, err = s.repo.LatestTemplate(ctx, owner, name)
	} else {
		st, err = s.repo.GetTemplate(ctx, owner, name, version)
	}
	if kernel.Is(err, kernel.KindNotFound) {
		return template.Definition{}, "", kernel.NotFound("no template named %q", name).WithFix("run `cadenza templates` to list them")
	}
	if err != nil {
		return template.Definition{}, "", err
	}
	d, err := template.ParseDefinition(st.Definition)
	if err != nil {
		return template.Definition{}, "", kernel.Internal(err, "stored template %s v%d is invalid", name, st.Version)
	}
	d.Version = st.Version
	return d, st.Source, nil
}

// Check validates filled template data and renders it for the apps.
func (s *Service) Check(ctx context.Context, owner kernel.OwnerID, name string, version int, data json.RawMessage) (review.Checked, error) {
	d, _, err := s.Lookup(ctx, owner, name, version)
	if err != nil {
		return review.Checked{}, err
	}
	res, err := d.Validate(data, template.Remote)
	if err != nil {
		return review.Checked{}, err
	}
	view, err := d.Render(data, s.render)
	if err != nil {
		return review.Checked{}, err
	}
	out := review.Checked{Version: d.Version, Files: res.Refs, View: view}
	if text, choices, ok := d.Question(data); ok {
		out.Question = &review.TemplateQuestion{Text: text, Choices: choices}
	}
	return out, nil
}

// List returns built-ins, then custom templates.
func (s *Service) List(ctx context.Context, owner kernel.OwnerID) (spec.TemplateList, error) {
	out := spec.TemplateList{Templates: []spec.TemplateSummary{}}
	for _, d := range template.Builtins() {
		out.Templates = append(out.Templates, spec.TemplateSummary{Name: d.Name, Version: template.BuiltinVersion, UseFor: d.UseFor, Source: SourceBuiltin})
	}
	custom, err := s.repo.ListLatestTemplates(ctx, owner)
	if err != nil {
		return out, err
	}
	for _, st := range custom {
		d, err := template.ParseDefinition(st.Definition)
		if err != nil {
			continue // never fail a listing over one bad row
		}
		out.Templates = append(out.Templates, spec.TemplateSummary{Name: st.Name, Version: st.Version, UseFor: d.UseFor, Source: st.Source})
	}
	return out, nil
}

// Get returns one template with its definition, guide and example.
func (s *Service) Get(ctx context.Context, owner kernel.OwnerID, name string) (spec.TemplateInfo, error) {
	d, source, err := s.Lookup(ctx, owner, name, 0)
	if err != nil {
		return spec.TemplateInfo{}, err
	}
	def, err := json.Marshal(d)
	if err != nil {
		return spec.TemplateInfo{}, kernel.Internal(err, "encode template")
	}
	return spec.TemplateInfo{Name: d.Name, Version: d.Version, Source: source, Definition: def, Guide: d.Guide, Example: d.Example}, nil
}

// Make stores a custom template. It is usable at once, with no approval
// (Cadenza does not gate agents). An existing name gets a new version.
func (s *Service) Make(ctx context.Context, owner kernel.OwnerID, source string, raw json.RawMessage) (spec.MakeTemplateOutput, error) {
	d, err := template.ParseDefinition(raw)
	if err != nil {
		return spec.MakeTemplateOutput{}, err
	}
	if _, ok := template.Builtin(d.Name); ok {
		return spec.MakeTemplateOutput{}, kernel.Conflict("%q is a built-in template", d.Name).WithFix("pick another name, e.g. %q", d.Name+"-custom")
	}
	var lastErr error
	for range 3 { // a concurrent Make of the same name can take our version; try again
		version := 1
		latest, err := s.repo.LatestTemplate(ctx, owner, d.Name)
		switch {
		case err == nil:
			version = latest.Version + 1
		case !kernel.Is(err, kernel.KindNotFound):
			return spec.MakeTemplateOutput{}, err
		}
		d.Version = version
		def, err := json.Marshal(d)
		if err != nil {
			return spec.MakeTemplateOutput{}, kernel.Internal(err, "encode template")
		}
		lastErr = s.repo.CreateTemplate(ctx, &Stored{Owner: owner, Name: d.Name, Version: version, Source: source, Definition: def, CreatedAt: s.clock.Now()})
		if lastErr == nil {
			return spec.MakeTemplateOutput{Name: d.Name, Version: version, Created: version == 1}, nil
		}
	}
	return spec.MakeTemplateOutput{}, lastErr
}
