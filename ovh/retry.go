// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

package ovh

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Retry limits. They are not options: they protect the API from clients that would retry too
// often or too fast, and a RetryPolicy outside them is refused by WithRetry.
const (
	// MaxRetryAttempts is the most attempts a request may make, the first one included.
	MaxRetryAttempts = 3
	// MinRetryDelay is the shortest wait before a retry. It applies to the backoff and to a
	// Retry-After header asking for less.
	MinRetryDelay = 3 * time.Second
	// MaxRetryDelay is the longest single wait a policy may allow.
	MaxRetryDelay = 60 * time.Second
	// MaxRetryWait bounds the total time one request spends waiting between attempts.
	MaxRetryWait = 2 * time.Minute
)

// Default backoff bounds used when a RetryPolicy leaves them at zero.
const (
	defaultRetryBaseDelay = MinRetryDelay
	defaultRetryMaxDelay  = 30 * time.Second
)

// RetryPolicy configures automatic retrying of transient API failures.
//
// The zero value disables retrying, so retries are opt-in:
//
//	c, err := ovh.NewClient(ovh.WithRetry(ovh.DefaultRetryPolicy()))
//
// Only responses that are safe to repeat are retried: status 429, 503 and 504, and only for
// GET, HEAD and DELETE. POST, PUT and PATCH are never retried: on this API a PUT can start a
// task or place an order, so repeating it is not safe. A DELETE that the server had already
// applied before its retry answers 404 on the retry.
//
// Transport-level failures (connection reset, dial timeout) are also not retried: when a
// request never produced a response, there is no way to know whether the server applied it.
//
// A policy must stay within MaxRetryAttempts, MinRetryDelay and MaxRetryDelay; WithRetry
// refuses one that does not.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts, including the first: 0 or 1 disables
	// retrying, and it may not exceed MaxRetryAttempts.
	MaxAttempts int
	// BaseDelay is the first backoff step, doubling per attempt. Zero means MinRetryDelay;
	// otherwise it must be at least MinRetryDelay.
	BaseDelay time.Duration
	// MaxDelay caps any single wait. Zero means 30s; otherwise it must be between BaseDelay
	// and MaxRetryDelay.
	//
	// It also acts as a ceiling on cooperation: if a Retry-After header asks for longer than
	// this, the response is returned to the caller instead of being retried, because retrying
	// sooner than the server asked would defeat the purpose of the header.
	MaxDelay time.Duration
}

// DefaultRetryPolicy returns the recommended policy: up to three attempts, waiting from 3s to
// 30s between them.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: MaxRetryAttempts,
		BaseDelay:   defaultRetryBaseDelay,
		MaxDelay:    defaultRetryMaxDelay,
	}
}

// validate reports why a policy is outside the retry limits, or nil.
func (p RetryPolicy) validate() error {
	switch {
	case p.MaxAttempts < 0 || p.MaxAttempts > MaxRetryAttempts:
		return fmt.Errorf("MaxAttempts is %d: it must be between 0 and %d", p.MaxAttempts, MaxRetryAttempts)
	case p.BaseDelay < 0 || (p.BaseDelay > 0 && p.BaseDelay < MinRetryDelay):
		return fmt.Errorf("BaseDelay is %s: it must be 0 (default) or at least %s", p.BaseDelay, MinRetryDelay)
	case p.MaxDelay < 0 || (p.MaxDelay > 0 && p.MaxDelay < MinRetryDelay) || p.MaxDelay > MaxRetryDelay:
		return fmt.Errorf("MaxDelay is %s: it must be 0 (default) or between %s and %s", p.MaxDelay, MinRetryDelay, MaxRetryDelay)
	}
	if base, max := p.bounds(); base > max {
		return fmt.Errorf("BaseDelay (%s) is longer than MaxDelay (%s)", base, max)
	}
	return nil
}

// attempts returns the effective total attempt count, never below 1.
func (p RetryPolicy) attempts() int {
	if p.MaxAttempts < 1 {
		return 1
	}
	return p.MaxAttempts
}

// bounds resolves the delay bounds, substituting defaults for zero values.
func (p RetryPolicy) bounds() (base, max time.Duration) {
	base, max = p.BaseDelay, p.MaxDelay
	if base <= 0 {
		base = defaultRetryBaseDelay
	}
	if max <= 0 {
		max = defaultRetryMaxDelay
	}
	return base, max
}

// retryableStatusCodes are the responses worth repeating: the server is telling us it is
// overloaded or temporarily unavailable, not that the request was wrong.
var retryableStatusCodes = map[int]bool{
	http.StatusTooManyRequests:    true, // 429
	http.StatusServiceUnavailable: true, // 503
	http.StatusGatewayTimeout:     true, // 504
}

// retryableMethods may be repeated. Only methods that read or delete: the API has PUTs that
// start tasks or place orders, so the HTTP notion of an idempotent PUT does not hold here.
var retryableMethods = map[string]bool{
	http.MethodGet:    true,
	http.MethodHead:   true,
	http.MethodDelete: true,
}

// retryDrainLimit bounds how much of a discarded response body is read before the next
// attempt. Draining lets the connection be reused; the cap stops a large error payload from
// stalling the retry.
const retryDrainLimit = 4 << 10

// shouldRetry reports whether this response may be attempted again.
func shouldRetry(request *http.Request, resp *http.Response) bool {
	if resp == nil || !retryableStatusCodes[resp.StatusCode] {
		return false
	}
	if !retryableMethods[request.Method] {
		return false
	}
	// A body we cannot rewind cannot be resent.
	if request.Body != nil && request.Body != http.NoBody && request.GetBody == nil {
		return false
	}
	return true
}

// parseRetryAfter interprets a Retry-After header, which may be either a delay in seconds or
// an HTTP date.
func parseRetryAfter(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, true
		}
		return time.Duration(seconds) * time.Second, true
	}

	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay, true
		}
		return 0, true
	}

	return 0, false
}

// retryDelay returns how long to wait before the attempt after this one, and whether to retry
// at all.
//
// A Retry-After header wins over local backoff, since the server knows its own load better
// than we do. If it asks for longer than MaxDelay, this reports false rather than clamping:
// retrying sooner than the server asked would burn an attempt to earn another 429.
//
// Otherwise the delay is exponential with equal jitter: half the backoff plus a random part up
// to the other half, so clients throttled by the same spike do not retry in lockstep.
//
// Either way the delay is never below MinRetryDelay.
func retryDelay(policy RetryPolicy, attempt int, resp *http.Response) (time.Duration, bool) {
	delay, ok := rawRetryDelay(policy, attempt, resp)
	return max(delay, MinRetryDelay), ok
}

func rawRetryDelay(policy RetryPolicy, attempt int, resp *http.Response) (time.Duration, bool) {
	base, max := policy.bounds()

	if resp != nil {
		if delay, ok := parseRetryAfter(resp.Header.Get("Retry-After")); ok {
			if delay > max {
				return 0, false
			}
			return delay, true
		}
	}

	backoff := base
	for i := 1; i < attempt && backoff < max; i++ {
		backoff *= 2
	}
	if backoff > max || backoff <= 0 {
		backoff = max
	}

	half := backoff / 2
	if half <= 0 {
		return backoff, true
	}
	return half + time.Duration(rand.Int63n(int64(half)+1)), true
}
