package devtools

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cuonggt/tug/internal/ulid"
)

// Of each tab's entries, and of those of the requests with none, as from a
// browser without the extension, the newest keepPerTab are kept, for
// keepFor at most, which a prune every pruneEvery deletes after.
const (
	keepPerTab = 100
	keepFor    = 24 * time.Hour
	pruneEvery = 5 * time.Minute
)

// store keeps the entries in files, one each, named by its ID, in dir, as
// tug dev builds the app again at each change, which a run's memory would
// forget. An index of their metas, read from the files at the first use,
// answers the list.
type store struct {
	dir string
	now func() time.Time

	mu     sync.Mutex
	index  map[string]meta // nil until it's read from the files
	pruned time.Time
}

// save keeps e, and the newest of its tab's alone.
func (st *store) save(e entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(st.dir, 0o700); err != nil {
		return err
	}
	// A whole file or none: written aside, then renamed into place.
	tmp, err := os.CreateTemp(st.dir, ".entry-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), st.path(e.Meta.ID))
	}
	if err != nil {
		os.Remove(tmp.Name())
		return err
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	st.read()
	st.index[e.Meta.ID] = e.Meta
	st.limit(e.Meta.TabUUID)
	if now := st.now(); now.Sub(st.pruned) >= pruneEvery {
		st.prune(now)
		st.pruned = now
	}
	return nil
}

// get returns the entry with id, as JSON, and whether there is one.
func (st *store) get(id string) ([]byte, bool) {
	if !ulid.Valid(id) {
		return nil, false
	}
	data, err := os.ReadFile(st.path(id))
	return data, err == nil
}

// list returns the metas of the entries, the newest first.
func (st *store) list() []meta {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.read()
	metas := make([]meta, 0, len(st.index))
	for _, m := range st.index {
		metas = append(metas, m)
	}
	slices.SortFunc(metas, func(a, b meta) int { return strings.Compare(b.ID, a.ID) })
	return metas
}

func (st *store) path(id string) string {
	return filepath.Join(st.dir, id+".json")
}

// read fills the index from the files, once. st.mu is held.
func (st *store) read() {
	if st.index != nil {
		return
	}
	st.index = map[string]meta{}
	files, err := os.ReadDir(st.dir)
	if err != nil {
		return
	}
	for _, f := range files {
		id, ok := strings.CutSuffix(f.Name(), ".json")
		if !ok || !ulid.Valid(id) {
			continue
		}
		data, err := os.ReadFile(st.path(id))
		if err != nil {
			continue
		}
		var e struct {
			Meta meta `json:"__meta"`
		}
		if json.Unmarshal(data, &e) == nil && e.Meta.ID == id {
			st.index[id] = e.Meta
		}
	}
}

// limit deletes the entries of tab past the newest keepPerTab. st.mu is
// held.
func (st *store) limit(tab *string) {
	var ids []string
	for id, m := range st.index {
		if sameTab(m.TabUUID, tab) {
			ids = append(ids, id)
		}
	}
	if len(ids) <= keepPerTab {
		return
	}
	slices.Sort(ids)
	st.delete(ids[:len(ids)-keepPerTab])
}

// prune deletes the entries kept longer than keepFor. st.mu is held.
func (st *store) prune(now time.Time) {
	cutoff := float64(now.Add(-keepFor).UnixMicro()) / 1e6
	var old []string
	for id, m := range st.index {
		if m.Utime < cutoff {
			old = append(old, id)
		}
	}
	st.delete(old)
}

// delete deletes the entries with ids, and their files. st.mu is held.
func (st *store) delete(ids []string) {
	for _, id := range ids {
		if err := os.Remove(st.path(id)); err == nil || errors.Is(err, fs.ErrNotExist) {
			delete(st.index, id)
		}
	}
}

func sameTab(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
