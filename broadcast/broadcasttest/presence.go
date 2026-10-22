package broadcasttest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cuonggt/tug/broadcast"
)

// TestPresenceStore checks that the stores open returns keep the promises
// a broadcast.Hub depends on of who's on a channel, with a new one for
// each test. A store's own tests call it, as the auth starter's do for its
// database:
//
//	func TestThePresenceKeepsItsPromises(t *testing.T) {
//		broadcasttest.TestPresenceStore(t, func(t *testing.T) broadcast.PresenceStore {
//			return &presences{db: testDB(t)}
//		})
//	}
func TestPresenceStore(t *testing.T, open func(t *testing.T) broadcast.PresenceStore) {
	ctx := context.Background()
	soon := func() time.Time { return time.Now().Add(time.Minute) }
	enter := func(t *testing.T, p broadcast.PresenceStore, channel, id, key string) bool {
		t.Helper()
		arrived, err := p.Enter(ctx, channel, broadcast.Member{ID: id, Data: map[string]string{"name": id}}, key, soon())
		if err != nil {
			t.Fatalf("Enter: %v", err)
		}
		return arrived
	}
	here := func(t *testing.T, p broadcast.PresenceStore, channel string) []string {
		t.Helper()
		members, err := p.Here(ctx, channel)
		if err != nil {
			t.Fatalf("Here: %v", err)
		}
		ids := make([]string, 0, len(members))
		for _, m := range members {
			ids = append(ids, m.ID)
		}
		return ids
	}
	leave := func(t *testing.T, p broadcast.PresenceStore, channel, key string) bool {
		t.Helper()
		gone, err := p.Leave(ctx, channel, key)
		if err != nil {
			t.Fatalf("Leave: %v", err)
		}
		return gone
	}

	t.Run("a member entered is on the channel, and nowhere else", func(t *testing.T) {
		p := open(t)
		if !enter(t, p, "users.1", "ann", "one") {
			t.Error("the first connection of a member isn't an arrival")
		}
		if got := here(t, p, "users.1"); len(got) != 1 || got[0] != "ann" {
			t.Errorf("who's on users.1: %q", got)
		}
		if got := here(t, p, "users.2"); len(got) != 0 {
			t.Errorf("who's on a channel nobody entered: %q", got)
		}
	})

	t.Run("a member's second page is no second arrival, and is there once", func(t *testing.T) {
		p := open(t)
		enter(t, p, "posts", "ann", "one")
		if enter(t, p, "posts", "ann", "other") {
			t.Error("a member's second connection is an arrival")
		}
		if got := here(t, p, "posts"); len(got) != 1 {
			t.Errorf("who's on posts: %q, want ann once", got)
		}
		// Another member of the same channel is an arrival of their own.
		if !enter(t, p, "posts", "bo", "third") {
			t.Error("another member isn't an arrival")
		}
		if got := here(t, p, "posts"); len(got) != 2 {
			t.Errorf("who's on posts: %q, want both", got)
		}
	})

	t.Run("the last page of a member leaving is the member leaving", func(t *testing.T) {
		p := open(t)
		enter(t, p, "posts", "ann", "one")
		enter(t, p, "posts", "ann", "other")
		if leave(t, p, "posts", "one") {
			t.Error("a member with a page left is gone")
		}
		if got := here(t, p, "posts"); len(got) != 1 {
			t.Errorf("who's on posts: %q, want ann still", got)
		}
		if !leave(t, p, "posts", "other") {
			t.Error("a member whose last page went isn't gone")
		}
		if got := here(t, p, "posts"); len(got) != 0 {
			t.Errorf("who's on posts: %q, want nobody", got)
		}
		// A key that isn't there is nobody leaving, and no error.
		if leave(t, p, "posts", "one") {
			t.Error("a key that had left, left again")
		}
	})

	t.Run("a member whose lease has ended isn't here, and arrives again", func(t *testing.T) {
		p := open(t)
		gone, err := p.Enter(ctx, "posts", broadcast.Member{ID: "ann"}, "one", time.Now().Add(-time.Second))
		if err != nil {
			t.Fatalf("Enter: %v", err)
		}
		if !gone {
			t.Error("entering with a lease that has ended isn't an arrival")
		}
		if got := here(t, p, "posts"); len(got) != 0 {
			t.Errorf("who's on posts: %q, want nobody: the lease has ended", got)
		}
		if !enter(t, p, "posts", "ann", "other") {
			t.Error("a member whose lease had ended isn't an arrival again")
		}
	})

	t.Run("a lease said again keeps a member here, and a key that left is left out", func(t *testing.T) {
		p := open(t)
		if _, err := p.Enter(ctx, "posts", broadcast.Member{ID: "ann"}, "one", time.Now().Add(50*time.Millisecond)); err != nil {
			t.Fatalf("Enter: %v", err)
		}
		if err := p.Keep(ctx, []string{"one", "gone"}, soon()); err != nil {
			t.Fatalf("Keep: %v", err)
		}
		time.Sleep(60 * time.Millisecond)
		if got := here(t, p, "posts"); len(got) != 1 {
			t.Errorf("who's on posts: %q, want ann, whose lease was said again", got)
		}
	})

	t.Run("a member's data comes back as the JSON it was kept as", func(t *testing.T) {
		p := open(t)
		enter(t, p, "posts", "ann", "one")
		members, err := p.Here(ctx, "posts")
		if err != nil {
			t.Fatalf("Here: %v", err)
		}
		if len(members) != 1 {
			t.Fatalf("who's on posts: %d members", len(members))
		}
		data, err := json.Marshal(members[0].Data)
		if err != nil {
			t.Fatalf("the member's data isn't JSON: %v", err)
		}
		if string(data) != `{"name":"ann"}` {
			t.Errorf("the member's data is %s", data)
		}
	})

	t.Run("a member with no data of their own is still here", func(t *testing.T) {
		p := open(t)
		if _, err := p.Enter(ctx, "posts", broadcast.Member{ID: "ann"}, "one", soon()); err != nil {
			t.Fatalf("Enter: %v", err)
		}
		if got := here(t, p, "posts"); len(got) != 1 || got[0] != "ann" {
			t.Errorf("who's on posts: %q", got)
		}
	})
}
