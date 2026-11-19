// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

package ovh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Request describes one API call. It is what the section packages build, and what Do sends.
//
//	var me map[string]any
//	err := c.Do(ctx, &ovh.Request{Method: http.MethodGet, Path: "/v1/me"}, &me)
type Request struct {
	// Operation describes the operation the request calls. Generated methods set it; a
	// Request built by hand may leave it nil, and the registry is then searched for the
	// route (see [MatchOperation]).
	Operation *Operation
	// Method is the HTTP method, e.g. http.MethodGet.
	Method string
	// Path is appended to the client's endpoint and includes the API version: "/v1/me",
	// "/v2/publicCloud/project". Build one with path parameters using Path.
	Path string
	// Query and Header are added to the request as they are.
	Query  url.Values
	Header http.Header
	// Params is a struct whose fields carry `ovh:"query,name"` or `ovh:"header,name"` tags,
	// as the generated XxxParams types do. Nil pointers and empty values are skipped.
	Params any
	// Body is sent as JSON when non-nil.
	Body any
}

// Do sends req and decodes the JSON response into out, which may be nil to discard it.
//
// A response outside 2xx is returned as an *APIError.
func (c *Client) Do(ctx context.Context, req *Request, out any) error {
	_, err := c.do(ctx, req, out)
	return err
}

// do sends one logical call. The steps run in this order: find the operation, apply the
// policy (WithReadOnly, WithAllowedOperations), check and encode the request, run the before
// hooks, stop there in dry-run mode, then sign the request and send it. The after hooks see
// every outcome, refusals included.
func (c *Client) do(ctx context.Context, req *Request, out any) (header http.Header, err error) {
	start := time.Now()
	call := &CallInfo{Method: req.Method, Path: req.Path, Query: req.Query, Body: req.Body}
	var result CallResult
	if len(c.afterRequest) > 0 {
		defer func() {
			// A panic (in a before hook, a token source...) must not look like a success to
			// the after hooks. They see it as an error, then the panic goes on.
			p := recover()
			result.Err, result.Duration = err, time.Since(start)
			if p != nil {
				result.Err = fmt.Errorf("ovh: panic: %v", p)
			}
			for _, hook := range c.afterRequest {
				hook(ctx, call, result)
			}
			if p != nil {
				panic(p)
			}
		}()
	}

	// Only an operation the caller gives sets X-Schemas-Version. A Request built by hand
	// decodes into the caller's own types, so a matched operation does not change the contract.
	schemasVersion := ""
	if req.Operation != nil {
		schemasVersion = req.Operation.SchemasVersion
	}
	op := req.Operation
	if op == nil {
		op, _ = MatchOperation(req.Method, req.Path)
	}
	call.Operation = op
	if err := c.allow(op, req); err != nil {
		return nil, err
	}
	if req.Operation == nil && op != nil {
		named := *req // the caller's Request is left as it is
		named.Operation = op
		req = &named
	}
	prepared, err := c.prepare(req)
	if err != nil {
		return nil, err
	}
	prepared.schemasVersion = schemasVersion
	call.Query = prepared.url.Query()

	for _, hook := range c.beforeRequest {
		if err := hook(ctx, call); err != nil {
			return nil, fmt.Errorf("ovh: %s (%s %s) not sent: refused by a hook: %w", callName(op), req.Method, req.Path, err)
		}
	}
	if c.dryRun && !readOnlyMethod(req.Method) {
		header := c.headersFor(req, prepared)
		for name := range sensitiveDebugHeaders {
			header.Del(name)
		}
		return nil, &DryRunError{Operation: op, Method: req.Method, URL: prepared.url.String(),
			Header: header, Body: prepared.body}
	}

	httpReq, err := c.sign(ctx, req, prepared)
	if err != nil {
		return nil, err
	}

	resp, attempts, err := c.send(httpReq)
	result.Attempts = attempts
	if resp != nil {
		// Also on an error: send may return the response it could not use.
		result.StatusCode, result.QueryID = resp.StatusCode, resp.Header.Get("X-Ovh-Queryid")
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.Header, fmt.Errorf("ovh: %s %s: reading response: %w", req.Method, req.Path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.Header, newAPIError(op, req, resp, body)
	}

	if out == nil || len(body) == 0 {
		return resp.Header, nil
	}
	// Some v1 endpoints answer a bare, unquoted string.
	if s, ok := out.(*string); ok && !json.Valid(body) {
		*s = string(body)
		return resp.Header, nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return resp.Header, fmt.Errorf("ovh: %s %s: decoding response: %w", req.Method, req.Path, err)
	}
	return resp.Header, nil
}

// prepared is a request checked and encoded, but not signed: building it needs no token. A dry
// run reports it, and sign turns it into an *http.Request.
type prepared struct {
	url    *url.URL
	header http.Header // the headers from Params
	body   []byte      // the JSON body, nil when the request has none
	// schemasVersion is the X-Schemas-Version to send, empty for none.
	schemasVersion string
}

func (c *Client) prepare(req *Request) (*prepared, error) {
	// The path is appended to the endpoint as text, so one not starting with "/" would change
	// the host the request (and its bearer token) goes to: "@evil.example/" or ".evil.example/".
	if !strings.HasPrefix(req.Path, "/") {
		return nil, fmt.Errorf("%w: path %q: it must start with /", ErrInvalidRequest, req.Path)
	}
	path, _, _ := strings.Cut(req.Path, "?")
	if m := placeholder.FindStringSubmatch(path); m != nil {
		where := req.Method + " " + req.Path
		if req.Operation != nil {
			where = req.Operation.Name + " (" + where + ")"
		}
		return nil, fmt.Errorf("%w: %s: the path parameter %s is empty", ErrInvalidRequest, where, m[1])
	}
	u, err := url.Parse(c.endpoint + req.Path)
	if err != nil {
		return nil, fmt.Errorf("%w: path %q: %w", ErrInvalidRequest, req.Path, err)
	}

	query := u.Query()
	for k, vs := range req.Query {
		query[k] = append(query[k], vs...)
	}
	p := &prepared{url: u, header: http.Header{}}
	if err := encodeParams(req.Params, query, p.header); err != nil {
		return nil, err
	}
	u.RawQuery = query.Encode()

	if req.Body != nil {
		p.body, err = json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("ovh: %s %s: encoding body: %w", req.Method, req.Path, err)
		}
	}
	return p, nil
}

// sign builds the *http.Request of a prepared request: it adds the headers and the OAuth2
// token.
func (c *Client) sign(ctx context.Context, req *Request, p *prepared) (*http.Request, error) {
	var body io.Reader
	if p.body != nil {
		// A *bytes.Reader lets http.NewRequest set GetBody, so retries can resend it.
		body = bytes.NewReader(p.body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, p.url.String(), body)
	if err != nil {
		return nil, err
	}

	httpReq.Header = c.headersFor(req, p)

	token, err := c.tokens.Token()
	if err != nil {
		return nil, fmt.Errorf("ovh: obtaining an OAuth2 token: %w", err)
	}
	token.SetAuthHeader(httpReq)

	return httpReq, nil
}

// headersFor returns the headers of a request, all but the Authorization header that sign adds
// last.
func (c *Client) headersFor(req *Request, p *prepared) http.Header {
	header := http.Header{}
	header.Set("Accept", "application/json")
	if p.body != nil {
		header.Set("Content-Type", "application/json")
	}
	for _, h := range []http.Header{c.headers, p.header, req.Header} {
		for k, vs := range h {
			for _, v := range vs {
				header.Add(k, v)
			}
		}
	}
	if p.schemasVersion != "" {
		// Set, not added: the API reads the first value.
		header.Set(schemasVersionHeader, p.schemasVersion)
	}
	// Set last, so neither the client's headers nor a Request can replace it.
	header.Set("User-Agent", c.userAgent)
	return header
}

// schemasVersionHeader carries the version of the /v2 contract: the one a request asks for,
// and the one the API answered with.
const schemasVersionHeader = "X-Schemas-Version"

// Call sends req and returns the decoded response. The section packages use it, one call per
// operation.
func Call[T any](ctx context.Context, c *Client, req *Request) (T, error) {
	var out T
	if err := c.Do(ctx, req, &out); err != nil {
		var zero T
		return zero, err
	}
	return out, nil
}

// Path fills the {name} placeholders of path from name/value pairs, escaping each value:
//
//	ovh.Path("/v1/vrack/{serviceName}/ip/{ip}", "serviceName", sn, "ip", ip)
//
// An empty value (or a nil pointer) leaves its placeholder in place, and Do then refuses the
// request: GET /v1/vrack/ instead of GET /v1/vrack/{serviceName} would call another route.
func Path(path string, pairs ...any) string {
	for i := 0; i+1 < len(pairs); i += 2 {
		name, _ := pairs[i].(string)
		value := ""
		if vs := encodeValue(reflect.ValueOf(pairs[i+1])); len(vs) > 0 {
			value = vs[0]
		}
		if value == "" {
			continue
		}
		path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(value))
	}
	return path
}

// placeholder finds a {name} left in a path. Path escapes the braces of every value it fills
// in, so one left over is a parameter that was never set. Only the part before any query
// string is checked: braces in a query are data.
var placeholder = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_.-]*)\}`)

// Values builds query parameters from name/value pairs, encoded as Params fields are.
func Values(pairs ...any) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		name, _ := pairs[i].(string)
		v[name] = append(v[name], encodeValue(reflect.ValueOf(pairs[i+1]))...)
	}
	return v
}

// Headers builds headers from name/value pairs, encoded as Params fields are.
func Headers(pairs ...any) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(pairs); i += 2 {
		name, _ := pairs[i].(string)
		if vs := encodeValue(reflect.ValueOf(pairs[i+1])); len(vs) > 0 {
			h.Set(name, strings.Join(vs, ","))
		}
	}
	return h
}

// Optional returns the first of an operation's optional parameter structs, or nil. Generated
// operations take their optional parameters as a variadic argument, so they can be omitted.
func Optional[T any](params []T) any {
	if len(params) == 0 {
		return nil
	}
	return &params[0]
}

// Ptr returns a pointer to v, for filling optional fields: ovh.Ptr("GRA11").
func Ptr[T any](v T) *T {
	return &v
}

// Value returns what p points to, or the zero value when p is nil, for reading optional fields:
// ovh.Value(account.Email).
func Value[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

var timeType = reflect.TypeFor[time.Time]()

// encodeParams adds the fields of a Params struct to query and header.
func encodeParams(params any, query url.Values, header http.Header) error {
	if params == nil {
		return nil
	}
	v := reflect.ValueOf(params)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("ovh: Params must be a struct, got %T", params)
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		in, name, ok := strings.Cut(t.Field(i).Tag.Get("ovh"), ",")
		if !ok {
			continue
		}
		values := encodeValue(v.Field(i))
		if len(values) == 0 {
			continue
		}
		switch in {
		case "query":
			query[name] = append(query[name], values...)
		case "header":
			header.Set(name, strings.Join(values, ","))
		default:
			return fmt.Errorf("ovh: field %s.%s: unknown parameter location %q", t.Name(), t.Field(i).Name, in)
		}
	}
	return nil
}

// encodeValue renders one parameter value. Nil and empty values yield nothing; slices yield one
// value per element (repeated keys); maps and structs are sent as JSON, which is how the API
// takes object parameters such as iamTags.
func encodeValue(v reflect.Value) []string {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return nil
	}
	if v.Type() == timeType {
		return []string{v.Interface().(time.Time).Format(time.RFC3339)}
	}

	switch v.Kind() {
	case reflect.String:
		return []string{v.String()}
	case reflect.Bool:
		return []string{strconv.FormatBool(v.Bool())}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return []string{strconv.FormatInt(v.Int(), 10)}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return []string{strconv.FormatUint(v.Uint(), 10)}
	case reflect.Float32, reflect.Float64:
		return []string{strconv.FormatFloat(v.Float(), 'f', -1, 64)}
	case reflect.Slice, reflect.Array:
		var out []string
		for i := 0; i < v.Len(); i++ {
			out = append(out, encodeValue(v.Index(i))...)
		}
		return out
	case reflect.Map:
		if v.Len() == 0 {
			return nil
		}
		fallthrough
	default:
		buf, err := json.Marshal(v.Interface())
		if err != nil {
			return nil
		}
		return []string{string(buf)}
	}
}
