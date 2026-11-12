// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

package ovh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"time"
)

// send performs one logical request: debug logging, retries and the response size cap. It
// returns the response unread, exactly as the transport handed it back.
func (c *Client) send(request *http.Request) (*http.Response, error) {
	attempts := c.retry.attempts()
	var waited time.Duration

	for attempt := 1; ; attempt++ {
		if c.debug {
			dump, err := httputil.DumpRequestOut(request, c.debugBody)
			if err != nil {
				return nil, err
			}
			log.Printf("\n%s\n", redactDebugDump(dump))
		}

		resp, err := c.httpClient.Do(request)
		if err != nil {
			// Not retried: a request that produced no response may still have been applied by
			// the server, and we cannot tell.
			return resp, err
		}

		if c.debug {
			dump, err := httputil.DumpResponse(resp, c.debugBody)
			if err != nil {
				// The caller drops a response that comes with an error, so close it here.
				resp.Body.Close()
				return nil, err
			}
			log.Printf("\n%s\n", redactDebugDump(dump))
		}

		if attempt >= attempts || !shouldRetry(request, resp) {
			return c.capResponseBody(resp), nil
		}

		delay, ok := retryDelay(c.retry, attempt, resp)
		if !ok || waited+delay > MaxRetryWait {
			// The server asked us to wait longer than MaxDelay allows, or this request has
			// waited long enough. Hand the response back untouched -- its body has not been
			// consumed -- rather than retry too early.
			return c.capResponseBody(resp), nil
		}
		waited += delay

		// Drain a little of the discarded response so the connection can be reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, retryDrainLimit))
		resp.Body.Close()

		if request.GetBody != nil {
			body, err := request.GetBody()
			if err != nil {
				return nil, fmt.Errorf("ovh: cannot rewind request body for retry: %w", err)
			}
			request.Body = body
		}

		if c.debug {
			log.Printf("retrying %s %s after %s (attempt %d/%d, got %s)",
				request.Method, request.URL.Path, delay, attempt+1, attempts, resp.Status)
		}

		if err := c.wait(request.Context(), delay); err != nil {
			return nil, err
		}
	}
}

// sleep waits for d, or until ctx is done. It is the default Client.wait.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// redactedPlaceholder replaces the value of a credential-bearing header in debug output.
const redactedPlaceholder = "[REDACTED]"

// sensitiveDebugHeaders lists headers whose values must never reach the logs, in Go's
// canonical header casing.
var sensitiveDebugHeaders = map[string]bool{
	"Authorization":       true,
	"Proxy-Authorization": true,
	"Cookie":              true,
	"Set-Cookie":          true,
	// Legacy OVH application-key auth. This SDK authenticates with OAuth2, but a caller can
	// still set these through WithHeader.
	"X-Ovh-Application": true,
	"X-Ovh-Consumer":    true,
	"X-Ovh-Signature":   true,
}

// redactDebugDump blanks the values of credential-bearing headers in an httputil dump.
//
// It rewrites the serialised text rather than the request or response, so it cannot disturb
// what is about to be sent or read. Only the header block is touched: scanning stops at the
// first blank line, so a body that happens to contain "Authorization: ..." is left intact.
func redactDebugDump(dump []byte) []byte {
	lines := bytes.Split(dump, []byte("\n"))

	for i, line := range lines {
		if i == 0 {
			continue // request line or status line
		}

		trimmed := bytes.TrimRight(line, "\r")
		if len(trimmed) == 0 {
			break // end of the header block
		}

		colon := bytes.IndexByte(trimmed, ':')
		if colon <= 0 {
			continue
		}

		name := http.CanonicalHeaderKey(string(bytes.TrimSpace(trimmed[:colon])))
		if !sensitiveDebugHeaders[name] {
			continue
		}

		replacement := name + ": " + redactedPlaceholder
		if bytes.HasSuffix(line, []byte("\r")) {
			replacement += "\r"
		}
		lines[i] = []byte(replacement)
	}

	return bytes.Join(lines, []byte("\n"))
}

// ErrResponseTooLarge is returned, wrapped, when a response body exceeds the cap set with
// WithMaxResponseBytes. Match it with errors.Is.
var ErrResponseTooLarge = errors.New("ovh: response exceeds the maximum response size")

// limitedResponseBody fails rather than truncating: a silently truncated body either fails to
// decode with a message pointing nowhere near the cause, or decodes into a struct quietly
// missing fields.
type limitedResponseBody struct {
	body      io.ReadCloser
	remaining int64
}

func (l *limitedResponseBody) Read(p []byte) (int, error) {
	if l.remaining < 0 {
		return 0, ErrResponseTooLarge
	}

	// Permit one byte beyond the cap so exceeding it is detectable rather than looking like a
	// body that happened to end exactly on the limit.
	if int64(len(p)) > l.remaining+1 {
		p = p[:l.remaining+1]
	}

	n, err := l.body.Read(p)
	l.remaining -= int64(n)
	if l.remaining < 0 {
		return 0, ErrResponseTooLarge
	}
	return n, err
}

func (l *limitedResponseBody) Close() error { return l.body.Close() }

// capResponseBody wraps resp.Body when a cap is configured. It is applied after any debug
// dump, so the dump is unaffected.
func (c *Client) capResponseBody(resp *http.Response) *http.Response {
	if resp == nil || resp.Body == nil || c.maxResponseBytes <= 0 {
		return resp
	}
	resp.Body = &limitedResponseBody{body: resp.Body, remaining: c.maxResponseBytes}
	return resp
}
