// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

package ovh

import (
	"net/http"
	"slices"
	"strings"
	"sync"
)

// Operation describes one API operation of a generated package: what it is called in Go, the
// route it calls, and the IAM actions it needs. Every generated package registers its
// operations when it is imported; the generated methods pass theirs with each request, so an
// [APIError] names the operation, and [WithReadOnly] and [WithAllowedOperations] can decide
// whether to send it.
//
//	op, ok := ovh.LookupOperation("vrack.GetVrack")
//	fmt.Println(op.Method, op.Path, op.IAMActions) // GET /v1/vrack/{serviceName} [vrack:apiovh:get]
type Operation struct {
	// Name is the package and the method a caller uses: "vrack.GetVrack". For a list the API
	// paginates by cursor, it is the iterator, the only method that calls it: "vrack.AllTasks".
	Name string
	// Method and Path are the route: "GET", "/v1/vrack/{serviceName}".
	Method string
	Path   string
	// Summary is the API's one-line description of the operation.
	Summary string
	// IAMActions are the IAM actions a caller's policy must grant for the call to succeed.
	IAMActions []string
	// Deprecated is set when the API has deprecated the operation.
	Deprecated bool
	// SchemasVersion is the version of the /v2 contract the operation was generated from:
	// "1.0". It is sent as the X-Schemas-Version header. Empty for a /v1 operation.
	SchemasVersion string
}

// ReadOnly reports whether the operation only reads: its method is GET or HEAD.
func (o *Operation) ReadOnly() bool {
	return readOnlyMethod(o.Method)
}

func (o *Operation) String() string {
	return o.Name + " (" + o.Method + " " + o.Path + ")"
}

func readOnlyMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

var registry struct {
	sync.RWMutex
	byName map[string]*Operation
	// byRoute holds each method's operations, their path split on "/", for MatchOperation.
	byRoute map[string][]route
}

type route struct {
	segments []string
	op       *Operation
}

// RegisterOperations adds operations to the registry. Generated packages call it when they
// are imported; an operation registered again under the same name replaces the first.
func RegisterOperations(ops ...*Operation) {
	registry.Lock()
	defer registry.Unlock()
	if registry.byName == nil {
		registry.byName = map[string]*Operation{}
		registry.byRoute = map[string][]route{}
	}
	for _, op := range ops {
		if old, ok := registry.byName[op.Name]; ok {
			routes := registry.byRoute[old.Method]
			registry.byRoute[old.Method] = slices.DeleteFunc(routes, func(r route) bool { return r.op == old })
		}
		registry.byName[op.Name] = op
		registry.byRoute[op.Method] = append(registry.byRoute[op.Method], route{strings.Split(op.Path, "/"), op})
	}
}

// Operations returns the operations of the packages the program imports, sorted by name. A
// package that is not imported is not registered: the module's index/ directory lists every
// operation of every package.
func Operations() []*Operation {
	registry.RLock()
	defer registry.RUnlock()
	ops := make([]*Operation, 0, len(registry.byName))
	for _, op := range registry.byName {
		ops = append(ops, op)
	}
	slices.SortFunc(ops, func(a, b *Operation) int { return strings.Compare(a.Name, b.Name) })
	return ops
}

// LookupOperation returns the operation of that name: "vrack.GetVrack".
func LookupOperation(name string) (*Operation, bool) {
	registry.RLock()
	defer registry.RUnlock()
	op, ok := registry.byName[name]
	return op, ok
}

// MatchOperation returns the operation a request with that method and path calls, such as
// GET /v1/vrack/pn-12345 for vrack.GetVrack. A route with fewer parameters wins over one with
// more: /v1/me/bill matches GET /v1/me/bill rather than GET /v1/me/{id}. A query string is
// ignored.
func MatchOperation(method, path string) (*Operation, bool) {
	path, _, _ = strings.Cut(path, "?")
	segments := strings.Split(path, "/")
	registry.RLock()
	defer registry.RUnlock()
	var best *Operation
	bestParams := -1
	for _, r := range registry.byRoute[method] {
		params, ok := matchSegments(r.segments, segments)
		if ok && (best == nil || params < bestParams) {
			best, bestParams = r.op, params
		}
	}
	return best, best != nil
}

// matchSegments matches a path against a route, a {name} segment matching any non-empty one,
// and counts the parameters it used.
func matchSegments(routeSegments, pathSegments []string) (int, bool) {
	if len(routeSegments) != len(pathSegments) {
		return 0, false
	}
	params := 0
	for i, s := range routeSegments {
		switch {
		case strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}"):
			if pathSegments[i] == "" {
				return 0, false
			}
			params++
		case s != pathSegments[i]:
			return 0, false
		}
	}
	return params, true
}
