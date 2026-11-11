package prepare

import (
	"bytes"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Secret scanning (plan §11): the CLI refuses files that hold secrets by
// their nature (.env, keys, credentials) and hides secret-looking text in
// what it shows. The rule list is small on purpose; it catches the common
// mistakes, not every possible secret.

// secretNames are exact file names that are refused.
var secretNames = map[string]bool{
	".env": true, "id_rsa": true, "id_dsa": true, "id_ecdsa": true, "id_ed25519": true,
	"credentials.json": true, "credentials": true, "service-account.json": true,
	".npmrc": true, ".netrc": true, "_netrc": true, ".pypirc": true, ".pgpass": true,
	".git-credentials": true, ".htpasswd": true, "secrets.json": true,
}

// secretExts are file extensions that are refused.
var secretExts = map[string]bool{
	".pem": true, ".key": true, ".p12": true, ".pfx": true, ".jks": true, ".keystore": true, ".kdbx": true,
}

// envExamples are .env.* names that hold placeholders, not secrets.
var envExamples = map[string]bool{".env.example": true, ".env.sample": true, ".env.template": true}

// CheckSecrets refuses a file whose name says it holds secrets (.env,
// .env.*, *.pem, *.key, id_rsa, credentials.json, .npmrc, .netrc and the
// like) or whose first bytes are a private key.
func CheckSecrets(path string, head []byte) error {
	name := strings.ToLower(filepath.Base(path))
	refused := secretNames[name] || secretExts[filepath.Ext(name)] ||
		(strings.HasPrefix(name, ".env.") && !envExamples[name])
	if !refused && bytes.Contains(head, []byte("-----BEGIN")) && bytes.Contains(head, []byte("PRIVATE KEY-----")) {
		refused = true
	}
	if refused {
		return kernel.Invalid("%s looks like a secrets file; cadenza does not upload it", filepath.Base(path)).
			WithFix("do not show secrets; show a redacted copy or describe the setting instead")
	}
	return nil
}

// Finding is one secret-looking span in a text. Start and End are byte
// offsets of the secret value itself (for assignments, only the value).
type Finding struct {
	Rule  string
	Start int
	End   int
}

// secretRule finds one kind of secret. When group is set, only that
// capture group is the secret (the rest is context such as "password=").
type secretRule struct {
	name  string
	re    *regexp.Regexp
	group int
}

// secretRules is the full list. Each rule is tested in secrets_test.go.
var secretRules = []secretRule{
	{"private-key", regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|\z)`), 0},
	{"aws-access-key", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), 0},
	{"github-token", regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{22,})`), 0},
	{"slack-token", regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}`), 0},
	{"api-key", regexp.MustCompile(`\bsk-(?:ant-|proj-)?[A-Za-z0-9_-]{20,}`), 0},
	{"cadenza-key", regexp.MustCompile(`\bcz_(?:mk|sk)_[A-Za-z0-9_-]{32,}`), 0},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), 0},
	{"assignment", regexp.MustCompile(`(?i)(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|secret[_-]?key|client[_-]?secret|auth[_-]?token)["']?\s*[:=]\s*["']?([^\s"',;<>]{8,})`), 1},
}

// ScanText returns the secret-looking spans in text, sorted by position.
// Overlapping spans are merged and keep the first rule's name.
func ScanText(text string) []Finding {
	var found []Finding
	for _, r := range secretRules {
		for _, m := range r.re.FindAllStringSubmatchIndex(text, -1) {
			s, e := m[2*r.group], m[2*r.group+1]
			if s >= 0 && e > s {
				found = append(found, Finding{Rule: r.name, Start: s, End: e})
			}
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].Start < found[j].Start })
	var out []Finding
	for _, f := range found {
		if n := len(out); n > 0 && f.Start < out[n-1].End {
			out[n-1].End = max(out[n-1].End, f.End)
			continue
		}
		out = append(out, f)
	}
	return out
}

// Redact replaces every secret-looking span with "[redacted <rule>]".
func Redact(text string) string {
	found := ScanText(text)
	if len(found) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	for _, f := range found {
		b.WriteString(text[last:f.Start])
		b.WriteString("[redacted " + f.Rule + "]")
		last = f.End
	}
	b.WriteString(text[last:])
	return b.String()
}
