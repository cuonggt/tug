package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// A Carrier is what a job takes from the context it's pushed from, and
// gives back to the context it runs in: the ID of the request that pushed
// it, say, or a trace's parent. Carry puts what it takes from ctx in into,
// by name, and Restore returns ctx with what from has of it. It's the
// shape of OpenTelemetry's propagators, with a map for their carrier, as
// propagation.MapCarrier is, so one of them adapts in a few lines.
//
// Config.Carry lists the queue's. package middleware's CarryRequestID is
// one, of the ID RequestID gives a request.
type Carrier interface {
	Carry(ctx context.Context, into map[string]string)
	Restore(ctx context.Context, from map[string]string) context.Context
}

// maxCarried is the most a job carries, as JSON: a few values, as a
// request's headers have.
const maxCarried = 4 << 10

// carried is what q's Carriers take from ctx for a job pushed to s: nil
// when s doesn't keep it, a CarryStore's extra, or there's nothing to
// take, and an error when it's more than maxCarried.
func (q *Queue) carried(ctx context.Context, s Store) (map[string]string, error) {
	if len(q.carry) == 0 {
		return nil, nil
	}
	if _, ok := s.(CarryStore); !ok {
		return nil, nil
	}
	into := map[string]string{}
	for _, c := range q.carry {
		c.Carry(ctx, into)
	}
	if len(into) == 0 {
		return nil, nil
	}
	if data, err := json.Marshal(into); err != nil || len(data) > maxCarried {
		return nil, fmt.Errorf("it carries %d bytes from its context, past the %d a job may", len(data), maxCarried)
	}
	return into, nil
}

// restore is ctx with what j carried given back by q's Carriers.
func (q *Queue) restore(ctx context.Context, j *Job) context.Context {
	if len(j.Carried) == 0 {
		return ctx
	}
	for _, c := range q.carry {
		ctx = c.Restore(ctx, j.Carried)
	}
	return ctx
}

// about is what a log line says of j: its kind and ID, then attrs, and
// what it carried, by name, as the ID of the request that pushed it.
func about(j *Job, attrs ...any) []any {
	out := append([]any{"kind", j.Kind, "job", j.ID}, attrs...)
	for _, name := range slices.Sorted(maps.Keys(j.Carried)) {
		out = append(out, name, j.Carried[name])
	}
	return out
}
