package prepare

import (
	"strings"
	"testing"
)

// Synthetic secrets are built by concatenation so secret scanners (gitleaks)
// never see them whole in the source.

func TestCheckSecretsNames(t *testing.T) {
	refused := []string{".env", ".env.local", ".env.production", "server.pem", "tls.KEY", "id_rsa", "/home/u/.ssh/id_ed25519",
		"credentials.json", ".npmrc", ".netrc", "cert.p12", ".git-credentials"}
	for _, n := range refused {
		if e := wantInvalidNoFatal(CheckSecrets(n, nil)); e == "" {
			t.Errorf("%s: not refused", n)
		}
	}
	allowed := []string{".env.example", "env.md", "keys.md", "id_rsa.pub", "report.pdf", "credentials.md"}
	for _, n := range allowed {
		if err := CheckSecrets(n, []byte("hello")); err != nil {
			t.Errorf("%s: refused: %v", n, err)
		}
	}
	head := []byte("-----BEGIN " + "OPENSSH PRIVATE KEY-----\nabc")
	if CheckSecrets("notes.txt", head) == nil {
		t.Error("private key content not refused")
	}
}

func wantInvalidNoFatal(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestScanTextRules(t *testing.T) {
	cases := map[string]string{
		"aws-access-key": "AKIA" + "IOSFODNN7EXAMPLQ",
		"github-token":   "ghp_" + strings.Repeat("Ab1", 12),
		"slack-token":    "xoxb-" + "1234567890-abcdefghij",
		"api-key":        "sk-ant-" + "api03-" + strings.Repeat("x", 30),
		"cadenza-key":    "cz_mk_" + strings.Repeat("Q", 40),
		"jwt":            "eyJ" + "hbGciOiJIUzI1NiJ9" + ".eyJ" + "zdWIiOiIxMjM0NTY3ODkwIn0" + ".dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		"private-key":    "-----BEGIN " + "RSA PRIVATE KEY-----\nMIIabc\n-----END " + "RSA PRIVATE KEY-----",
	}
	for rule, secret := range cases {
		text := "before " + secret + " after"
		got := ScanText(text)
		if len(got) != 1 || got[0].Rule != rule || text[got[0].Start:got[0].End] != secret {
			t.Errorf("%s: got %+v", rule, got)
		}
		if r := Redact(text); r != "before [redacted "+rule+"] after" {
			t.Errorf("%s: redacted to %q", rule, r)
		}
	}
	more := []string{"github_pat_" + strings.Repeat("A", 30), "sk-" + "proj-" + strings.Repeat("z", 24), "ASIA" + "ABCDEFGHIJKLMNOP"}
	for _, s := range more {
		if len(ScanText(s)) != 1 {
			t.Errorf("%q not found", s)
		}
	}
}

func TestScanTextAssignment(t *testing.T) {
	val := "hunter2" + "hunter2"
	for _, text := range []string{"password=" + val, `"api_key": "` + val + `"`, "DB_PASSWORD: " + val, "client-secret = " + val} {
		got := Redact(text)
		if strings.Contains(got, val) || !strings.Contains(got, "[redacted assignment]") {
			t.Errorf("%q -> %q", text, got)
		}
	}
	if r := Redact("password=" + val); r != "password=[redacted assignment]" {
		t.Fatalf("key name lost: %q", r)
	}
	for _, text := range []string{"password=short", "the token is shown once", "secret: <redacted>"} {
		if f := ScanText(text); len(f) != 0 {
			t.Errorf("%q: false positive %+v", text, f)
		}
	}
}

func TestScanTextOverlapAndOrder(t *testing.T) {
	key := "sk-" + strings.Repeat("k", 30)
	text := "api_key=" + key + " and AKIA" + "ABCDEFGHIJKLMNOP"
	got := ScanText(text)
	if len(got) != 2 || got[0].Start > got[1].Start {
		t.Fatalf("got %+v", got)
	}
	if r := Redact(text); strings.Count(r, "[redacted") != 2 || strings.Contains(r, key) {
		t.Fatalf("redacted %q", r)
	}
	if Redact("nothing here") != "nothing here" {
		t.Fatal("clean text changed")
	}
	pk := "-----BEGIN " + "PRIVATE KEY-----\nMIIunterminated"
	if Redact("x "+pk) != "x [redacted private-key]" {
		t.Fatalf("unterminated key: %q", Redact("x "+pk))
	}
}
