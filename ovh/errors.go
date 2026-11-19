// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

package ovh

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// APIError is returned for any response outside 2xx. Its message is the API's own, so
// printing the error is usually enough to see what went wrong. It names the operation, and on
// a 401 or 403 the IAM actions the call needs:
//
//	me.GetMe: GET /v1/me: 403 Forbidden: ... (Client::Forbidden); requires the IAM action account:apiovh:me/get
//
// Match a common status with errors.Is:
//
//	if errors.Is(err, ovh.ErrNotFound) { ... }
//
// or use errors.As to read the status code, the message or the query ID:
//
//	var apiErr *ovh.APIError
//	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict { ... }
type APIError struct {
	// Operation is the operation's name, "me.GetMe", when the request was one the registry
	// knows: a generated method's, or one Client.Do matched (see [MatchOperation]).
	Operation string
	// IAMActions are the IAM actions the operation needs: on a 403, the caller's policy lacks
	// one of them.
	IAMActions []string
	Method     string
	Path       string
	StatusCode int
	Status     string
	// Class and Message come from the API's error body, when it has the usual shape
	// {"class": "...", "message": "..."}.
	Class   string
	Message string
	// QueryID identifies the request to OVHcloud support (the X-Ovh-Queryid header).
	QueryID string
	// SchemasVersion is the X-Schemas-Version the API answered with (/v2 routes only), or "".
	SchemasVersion string
	// Body is the raw response body.
	Body []byte
}

// An *APIError matches these with errors.Is when it has their status code. They are never
// returned on their own.
var (
	ErrBadRequest      = errors.New("ovh: 400 Bad Request")
	ErrUnauthorized    = errors.New("ovh: 401 Unauthorized")
	ErrForbidden       = errors.New("ovh: 403 Forbidden")
	ErrNotFound        = errors.New("ovh: 404 Not Found")
	ErrConflict        = errors.New("ovh: 409 Conflict")
	ErrTooManyRequests = errors.New("ovh: 429 Too Many Requests")
)

// A slice, not a map: errors.Is passes any target to Is, and looking up a target whose type
// cannot be hashed (a slice of errors, say) in a map would panic.
var statusErrors = []struct {
	err  error
	code int
}{
	{ErrBadRequest, http.StatusBadRequest},
	{ErrUnauthorized, http.StatusUnauthorized},
	{ErrForbidden, http.StatusForbidden},
	{ErrNotFound, http.StatusNotFound},
	{ErrConflict, http.StatusConflict},
	{ErrTooManyRequests, http.StatusTooManyRequests},
}

// Is reports whether target is the sentinel error of e's status code, such as ErrNotFound for
// a 404. It is what makes errors.Is(err, ovh.ErrNotFound) work.
func (e *APIError) Is(target error) bool {
	for _, s := range statusErrors {
		if target == s.err {
			return e.StatusCode == s.code
		}
	}
	return false
}

func newAPIError(op *Operation, req *Request, resp *http.Response, body []byte) *APIError {
	e := &APIError{
		Method:         req.Method,
		Path:           req.Path,
		StatusCode:     resp.StatusCode,
		Status:         resp.Status,
		QueryID:        resp.Header.Get("X-Ovh-Queryid"),
		SchemasVersion: resp.Header.Get(schemasVersionHeader),
		Body:           body,
	}
	var payload struct {
		Class   string `json:"class"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil {
		e.Class, e.Message = payload.Class, payload.Message
	}
	if op != nil {
		e.Operation, e.IAMActions = op.Name, op.IAMActions
	}
	return e
}

func (e *APIError) Error() string {
	var b strings.Builder
	if e.Operation != "" {
		fmt.Fprintf(&b, "%s: ", e.Operation)
	}
	fmt.Fprintf(&b, "%s %s: %s", e.Method, e.Path, e.Status)
	switch {
	case e.Message != "":
		fmt.Fprintf(&b, ": %s", e.Message)
	case len(e.Body) > 0:
		fmt.Fprintf(&b, ": %s", strings.TrimSpace(string(e.Body)))
	}
	if e.Class != "" {
		fmt.Fprintf(&b, " (%s)", e.Class)
	}
	if len(e.IAMActions) > 0 && (e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden) {
		s := ""
		if len(e.IAMActions) > 1 {
			s = "s"
		}
		fmt.Fprintf(&b, "; requires the IAM action%s %s", s, strings.Join(e.IAMActions, ", "))
	}
	if e.StatusCode == http.StatusBadRequest && strings.Contains(e.Message, "Unknown version") {
		b.WriteString("; this SDK release targets an X-Schemas-Version the API no longer serves: upgrade the SDK")
	}
	return b.String()
}

// ErrInvalidRequest is matched, with errors.Is, by the error of a request refused before it is
// sent because it is malformed: an empty path parameter, a path not starting with "/".
var ErrInvalidRequest = errors.New("ovh: invalid request")

// ErrNotAllowed is matched, with errors.Is, by the *NotAllowedError of a request the client's
// configuration refuses ([WithReadOnly], [WithAllowedOperations]).
var ErrNotAllowed = errors.New("ovh: operation not allowed")

// NotAllowedError is returned, before anything is sent, for a request the client is configured
// to refuse.
type NotAllowedError struct {
	// Operation is the refused operation's name, empty when the registry does not know the
	// route (a Client.Do call to a package the program does not import).
	Operation string
	Method    string
	Path      string
	// Reason says which option refused it.
	Reason string
}

func (e *NotAllowedError) Error() string {
	name := e.Operation
	if name == "" {
		name = "request"
	}
	return fmt.Sprintf("ovh: %s (%s %s) not sent: %s", name, e.Method, e.Path, e.Reason)
}

// Is makes errors.Is(err, ovh.ErrNotAllowed) true.
func (e *NotAllowedError) Is(target error) bool {
	return target == ErrNotAllowed
}
