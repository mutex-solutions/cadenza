package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// pollEvery is how often login checks whether the owner approved the code.
const pollEvery = 2 * time.Second

func cmdLogin(ctx context.Context, a *App, args []string) error {
	f, err := parseFlags(args, []string{"name"}, nil)
	if err != nil {
		return err
	}
	if len(f.pos) != 1 {
		return kernel.Invalid("login needs the server URL").WithFix("cadenza login https://cadenza.example.com")
	}
	server := strings.TrimRight(f.pos[0], "/")
	if u, err := url.Parse(server); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return kernel.Invalid("%q is not a server URL", f.pos[0]).WithFix("use the full address, e.g. https://cadenza.example.com")
	}
	var v spec.Version
	if err := a.do(ctx, server, "version", nil, nil, "", nil, &v); err != nil {
		return err
	}
	if v.API != spec.APIVersion {
		return kernel.Conflict("the server speaks API v%d; this cadenza speaks v%d", v.API, spec.APIVersion).WithFix("install the cadenza that matches the server")
	}
	name := orDefault(f.str("name"), a.Hostname)
	var start spec.PairStartOutput
	if err := a.do(ctx, server, "pair-start", nil, nil, "", spec.PairStartInput{Hostname: name}, &start); err != nil {
		return err
	}
	fmt.Fprintf(a.Stderr, "\n  Pairing code: %s\n\n  Open Cadenza (%s) → Machines → Pair a machine, and enter the code.\n  Waiting for approval (the code works for 10 minutes)…\n\n", start.Code, server)
	for {
		var poll spec.PairPollOutput
		if err := a.do(ctx, server, "pair-poll", map[string]string{"id": start.PairID}, nil, "", spec.PairPollInput{Secret: start.Secret}, &poll); err != nil {
			return err
		}
		switch poll.Status {
		case spec.PairApproved:
			if err := a.saveConfig(&Config{Server: server, MachineID: poll.MachineID, MachineName: poll.MachineName}, poll.MachineKey); err != nil {
				return err
			}
			return a.print(map[string]string{"paired": poll.MachineName, "server": server,
				"next": "run `cadenza setup` to teach your agents to use Cadenza"})
		case spec.PairExpired:
			return kernel.Conflict("the pairing code expired").WithFix("run `cadenza login` again")
		}
		if err := a.Sleep(ctx, pollEvery); err != nil {
			return err
		}
	}
}

func cmdLogout(_ context.Context, a *App, _ []string) error {
	dir := a.configDir()
	for _, p := range []string{"config.json", "machine.key"} {
		if err := os.Remove(filepath.Join(dir, p)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.RemoveAll(filepath.Join(dir, "sessions")); err != nil {
		return err
	}
	return a.print(map[string]string{"logged_out": dir, "note": "remove the machine in the app too, so its key stops working on the server"})
}
