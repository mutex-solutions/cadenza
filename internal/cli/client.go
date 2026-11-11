package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// client calls the server as one agent session.
type client struct {
	a          *App
	server     string
	machineKey string
	sess       agentSession
	sessionKey string
}

// agentClient loads the pairing and the agent session's key, minting one if
// this session has none yet.
func (a *App) agentClient(ctx context.Context, f *flags) (*client, error) {
	cfg, mk, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	if s := f.str("server"); s != "" {
		cfg.Server = strings.TrimRight(s, "/")
	}
	sess, err := a.resolveSession(f)
	if err != nil {
		return nil, err
	}
	c := &client{a: a, server: cfg.Server, machineKey: mk, sess: sess, sessionKey: a.loadSessionKey(sess.Agent, sess.ID)}
	if c.sessionKey == "" {
		if err := c.mint(ctx); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// mint gets a new key for this agent session using the machine key.
func (c *client) mint(ctx context.Context) error {
	var out spec.SessionKeyOutput
	in := spec.SessionKeyInput{Agent: c.sess.Agent, AgentSessionID: c.sess.ID, Cwd: c.a.Cwd, Repo: gitRemote(c.a.Cwd)}
	if err := c.a.do(ctx, c.server, "session-key", nil, nil, c.machineKey, in, &out); err != nil {
		return err
	}
	c.sessionKey = out.SessionKey
	return c.a.saveSessionKey(c.sess.Agent, c.sess.ID, out.SessionKey)
}

// call makes an API call as the session. An expired or replaced session key
// is renewed once, automatically.
func (c *client) call(ctx context.Context, name string, params map[string]string, query url.Values, in, out any) error {
	err := c.a.do(ctx, c.server, name, params, query, c.sessionKey, in, out)
	if kernel.Is(err, kernel.KindUnauthorized) {
		if merr := c.mint(ctx); merr != nil {
			return merr
		}
		err = c.a.do(ctx, c.server, name, params, query, c.sessionKey, in, out)
	}
	return err
}

// do performs one call described by the spec endpoint name.
func (a *App) do(ctx context.Context, server, name string, params map[string]string, query url.Values, token string, in, out any) error {
	e, ok := spec.Lookup(name)
	if !ok {
		return kernel.Internal(nil, "unknown endpoint %s", name)
	}
	path := e.Path
	for k, v := range params {
		path = strings.ReplaceAll(path, "{"+k+"}", url.PathEscape(v))
	}
	u := server + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, e.Method, u, body)
	if err != nil {
		return kernel.Invalid("bad server URL %q", server).WithFix("run `cadenza login <server-url>` again")
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("User-Agent", "cadenza-cli/"+a.Version)
	res, err := a.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &kernel.Error{Kind: kernel.KindUnavailable, Msg: fmt.Sprintf("Cadenza is unreachable at %s", server),
			Fix: "check the network or the server; you decide whether to carry on without it", Err: err}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return kernel.Unavailable("reading the server's answer failed: %v", err)
	}
	if res.StatusCode >= 300 {
		var se spec.Error
		if json.Unmarshal(raw, &se) != nil || se.Error.Kind == "" {
			return kernel.Unavailable("the server answered %d", res.StatusCode).WithFix("check that %s is a Cadenza server", server)
		}
		return &kernel.Error{Kind: kernel.Kind(se.Error.Kind), Msg: se.Error.Message, Fix: se.Error.Fix}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return kernel.Internal(err, "decode %s answer", name)
		}
	}
	return nil
}
