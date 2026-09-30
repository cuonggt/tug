package auth

import (
	"context"
	"errors"
	"net/http"
	"reflect"
)

// A Gate is what every ability made with it asks first: whether a user
// may do anything, as an admin may, or nothing, as a suspended account.
// A nil Gate, or the zero one, asks nothing first.
type Gate[U any] struct {
	// Before, when it's set, is asked first by every ability made with the
	// gate: true lets the user do what the ability is, false lets the
	// ability say, and a Deny stops them. Another error is a failure, as a
	// query that fails, which the ability returns.
	Before func(ctx context.Context, user U) (bool, error)
}

// An Ability is something a user may do with a thing of type T, or not,
// as edit a post, or see the jobs that failed: NewAbility makes one. Can
// asks it, as for a page to show a button, and Check says no with an
// error for the handler to return, which tug answers with a 403 and why.
// An ability with no thing, as seeing an admin's page, takes a None.
//
// A guest, the zero user, as a nil *User, may do nothing an ability
// names: neither the gate nor the check is asked.
type Ability[U, T any] struct {
	gate  *Gate[U]
	what  string
	check func(ctx context.Context, user U, thing T) (bool, error)
}

// None is the thing of an ability that has none, as seeing an admin's
// page.
type None struct{}

// NewAbility makes the ability what, as "edit this post", which its no
// says, "you may not edit this post", unless its check gives a reason of
// its own. The check says whether user may do it with thing: true or
// false, or false with a Deny that says why not, or an error of its own
// when it can't tell, as a query that fails. g is asked before the check,
// and may be nil. NewAbility panics without what or a check.
func NewAbility[U, T any](g *Gate[U], what string, check func(ctx context.Context, user U, thing T) (bool, error)) *Ability[U, T] {
	if what == "" || check == nil {
		panic(`auth: NewAbility takes what the ability is, as "edit this post", and a check`)
	}
	return &Ability[U, T]{gate: g, what: what, check: check}
}

// Can reports whether user may do what the ability is with thing. Its
// error is a failure's, as a query's the check makes, and never a no.
func (a *Ability[U, T]) Can(ctx context.Context, user U, thing T) (bool, error) {
	err := a.Check(ctx, user, thing)
	if _, no := errors.AsType[*Denial](err); no {
		return false, nil
	}
	return err == nil, err
}

// Check returns nil when user may do what the ability is with thing, or a
// *Denial when they may not, which says why, and which tug answers with a
// 403. Any other error is a failure's, as a query's the check makes.
func (a *Ability[U, T]) Check(ctx context.Context, user U, thing T) error {
	if reflect.ValueOf(&user).Elem().IsZero() {
		return a.no(nil)
	}
	if a.gate != nil && a.gate.Before != nil {
		yes, err := a.gate.Before(ctx, user)
		if err != nil {
			return a.no(err)
		}
		if yes {
			return nil
		}
	}
	yes, err := a.check(ctx, user, thing)
	if err != nil || !yes {
		return a.no(err)
	}
	return nil
}

// no is the ability's no: err, when it's a Denial, in the ability's own
// words when it gives none, and when err is nil; or else err, a failure.
func (a *Ability[U, T]) no(err error) error {
	d, denied := errors.AsType[*Denial](err)
	if err != nil && !denied {
		return err
	}
	if d == nil || d.reason == "" {
		return &Denial{reason: "you may not " + a.what}
	}
	return d
}

// A Denial is an ability's no. Its Error says why, to the person who gets
// it, and its StatusCode, 403, is what tug answers it with.
type Denial struct {
	reason string
}

// Deny returns a no that says why not, as "the post is locked", for an
// ability's check to return with false, or a Gate's Before, to stop a user
// doing anything. An empty reason is the ability's own words.
func Deny(reason string) error {
	return &Denial{reason: reason}
}

func (d *Denial) Error() string { return d.reason }

// StatusCode is 403, Forbidden: the user may not.
func (d *Denial) StatusCode() int { return http.StatusForbidden }
