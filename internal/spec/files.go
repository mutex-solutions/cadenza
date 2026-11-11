package spec

import "github.com/mutex-solutions/cadenza/internal/media"

// UploadRequest lists files the caller wants to store, by content hash.
type UploadRequest struct {
	Files []media.FileRef `json:"files"`
}

// UploadResponse says which files are new and gives a signed upload for each.
// Files already stored are listed in Existing and need no upload.
type UploadResponse struct {
	Uploads  []Upload `json:"uploads"`
	Existing []string `json:"existing"`
}

// Upload is one signed PUT, straight to object storage.
type Upload struct {
	SHA256  string            `json:"sha256"`
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Expires int64             `json:"expires"` // unix ms
}

// FileURL is a signed download link.
type FileURL struct {
	URL     string `json:"url"`
	Expires int64  `json:"expires"`
}
