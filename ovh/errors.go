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
// printing the error is usually enough to see what went wrong:
//
//	GET /v1/me: 401 Unauthorized: You must login first (Client::Unauthorized)
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

func newAPIError(req *Request, resp *http.Response, body []byte) *APIError {
	e := &APIError{
		Method:     req.Method,
		Path:       req.Path,
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		QueryID:    resp.Header.Get("X-Ovh-Queryid"),
		Body:       body,
	}
	var payload struct {
		Class   string `json:"class"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil {
		e.Class, e.Message = payload.Class, payload.Message
	}
	return e
}

func (e *APIError) Error() string {
	var b strings.Builder
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
	return b.String()
}
