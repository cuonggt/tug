package storage

import (
	"context"
	"crypto/rand"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/cuonggt/tug/internal/filetype"
)

// Uploader is a Disk a browser can put a file on itself, with no app in
// between: UploadURL is a link that takes a PUT of one key, until it
// expires, of what its Conditions say. Local and S3 are both.
//
// It's for a file too big to go through the app, whose request has a body
// limit, and whatever stands in front of it another: a video rather than a
// photo. The app makes the key and the link, the page puts the file there,
// and the app looks at what landed, with Checked, before it keeps the key
// anywhere.
type Uploader interface {
	UploadURL(ctx context.Context, key string, expires time.Time, c Conditions) (string, error)
}

// Conditions are what an upload link holds its upload to. ContentType is
// what the file says it is, which the link signs, so a PUT that says
// anything else is refused, by the bucket or by the local disk's route.
// MaxSize is the most it may be, in bytes, which the local disk refuses as
// it writes; a bucket's presigned PUT has no way to say it, so Checked is
// what sees a file that came in bigger.
type Conditions struct {
	ContentType string
	MaxSize     int64
}

// Stater is a Disk that says what a file it has is, for an app that has to
// look at one a browser uploaded: Local and S3 are both.
type Stater interface {
	Stat(ctx context.Context, key string) (Info, error)
}

// Info is what a file is: how big it is, the type its first bytes say it
// is, which is not what the upload said it was, and when it last changed,
// which is when it was uploaded for a file nothing has put again.
type Info struct {
	Size        int64
	ContentType string
	At          time.Time
}

// UploadKey is a key for a file a browser is about to upload: a random
// name under dir, with the extension the type has, as PutUpload names the
// files that come through the app. The app makes it, never the page: a key
// a browser picked could name another's file.
func UploadKey(dir, contentType string) string {
	return path.Join(dir, strings.ToLower(rand.Text())+filetype.Ext(contentType))
}

// Checked is the file under key as it landed, for the app to see before it
// keeps the key in a row: its size, and the type its bytes say it is. A
// file bigger than max, or whose type isn't one of types, is deleted, and
// the error says which it was; max of 0, or no types, is no limit of that
// kind. d has to be a Stater, as Local and S3 are.
//
// A link holds an upload to what its conditions say, and nothing can hold
// it to what the bytes are: a browser that says image/png can send
// anything. This is where the app finds out.
func Checked(ctx context.Context, d Disk, key string, max int64, types ...string) (Info, error) {
	stater, ok := d.(Stater)
	if !ok {
		return Info{}, fmt.Errorf("storage: this disk can't say what a file is, so one uploaded to it can't be checked: %T", d)
	}
	info, err := stater.Stat(ctx, key)
	if err != nil {
		return Info{}, err
	}
	var wrong error
	switch {
	case max > 0 && info.Size > max:
		wrong = fmt.Errorf("storage: the file uploaded to %s is %d bytes, and %d is the most allowed", key, info.Size, max)
	case len(types) > 0 && !slices.Contains(types, info.ContentType):
		wrong = fmt.Errorf("storage: the file uploaded to %s is %s, and the app takes %s", key, filetype.Names([]string{info.ContentType}), filetype.Names(types))
	}
	if wrong == nil {
		return info, nil
	}
	if err := d.Delete(ctx, key); err != nil {
		return Info{}, fmt.Errorf("%w, and it couldn't be deleted: %w", wrong, err)
	}
	return Info{}, wrong
}
