// Package s3 is a files.BlobStore over any S3-compatible object store (AWS,
// MinIO, Cloudflare R2, Backblaze B2, Garage). It only signs links: phones and
// agents move the bytes straight to and from the store (plan D9).
package s3

import (
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/mutex-solutions/cadenza/internal/files"
	"github.com/mutex-solutions/cadenza/internal/kernel"
)

// Checksum modes for Config.ChecksumMode.
const (
	// ChecksumSHA256 signs x-amz-checksum-sha256 into every upload, so the
	// store itself rejects bytes that do not match the file's SHA-256.
	ChecksumSHA256 = "sha256"
	// ChecksumNone signs no checksum header. Use it only for stores that
	// reject checksum headers (Backblaze B2). The store then accepts any
	// bytes under the key; the server must check size with Stat and the
	// content hash cannot be enforced by the store.
	ChecksumNone = "none"
)

// Config says how to reach the store.
type Config struct {
	// Endpoint is the store's base URL as the server reaches it, e.g.
	// "http://minio:9000". Empty means AWS S3 for Region.
	Endpoint string
	// PublicEndpoint, when set, is the base URL put into signed links, for
	// when phones and agents reach the store on a different host than the
	// server does. The store must see this host, because it is signed.
	PublicEndpoint string
	Region         string // e.g. "us-east-1"; "auto" for R2
	Bucket         string
	AccessKey      string
	SecretKey      string
	// UsePathStyle puts the bucket in the path (host/bucket/key) instead of
	// the host name. Nil means true when Endpoint is set and false for AWS.
	UsePathStyle *bool
	// ChecksumMode is ChecksumSHA256 (the default when empty) or ChecksumNone.
	ChecksumMode string
	// HTTPClient is used for Stat. Nil means http.DefaultClient.
	HTTPClient *http.Client
	// MaxAttempts is how many times Stat tries a failing request. Zero means 3.
	MaxAttempts int
}

// Store signs uploads and downloads for one bucket.
type Store struct {
	bucket   string
	checksum string
	api      *s3.Client        // reaches Endpoint, used for Stat
	presign  *s3.PresignClient // signs links for PublicEndpoint (or Endpoint)
}

var _ files.BlobStore = (*Store)(nil)

// New checks cfg and builds a Store. It does not contact the store.
func New(cfg Config) (*Store, error) {
	if cfg.Bucket == "" {
		return nil, kernel.Invalid("s3: bucket is not set")
	}
	if cfg.Region == "" {
		return nil, kernel.Invalid("s3: region is not set").WithFix(`set the region, e.g. "us-east-1" or "auto" for R2`)
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, kernel.Invalid("s3: access key and secret key are required")
	}
	switch cfg.ChecksumMode {
	case "":
		cfg.ChecksumMode = ChecksumSHA256
	case ChecksumSHA256, ChecksumNone:
	default:
		return nil, kernel.Invalid("s3: checksum mode %q is unknown", cfg.ChecksumMode).
			WithFix("use %q or %q", ChecksumSHA256, ChecksumNone)
	}
	for _, e := range []string{cfg.Endpoint, cfg.PublicEndpoint} {
		if e != "" && !strings.HasPrefix(e, "http://") && !strings.HasPrefix(e, "https://") {
			return nil, kernel.Invalid("s3: endpoint %q must start with http:// or https://", e)
		}
	}
	pathStyle := cfg.Endpoint != ""
	if cfg.UsePathStyle != nil {
		pathStyle = *cfg.UsePathStyle
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	public := cfg.PublicEndpoint
	if public == "" {
		public = cfg.Endpoint
	}
	return &Store{
		bucket:   cfg.Bucket,
		checksum: cfg.ChecksumMode,
		api:      newClient(cfg, cfg.Endpoint, pathStyle),
		presign:  s3.NewPresignClient(newClient(cfg, public, pathStyle)),
	}, nil
}

func newClient(cfg Config, endpoint string, pathStyle bool) *s3.Client {
	opts := s3.Options{
		Region:       cfg.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		UsePathStyle: pathStyle,
		// Only add checksums the caller asked for. Otherwise the SDK adds
		// x-amz-sdk-checksum-algorithm to signed links, which many
		// S3-compatible stores reject.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
		RetryMaxAttempts:           cfg.MaxAttempts,
	}
	if endpoint != "" {
		opts.BaseEndpoint = aws.String(endpoint)
	}
	if cfg.HTTPClient != nil {
		opts.HTTPClient = cfg.HTTPClient
	}
	return s3.New(opts)
}
