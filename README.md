# OVHcloud Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/ovh/sdk-go.svg)](https://pkg.go.dev/github.com/ovh/sdk-go)

> **Early version.** This SDK is in active development, and v0.x releases can include breaking
> changes. Pin a version and read the [changelog](CHANGELOG.md) before you upgrade. We will release
> v1.0 once the SDK is stable, in the coming weeks. From v1.0, every stable release follows semantic
> versioning: a breaking change means a new major version.

The Go SDK for the [OVHcloud API](https://developers.eu.ovhcloud.com/): a client for every
public operation, with OAuth2 service-account authentication, retries, typed errors and
iterators over paginated lists.

**Status:** maintained by OVHcloud. Most of the code under `pkg/` is generated from the API's
OpenAPI specifications at each release. Read [CONTRIBUTING.md](CONTRIBUTING.md) before you
propose a change.

- [1. Install](#1-install)
- [2. Your first program](#2-your-first-program)
  - [Coming from go-ovh](#coming-from-go-ovh)
- [3. Finding the operation you need](#3-finding-the-operation-you-need)
- [4. Calling an operation](#4-calling-an-operation)
- [5. Lists and pagination](#5-lists-and-pagination)
- [6. Handling errors](#6-handling-errors)
- [7. Configuration reference](#7-configuration-reference)
- [8. Calling any endpoint](#8-calling-any-endpoint)
- [9. Testing your code](#9-testing-your-code)
- [10. Observability](#10-observability)
- [11. Troubleshooting](#11-troubleshooting)

---

## 1. Install

```shell
go get github.com/ovh/sdk-go
```

Go 1.25 or later (tested on 1.25, 1.26 and 1.27).

You also need OAuth2 credentials: the client ID and secret of a service account.

1. Create the service account in the OVHcloud Control Panel or through the API:
   [Managing OVHcloud service accounts](https://docs.ovhcloud.com/en/guides/manage-and-operate/api/manage-service-account).
   The secret is shown once; save it.
2. Grant it the permissions your program needs with an IAM policy:
   [How to use IAM policies using the OVHcloud Control Panel](https://docs.ovhcloud.com/en/guides/account-and-service-management/account-information/iam-policy-ui),
   or [using the API](https://docs.ovhcloud.com/en/guides/account-and-service-management/account-information/iam-policies-api).

Keep the client ID and secret out of your source code: read them from the environment.

---

## 2. Your first program

This prints the details of your account: `GET /v1/me`. Copy it, set your credentials, run
it.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ovh/sdk-go/ovh"
	"github.com/ovh/sdk-go/pkg/me"
)

func main() {
	client, err := ovh.NewClient() // reads OVH_CLIENT_ID, OVH_CLIENT_SECRET, OVH_REGION
	if err != nil {
		log.Fatal(err)
	}

	account, err := me.New(client).GetAccountDetails(context.Background()) // GET /v1/me
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("account:", ovh.Value(account.Nichandle))
	fmt.Println("email:  ", ovh.Value(account.Email))
	fmt.Println("country:", ovh.Value(account.Country))
}
```

```shell
export OVH_CLIENT_ID=...  OVH_CLIENT_SECRET=...
export OVH_REGION=eu    # eu (default), ca or us
go run .
```

More examples: [examples/](examples/README.md).

Create **one** client and share it: it is safe for concurrent use, and it reuses connections and
tokens. Each API section is a package under `pkg/`: `me.New(client)` gives you the operations
of `me`.

### Choosing your region

Your account lives in one region. `OVH_REGION` (or `ovh.WithRegion(ovh.CA)`) selects both the
API host and the token endpoint, which must agree: a token from one region is refused by the
others.

| Region | `ovh.Region` | API |
|---|---|---|
| Europe | `ovh.EU` | `https://api.eu.ovhcloud.com` |
| Canada | `ovh.CA` | `https://api.ca.ovhcloud.com` |
| US | `ovh.US` | `https://api.us.ovhcloud.com` |

### Coming from go-ovh

This SDK is not [go-ovh](https://github.com/ovh/go-ovh) (`github.com/ovh/go-ovh/ovh`). It is a
new module. Both packages are named `ovh`: to use both in one file, rename one import. The main
differences:

- **Module:** `go get github.com/ovh/sdk-go`, then import `github.com/ovh/sdk-go/ovh` and the
  sections you call, under `github.com/ovh/sdk-go/pkg/`.
- **Credentials:** an OAuth2 service account (`OVH_CLIENT_ID`, `OVH_CLIENT_SECRET`). There is
  no application key, application secret or consumer key, and no `ovh.conf` file. See
  [Install](#1-install) to create one.
- **Region:** `OVH_REGION=eu`, `ca` or `us`, instead of the endpoint `ovh-eu`, `ovh-ca` or
  `ovh-us`.
- **Typed methods:** each API call has a method. The table below shows some of them.
- **Errors:** `errors.Is(err, ovh.ErrNotFound)`, not a check of the `Code` of an `*ovh.APIError`.

| go-ovh | This SDK |
|---|---|
| `ovh.NewOAuth2Client("ovh-eu", clientID, clientSecret)` | `ovh.NewClient(ovh.WithRegion(ovh.EU), ovh.WithOAuth2(clientID, clientSecret))` |
| `client.Get("/me", &account)` | `me.New(client).GetAccountDetails(ctx)` |
| `client.Get("/me/sshKey", &keys)` | `me.New(client).ListSSHKeys(ctx)` |
| `client.Post("/vrack/pn-123456/cloudProject", body, &task)` | `vrack.New(client).CreateCloudProject(ctx, "pn-123456", body)` |
| `apiErr.Code == 404` | `errors.Is(err, ovh.ErrNotFound)` |

To find the method of a route you called with go-ovh, add `/v1` in front of the path and look
it up in the index (see [section 3](#3-finding-the-operation-you-need)).

---

## 3. Finding the operation you need

The SDK mirrors the OVHcloud API: around 90 **sections**, each a Go package under `pkg/`.

To see what the API offers, browse the
[API reference](https://developers.eu.ovhcloud.com/reference/) on the OVHcloud developer hub:
every operation, grouped by product, with its route, parameters and responses. Its groups do not
match the SDK's packages one to one; take the route and find its method as below. The hub
writes a v1 route `/1.0/vrack`; the SDK writes it `/v1/vrack`.

### Guess the name from the URL

Method names follow the URL path, without the section's own prefix:

| Route | Method |
|---|---|
| `GET /vrack` (a list) | `vrack.ListVracks` |
| `GET /vrack/{serviceName}` | `vrack.GetVrack` |
| `PUT /vrack/{serviceName}` | `vrack.UpdateVrack` |
| `POST /vrack/{serviceName}/cloudProject` | `vrack.CreateCloudProject` |
| `DELETE /vrack/{serviceName}/cloudProject/{project}` | `vrack.DeleteCloudProject` |
| `POST /vrack/{serviceName}/terminate` (an action) | `vrack.Terminate` |

Path parameters are left out of the name and become arguments.

### Go names

Names follow the Go naming conventions, whatever the case the API writes them in:

| In the API | In Go |
|---|---|
| the section `publicCloud` | the package `publiccloud` (`pkg/publiccloud`) |
| `vrackId`, `ipBlock`, `sshKey`, `serviceIds` | `VrackID`, `IPBlock`, `SSHKey`, `ServiceIDs`: an initialism is in one case |
| the value `"networkAttack"` of `AbuseReportCategoryEnum` | the constant `AbuseReportCategoryEnumNetworkAttack`: the type's name, then the value's |
| the value `"CLOUDWEB_1"` of `HostingWebOfferEnum` | `HostingWebOfferEnumCloudweb1`: words in capitals are capitalised |

Two values of one enumeration that would make the same name (`"CLOUDWEB_1"` and
`"cloudweb1"`) are told apart by `Alt`: `HostingWebOfferEnumCloudweb1`,
`HostingWebOfferEnumCloudweb1Alt`. `go doc` on the type lists each constant with its value.

### Look up the route in the index

`index/<section>.jsonl`, at the root of the module, lists every method with the route it calls,
its signature and the IAM actions it requires, one JSON object per line. It is generated from the SDK's code at each
release, so it always matches it:

```shell
grep -h '"path":"/v1/vrack/{serviceName}"' "$(go list -m -f '{{.Dir}}' github.com/ovh/sdk-go)"/index/*.jsonl
```

Run it in your project: `go list -m` gives the SDK version your project uses.

```json
{"section":"vrack","method":"GetVrack","kind":"call","http":"GET","path":"/v1/vrack/{serviceName}","summary":"Get a vRack. Returns the name and description of the vRack.","signature":"GetVrack(ctx context.Context, serviceName string) (*VrackVrackWithIAM, error)","iam":["vrack:apiovh:get"]}
```

`index/README.md` describes each field. To list what a program needs, collect the `iam` of
the methods it calls: those are the actions to grant its service account.

### Search by URL path

Every method's documentation starts with its HTTP method and route, so searching for the route
always works, even when the name is not what you guessed:

```shell
grep -rh "calls GET /v1/cloud/project/{serviceName}/instance:" \
    "$(go list -m -f '{{.Dir}}' github.com/ovh/sdk-go)"/pkg/
```

```
// ListProjectInstances calls GET /v1/cloud/project/{serviceName}/instance: Get instance
```

Routes carry their API version: `/v1/...` or `/v2/...`.

### Look up one method

```shell
go doc github.com/ovh/sdk-go/pkg/vrack Service.GetVrack
```

```
func (s *Service) GetVrack(ctx context.Context, serviceName string) (*VrackVrackWithIAM, error)
    GetVrack calls GET /v1/vrack/{serviceName}: Get a vRack. Returns the name
    and description of the vRack.

    Arguments:
      - serviceName: The internal name of your vrack

    Requires the IAM action vrack:apiovh:get.
```

After the route, a method's documentation gives the API's description of the operation, what
each argument is, the IAM actions your service account needs, and whether the operation is
deprecated (with its removal date and replacement, when the API gives them). `go doc` on
a type shows its fields, each with its description.

`go doc` on a package shows the routes it calls and the lists to start from:

```shell
go doc github.com/ovh/sdk-go/pkg/vrack
```

### At run time

Every operation of the packages your program imports is registered, with its route, summary and
required IAM actions:

```go
op, ok := ovh.LookupOperation("vrack.GetVrack")
fmt.Println(op.Method, op.Path, op.IAMActions) // GET /v1/vrack/{serviceName} [vrack:apiovh:get]

op, ok = ovh.MatchOperation(http.MethodGet, "/v1/vrack/pn-123456") // the operation of a URL
for _, op := range ovh.Operations() { ... }                         // all of them, by name
```

---

## 4. Calling an operation

Arguments come in a fixed order: the context, then the path parameters, then any required query
parameters, then the request body. Every call returns the result and an error.

```go
v, err := vrack.New(client).GetVrack(ctx, "pn-123456")

task, err := vrack.New(client).CreateCloudProject(ctx, "pn-123456", vrack.CreateCloudProjectRequest{
	Project: "0123456789abcdef",
})
```

**Optional parameters** go in a `XxxParams` struct, passed last. Leave it out when you need none:

```go
ids, err := vracks.ListVracks(ctx)                                // no filter
ids, err := vracks.ListVracks(ctx, vrack.ListVracksParams{        // filtered on IAM tags
	IAMTags: map[string][]vrack.IAMResourceTagFilter{"env": {{Operator: ovh.Ptr(vrack.IAMResourceTagFilterOperatorEnumEq), Value: ovh.Ptr("prod")}}},
})
```

**Optional fields are pointers**, so `nil` means "not set". `ovh.Ptr(v)` makes one, and
`ovh.Value(p)` reads one, giving the zero value for `nil`:

```go
fmt.Println(ovh.Value(v.Name))
```

### Request bodies have no read-only fields

The API sets some fields itself: an ID, a creation date, a status. A type the API returns has
them, marked `Read-only` in its documentation. A request body never does, so you cannot send one
by mistake. A body whose type has read-only fields takes that type's **writable** form instead,
named with `Writable` after it.

To change an object, get it, then call its `Writable` method: it returns the object in that
form, nested objects included. Change what you need, then send it:

```go
web := hostingweb.New(client)
v, err := web.GetEnvVar(ctx, "example.com", "API_URL") // *HostingWebEnvVar: Key, Type and Status are read-only
body := v.Writable()                                   // HostingWebEnvVarWritable
body.Value = ovh.Ptr("https://api.example.com")
task, err := web.UpdateEnvVar(ctx, "example.com", "API_URL", body)
```

`Writable` returns a deep copy: changing `body` leaves `v` as it was. You can also build the
writable form yourself, with only the fields you set:
`hostingweb.HostingWebEnvVarWritable{Value: ovh.Ptr("https://api.example.com")}`.

### Waiting for the work to finish

Many operations only start the work. A v2 resource comes back `CREATING` or `UPDATING` and
becomes `READY` later; many v1 operations return a task that moves from `todo` to `done`.
Rather than sleeping in a loop, wait with the GET of the same object:

```go
inst, err := pc.CreateInstance(ctx, projectID, spec)
if err != nil {
	return err
}
inst, err = ovh.WaitReady(ctx, func(ctx context.Context) (*publiccloud.PublicCloudInstanceInstance, error) {
	return pc.GetInstance(ctx, projectID, ovh.Value(inst.ID))
})
```

| Function | Polls until |
|---|---|
| `ovh.WaitReady(ctx, get)` | the object's `ResourceStatus` is `READY` (v2 resources) |
| `ovh.WaitTask(ctx, get)` | the task's `Status` is `done` or `DONE` |
| `ovh.WaitGone(ctx, get)` | `get` answers 404, after a deletion |
| `ovh.Wait(ctx, get, done)` | your `done` function says so |

They poll every 2 seconds at first, then less often, up to every 30 seconds, and give up after
30 minutes (`ovh.WaitTimeout(d)`, `ovh.WaitPoll(first, max)`); the context can stop them
sooner. A failed status (a resource in `ERROR`, a task in error, cancelled, or waiting for an
action) ends the wait with an error matching `ovh.ErrWaitFailed`. A 429 or a 5xx is polled
again; any other error ends the wait. Each returns the last value it read, with the error.

### `List` and `All`

Many v1 lists return identifiers only. Their `All` twin is an iterator over the objects
themselves: it lists the identifiers, then fetches each object as your loop reaches it.

```go
ids, err := me.New(client).ListSSHKeys(ctx) // []string

for key, err := range me.New(client).AllSSHKeys(ctx) { // one *me.NichandleSSHKey per step
	if err != nil {
		return err
	}
	fmt.Println(ovh.Value(key.KeyName))
}
```

Each object is one request, so a list of 900 items is 901 requests; `break` stops them. An
object deleted between the list and its fetch is skipped. Any other error is yielded once, and
ends the loop.

### Deprecated operations

The API deprecates operations before it removes them. Their doc comment says so in a
`Deprecated:` paragraph, with the removal date and the route to use instead when the API gives
them. Editors and linters show them as deprecated, and their `ovh.Operation` has `Deprecated`
set. They still work, and never warn at run time.

---

## 5. Lists and pagination

Lists paginated by the API (most v2 lists) have **only an iterator**, `AllX`: it yields one
item at a time and fetches the next page only when the loop reaches it. The SDK never walks
every page behind your back; your loop decides whether to collect everything, filter, or stop
early.

```go
// Every project
var projects []*publiccloud.PublicCloudProjectProjectAsyncWithIAM
for p, err := range publiccloud.New(client).AllProjects(ctx) {
	if err != nil {
		return err
	}
	projects = append(projects, p)
}

// Only the first ready one: no further page is requested after the break
for p, err := range publiccloud.New(client).AllProjects(ctx) {
	if err != nil {
		return err
	}
	if ovh.Value(p.ResourceStatus) == publiccloud.CommonResourceStatusEnumReady {
		fmt.Println(ovh.Value(p.ID))
		break
	}
}
```

An error is yielded once, as `(nil, err)`, and ends the loop. A walk stops with an error after
200 pages rather than looping forever; use the context to bound the time it may take.

Iterators are standard Go 1.23 `iter.Seq2[*T, error]` values: nothing to import to range over
them. v1 lists, which are not paginated, keep their `ListX` method and gain an `AllX` iterator
over the objects (see [`List` and `All`](#list-and-all)).

---

## 6. Handling errors

Printing an error shows the method, the route and what the API said. On a 401 or a 403, it
also gives the IAM actions the call needs, which the service account's policy must grant:

```
vrack.GetVrack: GET /v1/vrack/pn-1: 403 Forbidden: This call has not been granted (Client::Forbidden); requires the IAM action vrack:apiovh:get
```

To act on a common status, use `errors.Is`:

```go
v, err := vracks.GetVrack(ctx, id)
if errors.Is(err, ovh.ErrNotFound) {
	// it does not exist
}
```

| Status | Error |
|---|---|
| 400 | `ovh.ErrBadRequest` |
| 401 | `ovh.ErrUnauthorized` |
| 403 | `ovh.ErrForbidden` |
| 404 | `ovh.ErrNotFound` |
| 409 | `ovh.ErrConflict` |
| 429 | `ovh.ErrTooManyRequests` |

For anything else, use `errors.As` with `*ovh.APIError`. It carries `StatusCode`, the API's
`Class` and `Message`, `QueryID` (the identifier OVHcloud support asks for), the raw `Body`,
the method's name in `Operation`, and the IAM actions the call needs in `IAMActions`:

```go
var apiErr *ovh.APIError
if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusServiceUnavailable {
	// try again later
}
```

An empty path parameter is refused before anything is sent, since `GetVrack(ctx, "")` would
otherwise call `GET /v1/vrack/`, another route:

```
ovh: invalid request: vrack.GetVrack (GET /v1/vrack/{serviceName}): the path parameter serviceName is empty
```

It matches `ovh.ErrInvalidRequest`. A request refused by the client's configuration
(`WithReadOnly`, `WithAllowedOperations`, below) is an `*ovh.NotAllowedError`, matching
`ovh.ErrNotAllowed`; nothing is sent either. A call that `WithDryRun` does not send is an
`*ovh.DryRunError`, matching `ovh.ErrDryRun`. A call refused by a hook returns an error that
wraps the hook's own error.

---

## 7. Configuration reference

Options are passed once, to `ovh.NewClient`. All are optional.

| Option | What it does |
|---|---|
| `WithOAuth2(id, secret)` | Credentials, instead of `OVH_CLIENT_ID` / `OVH_CLIENT_SECRET`. |
| `WithTokenSource(ts)` | Use tokens you obtain yourself. |
| `WithRegion(r)` | `ovh.EU`, `ovh.CA` or `ovh.US`, instead of `OVH_REGION`. |
| `WithTimeout(d)` | Gives up after `d`, including the time spent reading the response. **No timeout by default**: set one. |
| `WithRetry(ovh.DefaultRetryPolicy())` | Retry on 429, 503 and 504: at most 3 attempts, never sooner than 3 s apart, at most 2 minutes of waiting per call. Only `GET`, `HEAD` and `DELETE` are retried, never `POST`, `PUT` or `PATCH`, which can start tasks or place orders. Off by default; a policy outside these limits makes `NewClient` fail. |
| `WithMaxResponseBytes(n)` | Refuse responses larger than this instead of reading them into memory. Unlimited by default. |
| `WithDebug()` | Log every request and response. Passwords and tokens are blanked out. |
| `WithDebugBodies()` | Also log request and response bodies. **These often contain secrets**: use it only with test data. |
| `WithRootCAs(pool)` / `WithRootCAsFromPEM(pem)` | Trust a private certificate authority, for corporate networks. |
| `WithClientCertificates(...)` | Authenticate with a client certificate (mutual TLS). |
| `WithProxy(fn)` | Choose a proxy. By default the SDK honours `HTTP_PROXY` / `HTTPS_PROXY`. |
| `WithUserAgent("my-tool/1.2")` | Names your application in the `User-Agent`, after the SDK's own `ovhcloud-go-sdk/<version> (…)`, which is always sent. |
| `WithHTTPClient(hc)`, `WithEndpoint(url)`, `WithHeader(k, v)` | For proxies, tests and special cases. `WithHeader` cannot set `User-Agent`. |
| `WithReadOnly(except...)` | Send only `GET` and `HEAD`, and the operations named in `except`. Anything else is refused before it is sent. |
| `WithAllowedOperations(patterns...)` | Send only the operations whose name matches a pattern: `"vrack.GetVrack"`, `"vrack.*"`, `"*.Get*"`. |
| `WithDryRun()` | Send reads (`GET`, `HEAD`) as usual. Do not send anything else: return an `*ovh.DryRunError` with the URL and body it would have sent. |
| `WithBeforeRequest(hook)` | Run `hook` before each call, to log it or to refuse it by returning an error. |
| `WithAfterRequest(hook)` | Run `hook` after each call, with its status, query ID, error, duration and number of attempts. |

**Limiting what a client can do.** A program, or an AI agent, that should only look at an
account can get a read-only client, so a bug cannot change anything:

```go
client, err := ovh.NewClient(ovh.WithReadOnly())                                   // reads only
client, err := ovh.NewClient(ovh.WithReadOnly("cloud.RebootProjectInstance"))      // reads, and one action
client, err := ovh.NewClient(ovh.WithAllowedOperations("vrack.*", "dedicatedserver.Get*")) // these only
```

Operations are named as in Go, package and method. A request must pass both options when both
are given. Through `Client.Do`, a route is named by the packages your program imports; with
`WithAllowedOperations`, a route none of them declares is refused. The IAM policy of the service
account remains the real limit: these options protect against mistakes, not against someone
who controls the program.

**Planning changes with a dry run.** With `WithDryRun()`, a program reads the real state of the
account, but changes nothing. A call that would change something is not sent. It returns an
`*ovh.DryRunError`, which matches `ovh.ErrDryRun`, and holds what would have been sent:

```go
client, err := ovh.NewClient(ovh.WithDryRun())
...
_, err = vrack.New(client).CreateCloudProject(ctx, "pn-1", vrack.CreateCloudProjectRequest{
	Project: "0123456789abcdef",
})
var dry *ovh.DryRunError
if errors.As(err, &dry) {
	fmt.Println(dry.Method, dry.URL, string(dry.Body))
}
```

```
POST https://api.eu.ovhcloud.com/v1/vrack/pn-1/cloudProject {"project":"0123456789abcdef"}
```

Printed, the error itself reads:

```
ovh: vrack.CreateCloudProject (POST https://api.eu.ovhcloud.com/v1/vrack/pn-1/cloudProject) not sent: dry run
```

`Operation` is the operation (nil when the route is unknown), `URL` is the full URL with its
query, `Header` the headers, and `Body` the JSON body. No OAuth2 token is fetched for such a
call, and the credential headers (`Authorization`, `Cookie`...) are left out. It is never
retried. The policy still applies first: with `WithReadOnly` or `WithAllowedOperations`, a
refused call stays an `*ovh.NotAllowedError`.

**Watching every call with hooks.** A hook sees every call the client makes, with the operation
it calls. Use it to keep an audit log, or to refuse some calls:

```go
client, err := ovh.NewClient(
	ovh.WithBeforeRequest(func(ctx context.Context, call *ovh.CallInfo) error {
		if call.Operation != nil && call.Operation.Deprecated {
			return fmt.Errorf("%s is deprecated", call.Operation.Name)
		}
		return nil
	}),
	ovh.WithAfterRequest(func(ctx context.Context, call *ovh.CallInfo, r ovh.CallResult) {
		log.Printf("%s %s: %d in %s (%d attempts, query %s): %v",
			call.Method, call.Path, r.StatusCode, r.Duration, r.Attempts, r.QueryID, r.Err)
	}),
)
```

- `CallInfo` has the `Operation` (nil when the route is unknown), `Method`, `Path`, `Query` and
  `Body`. It never holds the token.
- A before hook runs once per call, just before it is sent. Retries do not run it again. If it
  returns an error, nothing is sent, and the error the caller gets wraps it (`errors.Is` works).
- An after hook runs once per call, after the retries. It also sees the calls that were not
  sent: refused by the policy or by a before hook, malformed, or stopped by a dry run. Then
  `StatusCode` and `Attempts` are 0.
- Several hooks run in the order they were given.
- A client is shared, so calls made at the same time run the same hook at the same time. A
  hook must be safe for concurrent use: protect what it writes with a mutex, for example.
- If a call panics (in a before hook, for example), the after hooks still run, with an error
  that says so, and then the panic goes on.
- Each page of an `AllX` iterator is a separate call, so the hooks run once per page.

In one call, the steps run in this order: find the operation, check `WithReadOnly` and
`WithAllowedOperations`, check the request, run the before hooks, stop if it is a dry run,
then send. The after hooks run last, whatever happened.

A reasonable starting point:

```go
client, err := ovh.NewClient(
	ovh.WithTimeout(30*time.Second),
	ovh.WithRetry(ovh.DefaultRetryPolicy()),
)
```

**X-Schemas-Version.** The v2 API serves several versions of its contract. The SDK sends, with
every v2 request, the version its types were generated from, so your account's own setting
does not change what the SDK receives. `WithHeader` cannot set it.

---

## 8. Calling any endpoint

The section packages are a convenience over one call, which you can make yourself, for a route
newer than your SDK version for example:

```go
var out map[string]any
err := client.Do(ctx, &ovh.Request{
	Method: http.MethodGet,
	Path:   ovh.Path("/v1/vrack/{serviceName}", "serviceName", "pn-123456"),
}, &out)
```

---

## 9. Testing your code

Your tests do not need the network or real credentials. Serve the API from an
[`httptest.Server`](https://pkg.go.dev/net/http/httptest), and give the client a fixed token:

- `ovh.WithTokenSource(oauth2.StaticTokenSource(...))`: no credentials, and no call to the
  token endpoint. `oauth2` is `golang.org/x/oauth2`, which the SDK already uses.
- `ovh.WithEndpoint(server.URL)` points the client at the test server. Paths have their
  version: `/v1/me`.

```go
func TestAccount(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me":
			fmt.Fprint(w, `{"nichandle":"ab12345-ovh"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"not found"}`)
		}
	}))
	defer api.Close()

	client, err := ovh.NewClient(
		ovh.WithEndpoint(api.URL),
		ovh.WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"})),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	account, err := me.New(client).GetAccountDetails(ctx)
	if err != nil || ovh.Value(account.Nichandle) != "ab12345-ovh" {
		t.Fatalf("got %v, %v", account, err)
	}
	if _, err := me.New(client).ListSSHKeys(ctx); !errors.Is(err, ovh.ErrNotFound) {
		t.Fatalf("got %v, want a 404", err)
	}
}
```

To test code that takes a result, build the struct yourself, with the fields your code reads:
`&me.NichandleNichandle{Nichandle: ovh.Ptr("ab12345-ovh")}`.

`ovh.WithDryRun()` is another way to test code that changes an account: it reads the real
state, and returns what each change would send instead of sending it (see
[Planning changes with a dry run](#7-configuration-reference)).

---

## 10. Observability

The SDK has no tracing or metrics of its own. Two places see every call:

- **Hooks** see each call with its operation. `ovh.WithAfterRequest` gives the status, the
  duration, the number of attempts and the `X-Ovh-Queryid`, which identifies a call for
  OVHcloud support, also when it succeeds. Use it for logs and metrics (see
  [Watching every call with hooks](#7-configuration-reference)):

  ```go
  client, err := ovh.NewClient(ovh.WithAfterRequest(
  	func(ctx context.Context, call *ovh.CallInfo, r ovh.CallResult) {
  		slog.InfoContext(ctx, "ovh call", "method", call.Method, "path", call.Path,
  			"status", r.StatusCode, "duration", r.Duration, "query_id", r.QueryID)
  	}))
  ```

- **Your own HTTP client** sees each request, token requests included. Wrap its transport,
  for example with the
  [OpenTelemetry instrumentation](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp)
  of `net/http`, which makes a span per request:

  ```go
  client, err := ovh.NewClient(ovh.WithHTTPClient(&http.Client{
  	Transport: otelhttp.NewTransport(http.DefaultTransport),
  }))
  ```

With your own HTTP client:

- The SDK still sets on each request the `Authorization`, `User-Agent` and the other headers
  it needs, and applies `ovh.WithTimeout` when you give it.
- The TLS and proxy options (`WithRootCAs`, `WithClientCertificates`, `WithProxy`) need its
  `Transport` to be an `*http.Transport`. With a wrapped transport, set them on the
  `*http.Transport` inside.
- Each retry is a new request, so the transport sees each attempt. A hook runs once per call.

---

## 11. Troubleshooting

**`ovh: no credentials`**: `OVH_CLIENT_ID` and `OVH_CLIENT_SECRET` are not set in the
environment the program runs in.

**`401 You must login first`**: the API did not accept your token. Check that `OVH_REGION`
matches the region your account and service account live in, then that the credentials are
valid.

**`403 This call has not been granted`**: you are authenticated, but your service account
lacks permission for that route. The method's documentation names the IAM actions it requires:
grant them with an
[IAM policy](https://docs.ovhcloud.com/en/guides/account-and-service-management/account-information/iam-policy-ui).

**`404 Not Found`**: usually a wrong identifier, such as a project ID from another region.

**The call hangs**: you have no timeout. Add `ovh.WithTimeout(30 * time.Second)`.

**See exactly what is sent**: add `ovh.WithDebug()`. Credential headers are blanked, so the
output is safe to paste into a bug report.

---

## Contributing

Issues and pull requests are welcome: read [CONTRIBUTING.md](CONTRIBUTING.md) first, since
`pkg/` is generated. Report bugs at <https://github.com/ovh/sdk-go/issues>.

## License

Copyright 2026 OVH SAS. Licensed under the [Apache License 2.0](LICENSE); see
[LICENSES/Apache-2.0.txt](LICENSES/Apache-2.0.txt) for the full text.

## Related links

- Contribute: [CONTRIBUTING.md](CONTRIBUTING.md)
- Report bugs: <https://github.com/ovh/sdk-go/issues>
- Releases and changes: [CHANGELOG.md](CHANGELOG.md), <https://github.com/ovh/sdk-go/releases>
- The package and its API reference: <https://pkg.go.dev/github.com/ovh/sdk-go>
- Examples: [examples/](examples/README.md)
- The API itself: the OVHcloud developer hub, <https://developers.eu.ovhcloud.com/>, and its
  [API reference](https://developers.eu.ovhcloud.com/reference/)
- Service accounts and IAM: [Managing OVHcloud service accounts](https://docs.ovhcloud.com/en/guides/manage-and-operate/api/manage-service-account),
  [IAM policies](https://docs.ovhcloud.com/en/guides/account-and-service-management/account-information/iam-policy-ui)
