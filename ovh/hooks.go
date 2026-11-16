// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

package ovh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// CallInfo describes one API call to the request hooks. It never holds the Authorization
// header or the token.
type CallInfo struct {
	// Operation is the operation called. It is nil when the registry does not know the route:
	// a Client.Do call to a package the program does not import.
	Operation *Operation
	// Method is the HTTP method, e.g. http.MethodPost.
	Method string
	// Path is the path with its parameters filled in: "/v1/vrack/pn-1/cloudProject".
	Path string
	// Query is the full query string the call sends: Request.Query, the query parameters of
	// Request.Params, and any query written in Path. For a call refused before it was encoded
	// (by the policy, or because it is malformed), it is Request.Query only.
	Query url.Values
	// Body is the request body as the caller gave it, before JSON encoding. It is nil when the
	// call has no body.
	Body any
}

// CallResult is what an after hook learns about how a call ended.
type CallResult struct {
	// StatusCode is the HTTP status of the last response, or 0 when there was none: the call
	// was refused before it was sent, or the network failed.
	StatusCode int
	// QueryID is the X-Ovh-Queryid header of the last response, which identifies the request
	// to OVHcloud support. It is empty when there was no response.
	QueryID string
	// Err is the error the caller gets, nil on success.
	Err error
	// Duration is the time the whole call took, retries included.
	Duration time.Duration
	// Attempts is the number of times the request was sent: 0 when it was not sent, more than
	// 1 when it was retried (see WithRetry).
	Attempts int
}

// BeforeRequestHook runs before a call is sent. A non-nil error stops the call: nothing is sent,
// and the caller gets an error that wraps this one. The hook must not change call.
//
// A Client is shared, so calls made at the same time run the same hook on different
// goroutines at the same time: a hook must be safe for concurrent use.
type BeforeRequestHook func(ctx context.Context, call *CallInfo) error

// AfterRequestHook runs once a call is over, whatever the outcome. The hook must not change
// call. Like a BeforeRequestHook, it must be safe for concurrent use.
//
// If the call panics (in a before hook, for example), the after hooks still run, with an Err
// that says so, and then the panic goes on.
type AfterRequestHook func(ctx context.Context, call *CallInfo, result CallResult)

// WithBeforeRequest adds a hook that runs before each call, to log it, or to veto it by
// returning an error. It runs once per call, before the first attempt: retries do not run it
// again. A call refused by WithReadOnly or WithAllowedOperations, or malformed, never reaches
// it. Several hooks run in the order they were added; the first error stops the call.
//
// Each page of a paginated list (the AllX iterators) is a separate call. The hook must be safe
// for concurrent use: calls made at the same time run it at the same time.
func WithBeforeRequest(hook BeforeRequestHook) Option {
	return func(c *config) error {
		if hook == nil {
			return errors.New("ovh: WithBeforeRequest: nil hook")
		}
		c.client.beforeRequest = append(c.client.beforeRequest, hook)
		return nil
	}
}

// WithAfterRequest adds a hook that runs after each call, once its retries are over. It sees
// every call, also those refused by the policy, by a before hook, or by WithDryRun, so it can
// keep a full audit log. Several hooks run in the order they were added.
//
// Each page of a paginated list (the AllX iterators) is a separate call. The hook must be safe
// for concurrent use: calls made at the same time run it at the same time.
func WithAfterRequest(hook AfterRequestHook) Option {
	return func(c *config) error {
		if hook == nil {
			return errors.New("ovh: WithAfterRequest: nil hook")
		}
		c.client.afterRequest = append(c.client.afterRequest, hook)
		return nil
	}
}

// WithDryRun lets a program, or an AI agent, plan changes without making them. Calls that only
// read (GET and HEAD) are sent as usual, so the program sees the real state. Any other call is
// not sent: it returns a *DryRunError, which matches ErrDryRun, with the URL and the body it
// would have sent. No token is fetched for it.
//
// The policy still applies first: with WithReadOnly or WithAllowedOperations, a refused call
// stays a *NotAllowedError.
func WithDryRun() Option {
	return func(c *config) error {
		c.client.dryRun = true
		return nil
	}
}

// ErrDryRun is matched, with errors.Is, by the *DryRunError of a call WithDryRun did not send.
var ErrDryRun = errors.New("ovh: not sent: dry run")

// DryRunError is returned for a call that WithDryRun did not send. It holds what the call would
// have sent, except the credentials: no token is fetched, and the Authorization header (with
// the other credential headers, such as Cookie) is left out.
//
//	var dry *ovh.DryRunError
//	if errors.As(err, &dry) {
//		fmt.Println(dry.Method, dry.URL, string(dry.Body))
//	}
type DryRunError struct {
	// Operation is the operation not called. It is nil when the registry does not know the
	// route.
	Operation *Operation
	Method    string
	// URL is the full URL: the endpoint, the path and the encoded query.
	URL string
	// Header holds the headers: Accept, Content-Type, User-Agent, those of WithHeader, of
	// Request.Header and of the header fields of Request.Params.
	Header http.Header
	// Body is the JSON body, nil when the call has none.
	Body json.RawMessage
}

func (e *DryRunError) Error() string {
	return fmt.Sprintf("ovh: %s (%s %s) not sent: dry run", callName(e.Operation), e.Method, e.URL)
}

// Is makes errors.Is(err, ovh.ErrDryRun) true.
func (e *DryRunError) Is(target error) bool {
	return target == ErrDryRun
}

// callName names a call in an error message: its operation, or "request" when the registry
// does not know it.
func callName(op *Operation) string {
	if op == nil {
		return "request"
	}
	return op.Name
}
