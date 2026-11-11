// Package server wires Cadenza's services, storage and adapters into one
// HTTP server. It is the composition root: the only place that knows which
// implementation backs each port.
package server

import (
	"fmt"
	"net/url"
	"strings"
)

// Config is read from CADENZA_* environment variables (see docs/self-hosting.md).
type Config struct {
	Addr      string // listen address, e.g. ":8080"
	PublicURL string // how phones and agents reach this server, e.g. https://cadenza.example.com
	Database  string // "sqlite:<path>" or "postgres://…"
	DataDir   string // local files: the SQLite database and trial-mode blobs

	S3 S3Config // object storage; when Bucket is empty, files are kept on local disk (trial mode)

	VAPIDSubject      string // contact for Web Push, e.g. mailto:you@example.com
	ExpoEnabled       bool   // send to the Android app through Expo push
	ExpoAccessToken   string // the self-hoster's own Expo access token (optional)
	FCMServiceAccount string // path to the self-hoster's Firebase service account JSON (optional)

	MinCLIVersion string
}

// S3Config is any S3-compatible store.
type S3Config struct {
	Endpoint       string
	PublicEndpoint string
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
	Checksum       string // "sha256" (default) or "none" for stores that reject checksums (Backblaze B2)
}

// LoadConfig reads the environment through getenv (os.Getenv in production).
func LoadConfig(getenv func(string) string) (Config, error) {
	get := func(k, def string) string {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
		return def
	}
	c := Config{
		Addr:      get("CADENZA_ADDR", ":8080"),
		PublicURL: strings.TrimRight(get("CADENZA_PUBLIC_URL", "http://localhost:8080"), "/"),
		DataDir:   get("CADENZA_DATA_DIR", "./data"),
		S3: S3Config{
			Endpoint: get("CADENZA_S3_ENDPOINT", ""), PublicEndpoint: get("CADENZA_S3_PUBLIC_ENDPOINT", ""),
			Region: get("CADENZA_S3_REGION", "us-east-1"), Bucket: get("CADENZA_S3_BUCKET", ""),
			AccessKey: get("CADENZA_S3_ACCESS_KEY", ""), SecretKey: get("CADENZA_S3_SECRET_KEY", ""),
			Checksum: get("CADENZA_S3_CHECKSUM", "sha256"),
		},
		VAPIDSubject:      get("CADENZA_VAPID_SUBJECT", "mailto:admin@localhost"),
		ExpoEnabled:       get("CADENZA_EXPO", "") == "1" || get("CADENZA_EXPO_ACCESS_TOKEN", "") != "",
		ExpoAccessToken:   get("CADENZA_EXPO_ACCESS_TOKEN", ""),
		FCMServiceAccount: get("CADENZA_FCM_SERVICE_ACCOUNT_FILE", ""),
		MinCLIVersion:     get("CADENZA_MIN_CLI_VERSION", "0.1.0"),
	}
	c.Database = get("CADENZA_DATABASE", "sqlite:"+strings.TrimRight(c.DataDir, "/")+"/cadenza.db")
	return c, c.validate()
}

func (c Config) validate() error {
	u, err := url.Parse(c.PublicURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("CADENZA_PUBLIC_URL %q must be an http(s) URL such as https://cadenza.example.com", c.PublicURL)
	}
	if !strings.HasPrefix(c.Database, "sqlite:") && !strings.HasPrefix(c.Database, "postgres://") && !strings.HasPrefix(c.Database, "postgresql://") {
		return fmt.Errorf("CADENZA_DATABASE %q must start with sqlite: or postgres://", c.Database)
	}
	if c.S3.Bucket != "" && (c.S3.AccessKey == "" || c.S3.SecretKey == "") {
		return fmt.Errorf("CADENZA_S3_BUCKET is set, so CADENZA_S3_ACCESS_KEY and CADENZA_S3_SECRET_KEY are required")
	}
	if c.S3.Checksum != "sha256" && c.S3.Checksum != "none" {
		return fmt.Errorf("CADENZA_S3_CHECKSUM must be sha256 or none")
	}
	return nil
}

// Secure reports whether the public URL is HTTPS (cookies get the Secure flag).
func (c Config) Secure() bool { return strings.HasPrefix(c.PublicURL, "https://") }

// Trial reports whether files are kept on local disk instead of S3.
func (c Config) Trial() bool { return c.S3.Bucket == "" }
