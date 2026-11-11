package fcm

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Scope is the OAuth2 scope that allows sending FCM messages.
const Scope = "https://www.googleapis.com/auth/firebase.messaging"

// serviceAccount is the part of a Google service account key file we use.
type serviceAccount struct {
	Type         string `json:"type"`
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	TokenURI     string `json:"token_uri"`
}

// parseServiceAccount reads the JSON key file and its RSA private key.
func parseServiceAccount(raw []byte) (serviceAccount, *rsa.PrivateKey, error) {
	var sa serviceAccount
	fix := "download a service account key (JSON) from Firebase console > Project settings > Service accounts"
	if err := json.Unmarshal(raw, &sa); err != nil {
		return sa, nil, kernel.Invalid("fcm service account is not valid JSON").WithFix("%s", fix)
	}
	if sa.ProjectID == "" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return sa, nil, kernel.Invalid("fcm service account lacks project_id, client_email or private_key").WithFix("%s", fix)
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	key, err := parseRSAKey(sa.PrivateKey)
	if err != nil {
		return sa, nil, err
	}
	return sa, key, nil
}

// parseRSAKey decodes a PEM RSA key in PKCS#8 (what Google issues) or PKCS#1 form.
func parseRSAKey(s string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, kernel.Invalid("fcm private_key is not PEM")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, kernel.Invalid("fcm private_key is not an RSA key")
	}
	k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, kernel.Invalid("fcm private_key cannot be parsed")
	}
	return k, nil
}

// tokenSource gets OAuth2 access tokens with the JWT bearer grant and caches
// each one until a minute before it expires.
type tokenSource struct {
	sa       serviceAccount
	key      *rsa.PrivateKey
	tokenURL string
	client   *http.Client
	clock    kernel.Clock

	mu      sync.Mutex
	token   string
	expires time.Time
}

// Token returns a cached access token or fetches a new one.
func (ts *tokenSource) Token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	now := ts.clock.Now()
	if ts.token != "" && now.Before(ts.expires.Add(-time.Minute)) {
		return ts.token, nil
	}
	tok, ttl, err := ts.fetch(ctx, now)
	if err != nil {
		return "", err
	}
	ts.token, ts.expires = tok, now.Add(ttl)
	return tok, nil
}

// Invalidate drops the cached token, e.g. after FCM answers 401.
func (ts *tokenSource) Invalidate() {
	ts.mu.Lock()
	ts.token = ""
	ts.mu.Unlock()
}

func (ts *tokenSource) fetch(ctx context.Context, now time.Time) (string, time.Duration, error) {
	assertion, err := ts.signJWT(now)
	if err != nil {
		return "", 0, err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, kernel.Internal(err, "build fcm token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := ts.client.Do(req)
	if err != nil {
		e := kernel.Unavailable("google token endpoint is unreachable")
		e.Err = err
		return "", 0, e
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", 0, kernel.Unavailable("google token endpoint returned %d: %s", resp.StatusCode, truncate(body))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return "", 0, kernel.Unavailable("google token endpoint returned no access token")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 3600
	}
	return out.AccessToken, time.Duration(out.ExpiresIn) * time.Second, nil
}

// signJWT builds the RS256 assertion: iss = client email, aud = token_uri,
// one hour lifetime.
func (ts *tokenSource) signJWT(now time.Time) (string, error) {
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	if ts.sa.PrivateKeyID != "" {
		header["kid"] = ts.sa.PrivateKeyID
	}
	claims := map[string]any{
		"iss":   ts.sa.ClientEmail,
		"scope": Scope,
		"aud":   ts.sa.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	h, _ := json.Marshal(header)
	c, _ := json.Marshal(claims)
	enc := base64.RawURLEncoding
	signing := enc.EncodeToString(h) + "." + enc.EncodeToString(c)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, ts.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", kernel.Internal(err, "sign fcm jwt")
	}
	return signing + "." + enc.EncodeToString(sig), nil
}
