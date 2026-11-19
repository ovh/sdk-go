// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

package ovh

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"
)

// Many operations only start the work: a v2 resource is created or updated in the
// background, its resourceStatus moving from CREATING or UPDATING to READY; a v1 operation
// returns a task whose status moves from todo to done. The Wait functions call a GET until the
// work is over:
//
//	inst, err := ovh.WaitReady(ctx, func(ctx context.Context) (*publiccloud.PublicCloudInstanceInstance, error) {
//		return pc.GetInstance(ctx, projectID, id)
//	})
//
// They poll every 2 seconds at first, then less often, up to every 30 seconds, and give up after
// 30 minutes; WaitPoll and WaitTimeout change that. A response of 429, 500, 502, 503 or 504 is
// polled again; any other error stops the wait. Every Wait function returns the last value it
// read, with the error.

// ErrWaitFailed is matched, with errors.Is, by the error of a wait that ended in a failure:
// a resourceStatus of ERROR, a task in error or cancelled, or one waiting for an action.
var ErrWaitFailed = errors.New("ovh: the operation failed")

// WaitOption changes how a Wait function polls.
type WaitOption func(*waitConfig)

type waitConfig struct {
	first, max, timeout time.Duration
}

// WaitPoll sets the first interval between two polls, and the longest one: the interval grows
// by half at each poll, from first to max.
func WaitPoll(first, max time.Duration) WaitOption {
	return func(c *waitConfig) {
		c.first, c.max = first, max
	}
}

// WaitTimeout sets how long to wait at most, 30 minutes by default. 0 waits as long as ctx
// allows.
func WaitTimeout(d time.Duration) WaitOption {
	return func(c *waitConfig) {
		c.timeout = d
	}
}

// Wait calls get, then done with what it returned, until done reports true, done or get
// returns an error, or the timeout or ctx ends. It returns get's last value.
//
//	order, err := ovh.Wait(ctx, getOrder, func(o *me.Order) (bool, error) {
//		return ovh.Value(o.Status) == "delivered", nil
//	})
func Wait[T any](ctx context.Context, get func(context.Context) (T, error), done func(T) (bool, error), opts ...WaitOption) (T, error) {
	cfg := waitConfig{first: 2 * time.Second, max: 30 * time.Second, timeout: 30 * time.Minute}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.first <= 0 {
		cfg.first = time.Second
	}
	if cfg.max < cfg.first {
		cfg.max = cfg.first
	}
	if cfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.timeout)
		defer cancel()
	}

	start := time.Now()
	interval := cfg.first
	var last T
	var state string
	for {
		v, err := get(ctx)
		switch {
		case err == nil:
			last = v
			finished, err := done(v)
			if err != nil || finished {
				return last, err
			}
			state = describe(v)
		case transient(err):
			state = err.Error()
		default:
			return last, err
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last, fmt.Errorf("ovh: gave up waiting after %s, %s: %w",
				time.Since(start).Round(time.Second), state, ctx.Err())
		case <-timer.C:
		}
		interval = min(interval+interval/2, cfg.max)
	}
}

// WaitReady waits for a v2 resource: until its ResourceStatus is READY. ERROR, SUSPENDED,
// DELETED and REMOVED end the wait with an error matching ErrWaitFailed. T must have a
// ResourceStatus field, as the v2 resources of the generated packages have.
func WaitReady[T any](ctx context.Context, get func(context.Context) (T, error), opts ...WaitOption) (T, error) {
	return Wait(ctx, get, func(v T) (bool, error) {
		status, err := statusField(v, "ResourceStatus")
		if err != nil {
			return false, err
		}
		switch status {
		case "READY":
			return true, nil
		case "ERROR", "SUSPENDED", "DELETED", "REMOVED":
			return false, fmt.Errorf("%w: resourceStatus is %s", ErrWaitFailed, status)
		}
		return false, nil
	}, opts...)
}

// Task statuses across the API: done in either case is success; these end the wait with an
// error matching ErrWaitFailed. Any other status (todo, doing, init, PENDING, RUNNING...) is
// polled again.
var (
	taskFailed = map[string]bool{"error": true, "ERROR": true, "cancelled": true, "canceled": true,
		"customerError": true, "ovhError": true, "problem": true, "FAILED": true, "CANCELLED": true}
	// The task waits for someone: polling will not move it on.
	taskBlocked = map[string]bool{"WAITING_USER_INPUT": true, "pause": true, "PAUSED": true, "standby": true}
)

// WaitTask waits for a task: until its Status is done (or DONE). A task in error, cancelled,
// or waiting for an action (WAITING_USER_INPUT, pause, standby) ends the wait with an error
// matching ErrWaitFailed. T must have a Status field, as the tasks of the generated packages
// have.
//
//	task, err := ds.Reboot(ctx, server)
//	...
//	task, err = ovh.WaitTask(ctx, func(ctx context.Context) (*dedicatedserver.DedicatedServerTask, error) {
//		return ds.GetTask(ctx, server, ovh.Value(task.TaskID))
//	})
func WaitTask[T any](ctx context.Context, get func(context.Context) (T, error), opts ...WaitOption) (T, error) {
	return Wait(ctx, get, func(v T) (bool, error) {
		status, err := statusField(v, "Status")
		if err != nil {
			return false, err
		}
		switch {
		case status == "done" || status == "DONE":
			return true, nil
		case taskFailed[status]:
			return false, fmt.Errorf("%w: the task's status is %s", ErrWaitFailed, status)
		case taskBlocked[status]:
			return false, fmt.Errorf("%w: the task's status is %s: it waits for an action", ErrWaitFailed, status)
		}
		return false, nil
	}, opts...)
}

// WaitGone waits for a deletion: until get answers 404 Not Found.
//
//	err := ovh.WaitGone(ctx, func(ctx context.Context) (*publiccloud.PublicCloudInstanceInstance, error) {
//		return pc.GetInstance(ctx, projectID, id)
//	})
func WaitGone[T any](ctx context.Context, get func(context.Context) (T, error), opts ...WaitOption) error {
	_, err := Wait(ctx, func(ctx context.Context) (bool, error) {
		_, err := get(ctx)
		if errors.Is(err, ErrNotFound) {
			return true, nil
		}
		return false, err
	}, func(gone bool) (bool, error) { return gone, nil }, opts...)
	return err
}

// transient reports whether a poll that failed is worth repeating.
func transient(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// statusField reads a status field of a generated type, a string or a pointer to one ("" when
// nil), through any pointers to the struct.
func statusField(v any, name string) (string, error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return "", fmt.Errorf("ovh: waiting on a nil %T", v)
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return "", fmt.Errorf("ovh: cannot wait on a %T: not a struct with a %s field", v, name)
	}
	f := rv.FieldByName(name)
	if !f.IsValid() {
		return "", fmt.Errorf("ovh: cannot wait on a %T: it has no %s field", v, name)
	}
	for f.Kind() == reflect.Pointer {
		if f.IsNil() {
			return "", nil
		}
		f = f.Elem()
	}
	if f.Kind() != reflect.String {
		return "", fmt.Errorf("ovh: cannot wait on a %T: its %s is a %s, not a string", v, name, f.Type())
	}
	return f.String(), nil
}

// describe says where a wait stands, for the error when it gives up.
func describe(v any) string {
	for _, name := range []string{"ResourceStatus", "Status"} {
		if s, err := statusField(v, name); err == nil && s != "" {
			return "last status " + s
		}
	}
	return "not done yet"
}
