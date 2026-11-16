// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

package ovh

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// WithReadOnly makes the client send only requests that read: GET and HEAD. Anything else is
// refused before a token is fetched or a byte is sent, with a *NotAllowedError that matches
// ErrNotAllowed. Give a program, or an AI agent acting for you, a read-only client to let it
// explore an account without being able to change it.
//
// except names operations allowed all the same, as patterns of operation names (see
// [WithAllowedOperations]): WithReadOnly("cloud.RebootProjectInstance") reads everything and
// can reboot an instance, nothing else.
func WithReadOnly(except ...string) Option {
	return func(c *config) error {
		if err := checkPatterns(except); err != nil {
			return fmt.Errorf("ovh: WithReadOnly: %w", err)
		}
		c.client.readOnly = true
		c.client.readOnlyExcept = append(c.client.readOnlyExcept, except...)
		return nil
	}
}

// WithAllowedOperations makes the client send only the operations whose name matches one of
// the patterns, refusing any other with a *NotAllowedError that matches ErrNotAllowed. A
// pattern is an operation name, "vrack.GetVrack", or a path.Match pattern of names:
// "vrack.*" (one package), "*.Get*" (every GetX method), "publiccloud.*Instance*".
//
// A request the registry cannot name is refused: a Client.Do call to a route of a package the
// program does not import. Used with WithReadOnly, a request must pass both.
func WithAllowedOperations(patterns ...string) Option {
	return func(c *config) error {
		if len(patterns) == 0 {
			return errors.New("ovh: WithAllowedOperations: no pattern: nothing could be sent")
		}
		if err := checkPatterns(patterns); err != nil {
			return fmt.Errorf("ovh: WithAllowedOperations: %w", err)
		}
		c.client.allowed = append(c.client.allowed, patterns...)
		return nil
	}
}

func checkPatterns(patterns []string) error {
	for _, p := range patterns {
		if strings.TrimSpace(p) == "" {
			return errors.New("an empty pattern")
		}
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("pattern %q: %w", p, err)
		}
	}
	return nil
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// allow applies the client's policy to a request about to be sent. op is nil when the registry
// does not know the route.
func (c *Client) allow(op *Operation, req *Request) error {
	if !c.readOnly && c.allowed == nil {
		return nil
	}
	refuse := func(reason string) error {
		e := &NotAllowedError{Method: req.Method, Path: req.Path, Reason: reason}
		if op != nil {
			e.Operation = op.Name
		}
		return e
	}
	name := ""
	if op != nil {
		name = op.Name
	}
	if c.readOnly && !readOnlyMethod(req.Method) && (name == "" || !matchAny(c.readOnlyExcept, name)) {
		return refuse("the client is read-only (ovh.WithReadOnly)")
	}
	if c.allowed != nil {
		if name == "" {
			return refuse("the client only sends listed operations (ovh.WithAllowedOperations), " +
				"and no imported package declares this route")
		}
		if !matchAny(c.allowed, name) {
			return refuse("not in the operations the client may send (ovh.WithAllowedOperations)")
		}
	}
	return nil
}
