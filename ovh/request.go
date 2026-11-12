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

func (c *Client) do(ctx context.Context, req *Request, out any) (http.Header, error) {
	httpReq, err := c.newHTTPRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := c.send(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.Header, fmt.Errorf("ovh: %s %s: reading response: %w", req.Method, req.Path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.Header, newAPIError(req, resp, body)
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

func (c *Client) newHTTPRequest(ctx context.Context, req *Request) (*http.Request, error) {
	// The path is appended to the endpoint as text, so one not starting with "/" would change
	// the host the request (and its bearer token) goes to: "@evil.example/" or ".evil.example/".
	if !strings.HasPrefix(req.Path, "/") {
		return nil, fmt.Errorf("ovh: invalid path %q: it must start with /", req.Path)
	}
	path, _, _ := strings.Cut(req.Path, "?")
	if m := placeholder.FindStringSubmatch(path); m != nil {
		return nil, fmt.Errorf("ovh: %s %s: the path parameter %s is empty", req.Method, req.Path, m[1])
	}
	u, err := url.Parse(c.endpoint + req.Path)
	if err != nil {
		return nil, fmt.Errorf("ovh: invalid path %q: %w", req.Path, err)
	}

	query := u.Query()
	for k, vs := range req.Query {
		query[k] = append(query[k], vs...)
	}
	header := http.Header{}
	if err := encodeParams(req.Params, query, header); err != nil {
		return nil, err
	}
	u.RawQuery = query.Encode()

	var body io.Reader
	if req.Body != nil {
		buf, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("ovh: %s %s: encoding body: %w", req.Method, req.Path, err)
		}
		// A *bytes.Reader lets http.NewRequest set GetBody, so retries can resend it.
		body = bytes.NewReader(buf)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, u.String(), body)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Accept", "application/json")
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	for _, h := range []http.Header{c.headers, header, req.Header} {
		for k, vs := range h {
			for _, v := range vs {
				httpReq.Header.Add(k, v)
			}
		}
	}
	// Set last, so neither the client's headers nor a Request can replace it.
	httpReq.Header.Set("User-Agent", c.userAgent)

	token, err := c.tokens.Token()
	if err != nil {
		return nil, fmt.Errorf("ovh: obtaining an OAuth2 token: %w", err)
	}
	token.SetAuthHeader(httpReq)

	return httpReq, nil
}

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
