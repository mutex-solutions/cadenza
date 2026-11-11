package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mutex-solutions/cadenza/internal/cli/prepare"
	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// prepared is a local file converted and ready to upload.
type prepared struct {
	*prepare.Prepared
	path string // as the agent wrote it
}

// abs resolves an agent-given path against the working directory.
func (a *App) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(a.Cwd, p)
}

// prepareFiles converts each file (plan D10, D11). The caller must call
// cleanup when done.
func (a *App) prepareFiles(ctx context.Context, paths []string) (out []prepared, warnings []string, cleanup func(), err error) {
	cleanup = func() {
		for _, p := range out {
			p.Cleanup()
		}
	}
	for _, p := range paths {
		pp, err := prepare.PrepareFile(ctx, a.abs(p), prepare.Options{TempDir: os.TempDir(), AllowFFmpeg: true})
		if err != nil {
			cleanup()
			return nil, nil, func() {}, err
		}
		out = append(out, prepared{Prepared: pp, path: p})
		for _, w := range pp.Warnings {
			warnings = append(warnings, p+": "+w)
		}
	}
	return out, warnings, cleanup, nil
}

// upload sends every prepared file (and preview) that the server does not
// have yet straight to storage with signed links.
func (c *client) upload(ctx context.Context, files []prepared) error {
	var refs []media.FileRef
	bodies := map[string]func() (io.ReadCloser, error){}
	for _, f := range files {
		if f.Inline {
			continue
		}
		refs = append(refs, f.File)
		bodies[f.File.SHA256] = f.Open
		if f.Preview != nil {
			refs = append(refs, f.Preview.File)
			data := f.Preview.Data
			bodies[f.Preview.File.SHA256] = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(string(data))), nil }
		}
	}
	if len(refs) == 0 {
		return nil
	}
	var resp spec.UploadResponse
	if err := c.call(ctx, "upload", nil, nil, spec.UploadRequest{Files: refs}, &resp); err != nil {
		return err
	}
	sizes := map[string]int64{}
	for _, r := range refs {
		sizes[r.SHA256] = r.Size
	}
	for _, up := range resp.Uploads {
		open, ok := bodies[up.SHA256]
		if !ok {
			return kernel.Internal(nil, "server asked for an unknown file %s", up.SHA256)
		}
		if err := c.put(ctx, up, open, sizes[up.SHA256]); err != nil {
			return err
		}
	}
	return nil
}

// put uploads one file, retrying twice on network errors and 5xx answers.
func (c *client) put(ctx context.Context, up spec.Upload, open func() (io.ReadCloser, error), size int64) error {
	var last error
	for attempt := range 3 {
		if attempt > 0 {
			if err := c.a.Sleep(ctx, time.Duration(attempt)*time.Second); err != nil {
				return err
			}
		}
		body, err := open()
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, orDefault(up.Method, http.MethodPut), up.URL, body)
		if err != nil {
			body.Close()
			return kernel.Internal(err, "bad upload link")
		}
		req.ContentLength = size
		for k, v := range up.Headers {
			req.Header.Set(k, v)
		}
		res, err := c.a.HTTP.Do(req)
		body.Close()
		if err != nil {
			last = &kernel.Error{Kind: kernel.KindUnavailable, Msg: "file storage is unreachable", Fix: "check the network; retry the command (it is safe to retry)", Err: err}
			continue
		}
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		res.Body.Close()
		switch {
		case res.StatusCode < 300:
			return nil
		case res.StatusCode >= 500:
			last = kernel.Unavailable("file storage answered %d", res.StatusCode)
		default:
			return kernel.Invalid("file storage refused the upload (%d): %s", res.StatusCode, strings.TrimSpace(string(msg))).
				WithFix("retry the command; if it keeps failing, check the server's storage settings")
		}
	}
	return last
}

// download saves a file from a signed link to dir/name and returns the path.
func (a *App) download(ctx context.Context, url, dir, name string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = "file"
	}
	dst := filepath.Join(dir, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	res, err := a.HTTP.Do(req)
	if err != nil {
		return "", &kernel.Error{Kind: kernel.KindUnavailable, Msg: "file storage is unreachable", Err: err}
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return "", kernel.Unavailable("downloading %s failed: %d", name, res.StatusCode)
	}
	tmp, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, res.Body); err != nil {
		tmp.Close()
		return "", fmt.Errorf("download %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(a.Cwd, dst)
	if err != nil {
		return dst, nil
	}
	return rel, nil
}

// saveResponseFiles downloads a response's files under .cadenza/responses/<id>/.
func (a *App) saveResponseFiles(ctx context.Context, r *spec.Response) error {
	for i := range r.Files {
		f := &r.Files[i]
		if f.URL == "" {
			continue
		}
		p, err := a.download(ctx, f.URL, filepath.Join(a.Cwd, ".cadenza", "responses", r.ID), f.Name)
		if err != nil {
			return err
		}
		f.Path, f.URL = p, ""
	}
	return nil
}
