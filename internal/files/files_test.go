package files

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

type memRepo struct{ recs map[string]Record }

func (m *memRepo) GetFile(_ context.Context, owner kernel.OwnerID, sha string) (*Record, error) {
	r, ok := m.recs[string(owner)+sha]
	if !ok {
		return nil, kernel.NotFound("no file")
	}
	return &r, nil
}

func (m *memRepo) SaveFile(_ context.Context, r *Record) error {
	if _, ok := m.recs[string(r.Owner)+r.SHA256]; !ok {
		m.recs[string(r.Owner)+r.SHA256] = *r
	}
	return nil
}

type fakeBlobs struct {
	objects map[string]int64
	fail    bool
	puts    int
}

func (f *fakeBlobs) PresignPut(_ context.Context, ref media.FileRef, ttl time.Duration) (spec.Upload, error) {
	if f.fail {
		return spec.Upload{}, errors.New("down")
	}
	f.puts++
	return spec.Upload{SHA256: ref.SHA256, URL: "https://s3/" + ref.SHA256, Method: "PUT"}, nil
}

func (f *fakeBlobs) PresignGet(_ context.Context, sha, typ, name string, ttl time.Duration) (spec.FileURL, error) {
	if f.fail {
		return spec.FileURL{}, errors.New("down")
	}
	return spec.FileURL{URL: "https://s3/" + sha + "?type=" + typ + "&name=" + name}, nil
}

func (f *fakeBlobs) Stat(_ context.Context, sha string) (int64, bool, error) {
	if f.fail {
		return 0, false, errors.New("down")
	}
	n, ok := f.objects[sha]
	return n, ok, nil
}

const (
	shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func newSvc() (*Service, *memRepo, *fakeBlobs) {
	r := &memRepo{recs: map[string]Record{}}
	b := &fakeBlobs{objects: map[string]int64{}}
	return NewService(r, b, kernel.NewFakeClock(time.Now())), r, b
}

func ref(sha string, size int64) media.FileRef {
	return media.FileRef{SHA256: sha, Size: size, Type: media.TypeWebP, Name: "a.webp"}
}

func TestPrepareUploadOnlySignsNewFiles(t *testing.T) {
	s, repo, blobs := newSvc()
	ctx := context.Background()
	blobs.objects[shaA] = 10 // already in the bucket, not yet recorded
	out, err := s.PrepareUpload(ctx, "o_1", []media.FileRef{ref(shaA, 10), ref(shaB, 20), ref(shaB, 20)})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Existing) != 1 || out.Existing[0] != shaA || len(out.Uploads) != 1 || out.Uploads[0].SHA256 != shaB {
		t.Fatalf("unexpected %+v", out)
	}
	if _, err := repo.GetFile(ctx, "o_1", shaA); err != nil {
		t.Fatal("found object should be recorded")
	}
	// A size mismatch means the object is not the one described: upload again.
	blobs.objects[shaB] = 99
	out, _ = s.PrepareUpload(ctx, "o_1", []media.FileRef{ref(shaB, 20)})
	if len(out.Uploads) != 1 {
		t.Fatal("size mismatch must require an upload")
	}
}

func TestPrepareUploadErrors(t *testing.T) {
	s, _, blobs := newSvc()
	ctx := context.Background()
	if _, err := s.PrepareUpload(ctx, "o_1", nil); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("empty list accepted")
	}
	if _, err := s.PrepareUpload(ctx, "o_1", []media.FileRef{{SHA256: "x"}}); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("invalid ref accepted")
	}
	blobs.fail = true
	if _, err := s.PrepareUpload(ctx, "o_1", []media.FileRef{ref(shaA, 1)}); !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatalf("storage down should be unavailable, got %v", err)
	}
}

func TestConfirm(t *testing.T) {
	s, _, blobs := newSvc()
	ctx := context.Background()
	err := s.Confirm(ctx, "o_1", []media.FileRef{ref(shaA, 10)})
	if !kernel.Is(err, kernel.KindInvalid) || !strings.Contains(err.Error(), "not uploaded") {
		t.Fatalf("missing file: %v", err)
	}
	blobs.objects[shaA] = 10
	if err := s.Confirm(ctx, "o_1", []media.FileRef{ref(shaA, 10)}); err != nil {
		t.Fatal(err)
	}
	// Previews are confirmed with size 0.
	blobs.objects[shaB] = 5
	if err := s.Confirm(ctx, "o_1", []media.FileRef{{SHA256: shaB, Type: media.TypeWebP}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Confirm(ctx, "o_1", nil); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadURL(t *testing.T) {
	s, _, blobs := newSvc()
	ctx := context.Background()
	if _, err := s.DownloadURL(ctx, "o_1", "nope"); !kernel.Is(err, kernel.KindInvalid) {
		t.Fatal("bad sha accepted")
	}
	if _, err := s.DownloadURL(ctx, "o_1", shaA); !kernel.Is(err, kernel.KindNotFound) {
		t.Fatal("unknown file signed")
	}
	blobs.objects[shaA] = 10
	if err := s.Confirm(ctx, "o_1", []media.FileRef{ref(shaA, 10)}); err != nil {
		t.Fatal(err)
	}
	u, err := s.DownloadURL(ctx, "o_1", shaA)
	if err != nil || !strings.Contains(u.URL, "type=image/webp") || !strings.Contains(u.URL, "name=a.webp") {
		t.Fatalf("DownloadURL = %v, %v", u, err)
	}
	if _, err := s.DownloadURL(ctx, "o_2", shaA); !kernel.Is(err, kernel.KindNotFound) {
		t.Fatal("another owner's file signed")
	}
	blobs.fail = true
	if _, err := s.DownloadURL(ctx, "o_1", shaA); !kernel.Is(err, kernel.KindUnavailable) {
		t.Fatal("storage down should be unavailable")
	}
}
