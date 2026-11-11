package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// gitRemote returns the origin URL of the repository containing dir, read
// straight from .git/config (no git binary needed), or "".
func gitRemote(dir string) string {
	for d := dir; d != "" && d != filepath.Dir(d); d = filepath.Dir(d) {
		f, err := os.Open(filepath.Join(d, ".git", "config"))
		if err != nil {
			continue
		}
		defer f.Close()
		inOrigin := false
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "[") {
				inOrigin = line == `[remote "origin"]`
				continue
			}
			if k, v, ok := strings.Cut(line, "="); inOrigin && ok && strings.TrimSpace(k) == "url" {
				return stripCredentials(strings.TrimSpace(v))
			}
		}
		return ""
	}
	return ""
}

// stripCredentials removes user:password@ from https URLs.
func stripCredentials(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		rest := u[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 && at < strings.Index(rest+"/", "/") {
			return u[:i+3] + rest[at+1:]
		}
	}
	return u
}
