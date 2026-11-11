package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Config is this machine's pairing, in <config dir>/config.json. The machine
// key lives in machine.key next to it, readable only by the user.
type Config struct {
	Server      string `json:"server"`
	MachineID   string `json:"machine_id"`
	MachineName string `json:"machine_name"`
}

func (a *App) configDir() string {
	if a.ConfigDir != "" {
		return a.ConfigDir
	}
	if d := a.Getenv("CADENZA_CONFIG_DIR"); d != "" {
		return d
	}
	if d := a.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "cadenza")
	}
	return filepath.Join(a.Home, ".config", "cadenza")
}

func (a *App) loadConfig() (*Config, string, error) {
	dir := a.configDir()
	b, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", kernel.Unauthorized("this machine is not paired with a Cadenza server").WithFix("run `cadenza login <server-url>`")
	}
	if err != nil {
		return nil, "", err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, "", kernel.Invalid("%s is damaged", filepath.Join(dir, "config.json")).WithFix("run `cadenza login <server-url>` again")
	}
	key, err := os.ReadFile(filepath.Join(dir, "machine.key"))
	if err != nil {
		return nil, "", kernel.Unauthorized("this machine's key is missing").WithFix("run `cadenza login <server-url>` again")
	}
	if s := a.Getenv("CADENZA_SERVER"); s != "" {
		c.Server = strings.TrimRight(s, "/")
	}
	return &c, strings.TrimSpace(string(key)), nil
}

func (a *App) saveConfig(c *Config, machineKey string) error {
	dir := a.configDir()
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := writeFileAtomic(filepath.Join(dir, "config.json"), b, 0o600); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, "machine.key"), []byte(machineKey+"\n"), 0o600)
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func (a *App) sessionKeyPath(agent, id string) string {
	name := unsafeChars.ReplaceAllString(agent+"__"+id, "_")
	if len(name) > 150 {
		name = name[:150] + "_" + kernel.HashSecret(name)[:16]
	}
	return filepath.Join(a.configDir(), "sessions", name+".key")
}

func (a *App) loadSessionKey(agent, id string) string {
	b, err := os.ReadFile(a.sessionKeyPath(agent, id))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (a *App) saveSessionKey(agent, id, key string) error {
	p := a.sessionKeyPath(agent, id)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(p, []byte(key+"\n"), 0o600)
}

// writeFileAtomic writes via a temp file and rename, so a crash never leaves
// a half-written key.
func writeFileAtomic(path string, data []byte, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
