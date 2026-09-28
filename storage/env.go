package storage

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// FromEnv returns the disk the environment names, by the variables
// Laravel's filesystem reads, and otherwise local. FILESYSTEM_DISK=s3
// names an S3: the bucket AWS_BUCKET, in AWS_DEFAULT_REGION, us-east-1
// unless set, with the keys AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY,
// and AWS_SESSION_TOKEN for temporary ones. A service other than AWS takes
// AWS_ENDPOINT, such as https://<account>.r2.cloudflarestorage.com, and
// AWS_USE_PATH_STYLE_ENDPOINT=true when it wants the bucket in the path,
// as MinIO does. A public disk's links start with AWS_URL when it's set.
// FILESYSTEM_DISK=local, or none, is local.
//
// Whether the files are public is the app's to say, not the environment's:
// the S3 is public when local is.
func FromEnv(local *Local) (Disk, error) {
	switch disk := os.Getenv("FILESYSTEM_DISK"); disk {
	case "", "local":
		if local == nil {
			return nil, errors.New("storage: FILESYSTEM_DISK is local, or isn't set, and FromEnv was given no local disk")
		}
		return local, nil
	case "s3":
		return s3FromEnv(local != nil && local.Public)
	default:
		return nil, fmt.Errorf("storage: FILESYSTEM_DISK is %q, and a disk is local or s3", disk)
	}
}

func s3FromEnv(public bool) (*S3, error) {
	s := &S3{
		Bucket:          os.Getenv("AWS_BUCKET"),
		Region:          os.Getenv("AWS_DEFAULT_REGION"),
		Endpoint:        os.Getenv("AWS_ENDPOINT"),
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
		Public:          public,
		BaseURL:         os.Getenv("AWS_URL"),
	}
	var missing []string
	for _, v := range []struct{ name, value string }{
		{"AWS_BUCKET", s.Bucket}, {"AWS_ACCESS_KEY_ID", s.AccessKeyID}, {"AWS_SECRET_ACCESS_KEY", s.SecretAccessKey},
	} {
		if v.value == "" {
			missing = append(missing, v.name)
		}
	}
	switch len(missing) {
	case 0:
	case 1:
		return nil, fmt.Errorf("storage: FILESYSTEM_DISK is s3, and %s isn't set", missing[0])
	default:
		return nil, fmt.Errorf("storage: FILESYSTEM_DISK is s3, and %s aren't set", strings.Join(missing, " and "))
	}
	if v := os.Getenv("AWS_USE_PATH_STYLE_ENDPOINT"); v != "" {
		pathStyle, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("storage: AWS_USE_PATH_STYLE_ENDPOINT is %q, which isn't true or false", v)
		}
		s.PathStyle = pathStyle
	}
	if _, err := s.objectURL("check"); err != nil {
		return nil, fmt.Errorf("storage: AWS_ENDPOINT is %q, which isn't an http:// or https:// address", s.Endpoint)
	}
	return s, nil
}
