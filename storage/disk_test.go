package storage

import (
	"bytes"
	"errors"
	"io/fs"
	"slices"
	"testing"
	"time"
)

// theDisksPromises are what a Disk promises about listing, copying and
// moving: the local disk runs them, and so does an S3 against a MinIO, so
// the two say the same thing or one of them fails here. Each gets a disk
// of its own, so a listing of the whole disk is what the test put on it.
func theDisksPromises(t *testing.T, newDisk func(*testing.T) Disk) {
	hello, other := []byte("hello, files"), []byte("something else")

	t.Run("a listing is the files under a directory", func(t *testing.T) {
		d := newDisk(t)
		put(t, d, "photos/ann.png", hello)
		put(t, d, "photos/deep/bo.png", hello)
		put(t, d, "videos/talk.mp4", hello)
		under := []string{"photos/ann.png", "photos/deep/bo.png"}
		if keys := listed(t, d, "photos"); !slices.Equal(keys, under) {
			t.Errorf("the files under photos: %v, want %v", keys, under)
		}
		// A slash at the end names the same directory.
		if keys := listed(t, d, "photos/"); !slices.Equal(keys, under) {
			t.Errorf("the files under photos/: %v, want %v", keys, under)
		}
		all := []string{"photos/ann.png", "photos/deep/bo.png", "videos/talk.mp4"}
		if keys := listed(t, d, ""); !slices.Equal(keys, all) {
			t.Errorf("the files on the disk: %v, want %v", keys, all)
		}
	})

	t.Run("a listing says how big a file is and when it changed", func(t *testing.T) {
		d := newDisk(t)
		put(t, d, "photos/ann.png", hello)
		var files []Entry
		for e, err := range d.List(ctx, "photos") {
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, e)
		}
		if len(files) != 1 {
			t.Fatalf("the files under photos: %v", files)
		}
		if files[0].Size != int64(len(hello)) {
			t.Errorf("%s is %d bytes, want %d", files[0].Key, files[0].Size, len(hello))
		}
		// The disk's clock is its own, and a bucket's is its service's.
		if since := time.Since(files[0].At); since < -time.Minute || since > 5*time.Minute {
			t.Errorf("%s last changed %s, which is %s ago", files[0].Key, files[0].At, since)
		}
	})

	t.Run("a prefix names a directory and not the start of a key", func(t *testing.T) {
		d := newDisk(t)
		put(t, d, "photos/ann.png", hello)
		for _, prefix := range []string{"photo", "photos/ann", "photos/ann.png", "nothing/here"} {
			if keys := listed(t, d, prefix); len(keys) != 0 {
				t.Errorf("the files under %q: %v, want none", prefix, keys)
			}
		}
	})

	t.Run("a listing stops where the range does", func(t *testing.T) {
		d := newDisk(t)
		for _, key := range []string{"photos/a.png", "photos/b.png", "photos/c.png"} {
			put(t, d, key, hello)
		}
		read := 0
		for _, err := range d.List(ctx, "photos") {
			if err != nil {
				t.Fatal(err)
			}
			read++
			break
		}
		if read != 1 {
			t.Errorf("a listing broken out of after one file read %d", read)
		}
	})

	t.Run("a copy leaves the file where it was", func(t *testing.T) {
		d := newDisk(t)
		put(t, d, "photos/ann.png", hello)
		if err := d.Copy(ctx, "photos/ann.png", "copies/ann.png"); err != nil {
			t.Fatal(err)
		}
		if got := read(t, d, "copies/ann.png"); !bytes.Equal(got, hello) {
			t.Errorf("the copy is %q", got)
		}
		if got := read(t, d, "photos/ann.png"); !bytes.Equal(got, hello) {
			t.Errorf("the file that was copied is %q", got)
		}
	})

	t.Run("a move takes the file with it", func(t *testing.T) {
		d := newDisk(t)
		put(t, d, "uploads/ann.png", hello)
		if err := d.Move(ctx, "uploads/ann.png", "photos/ann.png"); err != nil {
			t.Fatal(err)
		}
		if got := read(t, d, "photos/ann.png"); !bytes.Equal(got, hello) {
			t.Errorf("the file where it was moved to is %q", got)
		}
		if _, err := d.Open(ctx, "uploads/ann.png"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the file it was moved from: %v, want it gone", err)
		}
	})

	t.Run("a copy and a move go over the file at the key", func(t *testing.T) {
		d := newDisk(t)
		put(t, d, "photos/ann.png", hello)
		put(t, d, "photos/bo.png", other)
		if err := d.Copy(ctx, "photos/ann.png", "photos/bo.png"); err != nil {
			t.Fatal(err)
		}
		if got := read(t, d, "photos/bo.png"); !bytes.Equal(got, hello) {
			t.Errorf("the key copied over holds %q", got)
		}
		put(t, d, "photos/cass.png", other)
		if err := d.Move(ctx, "photos/bo.png", "photos/cass.png"); err != nil {
			t.Fatal(err)
		}
		if got := read(t, d, "photos/cass.png"); !bytes.Equal(got, hello) {
			t.Errorf("the key moved over holds %q", got)
		}
	})

	t.Run("there's no file to copy or move from a key that has none", func(t *testing.T) {
		d := newDisk(t)
		if err := d.Copy(ctx, "photos/gone.png", "copies/gone.png"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("copying a file that isn't there: %v", err)
		}
		if err := d.Move(ctx, "photos/gone.png", "photos/here.png"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("moving a file that isn't there: %v", err)
		}
		// Neither left anything behind where it was going.
		if _, err := d.Open(ctx, "copies/gone.png"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the key a failed copy was going to: %v", err)
		}
	})

	t.Run("a key copied or moved onto itself is left alone", func(t *testing.T) {
		d := newDisk(t)
		put(t, d, "photos/ann.png", hello)
		if err := d.Copy(ctx, "photos/ann.png", "photos/ann.png"); err != nil {
			t.Errorf("copying a key onto itself: %v", err)
		}
		if err := d.Move(ctx, "photos/ann.png", "photos/ann.png"); err != nil {
			t.Errorf("moving a key onto itself: %v", err)
		}
		if got := read(t, d, "photos/ann.png"); !bytes.Equal(got, hello) {
			t.Errorf("the file after both is %q", got)
		}
	})

	t.Run("a key or a prefix that isn't one is refused", func(t *testing.T) {
		d := newDisk(t)
		put(t, d, "photos/ann.png", hello)
		if err := d.Copy(ctx, "../etc/passwd", "copies/ann.png"); !errors.Is(err, ErrKey) {
			t.Errorf("copying from a key that isn't one: %v", err)
		}
		if err := d.Copy(ctx, "photos/ann.png", "../etc/passwd"); !errors.Is(err, ErrKey) {
			t.Errorf("copying to a key that isn't one: %v", err)
		}
		if err := d.Move(ctx, "photos/ann.png", "/etc/passwd"); !errors.Is(err, ErrKey) {
			t.Errorf("moving to a key that isn't one: %v", err)
		}
		said := 0
		for _, err := range d.List(ctx, "../etc") {
			said++
			if !errors.Is(err, ErrKey) {
				t.Errorf("listing a prefix that isn't one: %v", err)
			}
			break
		}
		if said == 0 {
			t.Error("listing a prefix that isn't one gave nothing, and said nothing")
		}
	})
}

// listed is the keys a listing gives, in order, as a disk promises none.
func listed(t *testing.T, d Disk, prefix string) []string {
	t.Helper()
	var keys []string
	for e, err := range d.List(ctx, prefix) {
		if err != nil {
			t.Fatalf("listing %q: %v", prefix, err)
		}
		keys = append(keys, e.Key)
	}
	slices.Sort(keys)
	return keys
}
