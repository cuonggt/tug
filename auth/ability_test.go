package auth

import (
	"context"
	"errors"
	"testing"
)

type member struct {
	ID        int64
	Admin     bool
	Suspended bool
}

type post struct {
	Author int64
	Locked bool
}

// members is asked first: admins may do anything, and a suspended account
// nothing.
var members = &Gate[*member]{Before: func(_ context.Context, m *member) (bool, error) {
	if m.Suspended {
		return false, Deny("your account is suspended")
	}
	return m.Admin, nil
}}

var editPost = NewAbility(members, "edit this post", func(_ context.Context, m *member, p *post) (bool, error) {
	if p.Locked {
		return false, Deny("the post is locked")
	}
	return p.Author == m.ID, nil
})

func TestAnAbilitySaysYesOrNoAndWhyNot(t *testing.T) {
	ctx := context.Background()
	ann, bob := &member{ID: 1}, &member{ID: 2}
	annsPost := &post{Author: 1}
	if err := editPost.Check(ctx, ann, annsPost); err != nil {
		t.Errorf("Ann may not edit her own post: %v", err)
	}
	if can, err := editPost.Can(ctx, ann, annsPost); !can || err != nil {
		t.Errorf("Can says Ann may edit her own post: %v, %v", can, err)
	}
	for name, no := range map[string]struct {
		who  *member
		post *post
		why  string
	}{
		"someone else's post":  {bob, annsPost, "you may not edit this post"},
		"a post that's locked": {ann, &post{Author: 1, Locked: true}, "the post is locked"},
	} {
		err := editPost.Check(ctx, no.who, no.post)
		if d, ok := errors.AsType[*Denial](err); !ok || d.Error() != no.why || d.StatusCode() != 403 {
			t.Errorf("%s: %v, want a 403 that says %q", name, err, no.why)
		}
		if can, err := editPost.Can(ctx, no.who, no.post); can || err != nil {
			t.Errorf("%s: Can says %v, %v", name, can, err)
		}
	}
}

func TestADenyWithNoReasonIsTheAbilitysOwnWords(t *testing.T) {
	archive := NewAbility(nil, "archive this post", func(context.Context, *member, *post) (bool, error) {
		return false, Deny("")
	})
	if err := archive.Check(context.Background(), &member{ID: 1}, &post{}); err == nil || err.Error() != "you may not archive this post" {
		t.Errorf("got %v", err)
	}
}

func TestTheGateIsAskedBeforeEveryAbility(t *testing.T) {
	ctx := context.Background()
	someonesPost := &post{Author: 9, Locked: true}
	if err := editPost.Check(ctx, &member{ID: 1, Admin: true}, someonesPost); err != nil {
		t.Errorf("an admin may not edit a post: %v", err)
	}
	if err := editPost.Check(ctx, &member{ID: 9, Suspended: true}, &post{Author: 9}); err == nil || err.Error() != "your account is suspended" {
		t.Errorf("a suspended author: %v, want the gate's no", err)
	}
}

func TestAGuestMayDoNothingAnAbilityNames(t *testing.T) {
	asked := false
	gate := &Gate[*member]{Before: func(context.Context, *member) (bool, error) {
		asked = true
		return true, nil
	}}
	read := NewAbility(gate, "read this post", func(_ context.Context, m *member, _ *post) (bool, error) {
		asked = true
		return m.ID != 0, nil // which a nil member would panic on
	})
	err := read.Check(context.Background(), nil, &post{})
	if asked || err == nil || err.Error() != "you may not read this post" {
		t.Errorf("a guest: %v, and the gate or the check was asked: %v", err, asked)
	}
}

func TestAFailureIsntANo(t *testing.T) {
	ctx := context.Background()
	down := errors.New("the database is down")
	report := NewAbility(nil, "see the report", func(context.Context, *member, None) (bool, error) {
		return false, down
	})
	if err := report.Check(ctx, &member{ID: 1}, None{}); !errors.Is(err, down) {
		t.Errorf("Check returned %v, want the check's failure", err)
	}
	if can, err := report.Can(ctx, &member{ID: 1}, None{}); can || !errors.Is(err, down) {
		t.Errorf("Can returned %v, %v, want the check's failure", can, err)
	}
	gate := &Gate[*member]{Before: func(context.Context, *member) (bool, error) { return false, down }}
	anyone := NewAbility(gate, "see the report", func(context.Context, *member, None) (bool, error) { return true, nil })
	if err := anyone.Check(ctx, &member{ID: 1}, None{}); !errors.Is(err, down) {
		t.Errorf("Check returned %v, want the gate's failure", err)
	}
}

func TestNewAbilityPanicsWithoutWhatOrACheck(t *testing.T) {
	for name, newAbility := range map[string]func(){
		"no what":  func() { NewAbility(nil, "", func(context.Context, *member, None) (bool, error) { return true, nil }) },
		"no check": func() { NewAbility[*member, None](nil, "see the report", nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s didn't panic", name)
				}
			}()
			newAbility()
		}()
	}
}
