package s3

import (
	"context"
	"errors"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

// Stat asks the store for the object's size with a HEAD request. A missing
// object is (0, false, nil); any other failure is kernel.KindUnavailable.
func (s *Store) Stat(ctx context.Context, sha256 string) (int64, bool, error) {
	if !media.ValidSHA256(sha256) {
		return 0, false, kernel.Invalid("s3: sha256 %q is not 64 lowercase hex characters", sha256)
	}
	out, err := s.api.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(media.ObjectKey(sha256)),
	})
	if err != nil {
		var status interface{ HTTPStatusCode() int }
		if errors.As(err, &status) && status.HTTPStatusCode() == http.StatusNotFound {
			return 0, false, nil
		}
		e := kernel.Unavailable("s3: checking object failed")
		e.Err = err
		return 0, false, e
	}
	return aws.ToInt64(out.ContentLength), true, nil
}
