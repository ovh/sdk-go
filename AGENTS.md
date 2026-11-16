# AGENTS.md

Notes for AI coding agents that write code with this SDK, or work in this repository.

## Find the right method

- Each API section is a package: `github.com/ovh/sdk-go/pkg/<section>`, in lowercase
  (`publicCloud` is `pkg/publiccloud`). Its operations are methods on `<section>.New(client)`.
- To find the method for a route, read `index/<section>.jsonl` at the root of this module. It
  has one JSON line per method, with the route, the Go signature, the summary and the IAM
  actions the call requires (`iam`). Search it before you guess a name. When you write a
  program, list the `iam` of the methods it calls: the service account needs those actions. From a project that uses the SDK, this searches the version it uses:

  ```shell
  grep -h '"path":"/v1/vrack/{serviceName}"' "$(go list -m -f '{{.Dir}}' github.com/ovh/sdk-go)"/index/*.jsonl
  ```

- The doc comment of each operation starts with its route:
  `GetVrack calls GET /v1/vrack/{serviceName}`. It then gives the API's description, the
  arguments, the IAM actions the call requires, and whether it is beta or deprecated. A
  deprecated operation names its replacement route when the API gives one: look it up in the
  index too. `AllX` iterators say which list they walk.
- Routes start with their API version, `/v1/` or `/v2/`. The same path can exist in both.

## Write the call

- One client for the whole program: `client, err := ovh.NewClient()`. It reads
  `OVH_CLIENT_ID`, `OVH_CLIENT_SECRET` and `OVH_REGION` (`eu`, `ca` or `us`). It is safe for
  concurrent use.
- Arguments come in this order: `ctx`, path parameters, required query parameters, the request
  body. Optional parameters go in a trailing `XxxParams` struct, which you can leave out.
- Optional fields are pointers. Set one with `ovh.Ptr(v)`, read one with `ovh.Value(p)`.
- Names follow the Go conventions: an initialism is in one case (`VrackID`, `IPBlock`). Enum
  constants are the type's name, then the value's: `vrack.IAMResourceTagFilterOperatorEnumEq`.
  Check the model file, or `go doc` on the type, before you use one.
- `ListX` returns a slice, often of IDs only. `AllX` is an `iter.Seq2[*T, error]` over the
  objects: range over it and check `err` at each step.
- Paginated v2 lists have only `AllX`. There is no method that loads every page at once.
- A path parameter must not be empty: the call fails before sending anything.
- No route in the SDK yet? Call it directly with
  `client.Do(ctx, &ovh.Request{Method: ..., Path: ovh.Path("/v1/...", "name", value)}, &out)`.

## Handle errors

- An error from the API is an `*ovh.APIError`. Its message is the API's own.
- Match common statuses with `errors.Is`: `ovh.ErrNotFound`, `ovh.ErrForbidden`,
  `ovh.ErrUnauthorized`, `ovh.ErrBadRequest`, `ovh.ErrConflict`, `ovh.ErrTooManyRequests`.
- Use `errors.As` to read `StatusCode`, `Class`, `Message` and `QueryID` (for support).
- A 403 means the service account lacks the IAM action shown in the method's doc comment.

## Configure the client

- There is no timeout by default. Set one: `ovh.WithTimeout(30 * time.Second)`.
- Retries are off by default. `ovh.WithRetry(ovh.DefaultRetryPolicy())` retries only `GET`,
  `HEAD` and `DELETE`, never `POST`, `PUT` or `PATCH`: those can start tasks or place orders.
- `ovh.WithDebug()` logs requests with credentials hidden. `WithDebugBodies()` also logs
  bodies, which can contain secrets.

## Change this repository

- `pkg/` and `index/` are generated at each release. Do not edit them: the next release
  overwrites the change. Report the problem instead, with the route involved.
- `ovh/` is hand-written: changes there follow [CONTRIBUTING.md](CONTRIBUTING.md).
- Check a change with `go build ./...` and `go vet ./...`. The SDK's tests are not part of this
  module.

More: [README.md](README.md) is the full guide.
