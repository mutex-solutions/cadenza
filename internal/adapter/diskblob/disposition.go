package diskblob

import (
	"mime"

	"github.com/mutex-solutions/cadenza/internal/media"
)

// contentType is the type the file is served as. Types Cadenza does not
// store become application/octet-stream.
func contentType(ct string) string {
	if media.Allowed(ct) {
		return ct
	}
	return media.TypeOctetStream
}

// disposition is "inline" for kinds the apps show (image, video, webpage,
// PDF) and "attachment" for everything else, with the file name when given.
func disposition(ct, name string) string {
	kind := "attachment"
	if media.KindOf(contentType(ct)) != media.KindOther {
		kind = "inline"
	}
	if name == "" || media.ValidName(name) != nil {
		return kind
	}
	if v := mime.FormatMediaType(kind, map[string]string{"filename": name}); v != "" {
		return v
	}
	return kind
}
