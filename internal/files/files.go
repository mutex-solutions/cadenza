package files

import (
	"context"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// Link lifetimes.
const (
	UploadTTL   = 15 * time.Minute
	DownloadTTL = time.Hour
)

// Record is a stored file known to belong to an owner.
type Record struct {
	Owner     kernel.OwnerID
	SHA256    string
	Size      int64
	Type      string
	Name      string
	CreatedAt time.Time
}

// Repo stores file records.
type Repo interface {
	GetFile(ctx context.Context, owner kernel.OwnerID, sha256 string) (*Record, error)
	// SaveFile inserts a record; an existing record is left as it is.
	SaveFile(ctx context.Context, r *Record) error
}

// Service decides which files need uploading, confirms uploads and signs
// downloads.
type Service struct {
	repo  Repo
	blobs BlobStore
	clock kernel.Clock
}

func NewService(repo Repo, blobs BlobStore, clock kernel.Clock) *Service {
	return &Service{repo: repo, blobs: blobs, clock: clock}
}

// PrepareUpload returns a signed upload for each file not yet stored.
func (s *Service) PrepareUpload(ctx context.Context, owner kernel.OwnerID, refs []media.FileRef) (spec.UploadResponse, error) {
	if len(refs) == 0 || len(refs) > 100 {
		return spec.UploadResponse{}, kernel.Invalid("send 1 to 100 files, got %d", len(refs))
	}
	out := spec.UploadResponse{Uploads: []spec.Upload{}, Existing: []string{}}
	seen := map[string]bool{}
	for _, ref := range refs {
		if err := ref.Validate(); err != nil {
			return spec.UploadResponse{}, err
		}
		if seen[ref.SHA256] {
			continue
		}
		seen[ref.SHA256] = true
		stored, err := s.stored(ctx, owner, ref)
		if err != nil {
			return spec.UploadResponse{}, err
		}
		if stored {
			out.Existing = append(out.Existing, ref.SHA256)
			continue
		}
		up, err := s.blobs.PresignPut(ctx, ref, UploadTTL)
		if err != nil {
			return spec.UploadResponse{}, kernel.Unavailable("file storage is not reachable").WithFix("check the server's storage settings")
		}
		out.Uploads = append(out.Uploads, up)
	}
	return out, nil
}

// stored reports whether ref's bytes exist, recording them for owner if so.
func (s *Service) stored(ctx context.Context, owner kernel.OwnerID, ref media.FileRef) (bool, error) {
	rec, err := s.repo.GetFile(ctx, owner, ref.SHA256)
	if err == nil {
		return ref.Size == 0 || rec.Size == ref.Size, nil
	}
	if !kernel.Is(err, kernel.KindNotFound) {
		return false, err
	}
	size, exists, err := s.blobs.Stat(ctx, ref.SHA256)
	if err != nil {
		return false, kernel.Unavailable("file storage is not reachable")
	}
	if !exists || (ref.Size != 0 && size != ref.Size) {
		return false, nil
	}
	typ, name := ref.Type, ref.Name
	if typ == "" {
		typ = media.TypeOctetStream
	}
	if name == "" {
		name = ref.SHA256[:12]
	}
	return true, s.repo.SaveFile(ctx, &Record{Owner: owner, SHA256: ref.SHA256, Size: size, Type: typ, Name: name, CreatedAt: s.clock.Now()})
}

// Confirm checks every file is stored. A ref with Size 0 matches any size
// (used for image previews, whose size the post does not carry).
func (s *Service) Confirm(ctx context.Context, owner kernel.OwnerID, refs []media.FileRef) error {
	for _, ref := range refs {
		ok, err := s.stored(ctx, owner, ref)
		if err != nil {
			return err
		}
		if !ok {
			return kernel.Invalid("file %q (%s) is not uploaded", ref.Name, ref.SHA256[:12]).
				WithFix("upload it first: cadenza does this for you; retry the command")
		}
	}
	return nil
}

// DownloadURL signs a link to a file the owner has.
func (s *Service) DownloadURL(ctx context.Context, owner kernel.OwnerID, sha256 string) (spec.FileURL, error) {
	if !media.ValidSHA256(sha256) {
		return spec.FileURL{}, kernel.Invalid("bad sha256")
	}
	rec, err := s.repo.GetFile(ctx, owner, sha256)
	if err != nil {
		return spec.FileURL{}, err
	}
	u, err := s.blobs.PresignGet(ctx, rec.SHA256, rec.Type, rec.Name, DownloadTTL)
	if err != nil {
		return spec.FileURL{}, kernel.Unavailable("file storage is not reachable")
	}
	return u, nil
}
