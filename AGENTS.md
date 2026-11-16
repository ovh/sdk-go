# AGENTS.md

Notes for AI coding agents that write code with this SDK, or work in this repository.

## Find the right method

- Each API section is a package: `github.com/ovh/sdk-go/pkg/<section>`, in lowercase
  (`publicCloud` is `pkg/publiccloud`). Its operations are methods on `<section>.New(client)`.
- To find the method for a route, read `index/<section>.jsonl` at the root of this module. It
  has one JSON line per method, with the route, the Go signature, the summary and the IAM
  actions the call requires (`iam`). Search it before you guess a name. When you write a
  program, list the `iam` of the methods it calls: the service account needs those actions.
  From a project that uses the SDK, this searches the version it uses:

  ```shell
  grep -h '"path":"/v1/vrack/{serviceName}"' "$(go list -m -f '{{.Dir}}' github.com/ovh/sdk-go)"/index/*.jsonl
  ```

- The doc comment of each operation starts with its route:
  `GetVrack calls GET /v1/vrack/{serviceName}`. It then gives the API's description, the
  arguments, the IAM actions the call requires, and whether it is deprecated. A
  deprecated operation names its replacement route when the API gives one: look it up in the
  index too. `AllX` iterators say which list they walk.
- Routes start with their API version, `/v1/` or `/v2/`. The same path can exist in both.
- At run time, `ovh.LookupOperation("vrack.GetVrack")` returns an operation's route, summary
  and required IAM actions, `ovh.MatchOperation("GET", "/v1/vrack/pn-1")` finds the operation
  of a concrete path, and `ovh.Operations()` lists them all. Only imported packages are known.

## Write the call

- One client for the whole program: `client, err := ovh.NewClient()`. It reads
  `OVH_CLIENT_ID`, `OVH_CLIENT_SECRET` and `OVH_REGION` (`eu`, `ca` or `us`). It is safe for
  concurrent use.
- Arguments come in this order: `ctx`, path parameters, required query parameters, the request
  body. Optional parameters go in a trailing `XxxParams` struct, which you can leave out.
- Optional fields are pointers. Set one with `ovh.Ptr(v)`, read one with `ovh.Value(p)`.
- A request body has no read-only field. A body whose type has some takes `<Type>Writable`
  instead (`GetEnvVar` returns `HostingWebEnvVar`, `UpdateEnvVar` takes
  `HostingWebEnvVarWritable`). To change an object: get it, call `v.Writable()`, change the
  fields you need, send it. A field marked `Read-only` in a returned type is set by the API.
- `go doc` on a package says which routes it calls and which lists to start from.
- Names follow the Go conventions: an initialism is in one case (`VrackID`, `IPBlock`). Enum
  constants are the type's name, then the value's: `vrack.IAMResourceTagFilterOperatorEnumEq`.
  Check the model file, or `go doc` on the type, before you use one.
- `ListX` returns a slice, often of IDs only. `AllX` is an `iter.Seq2[*T, error]` over the
  objects: range over it and check `err` at each step.
- Paginated v2 lists have only `AllX`. There is no method that loads every page at once.
- A path parameter must not be empty: the call fails before sending anything, with an error
  matching `ovh.ErrInvalidRequest`.
- No route in the SDK yet? Call it directly with
  `client.Do(ctx, &ovh.Request{Method: ..., Path: ovh.Path("/v1/...", "name", value)}, &out)`.

## Wait for the work to finish

- Many calls only start the work. A v2 resource comes back `CREATING` or `UPDATING`; a v1
  call often returns a task. Do not sleep in a loop: wait with the GET of the same object.
- `ovh.WaitReady(ctx, get)` polls until `ResourceStatus` is `READY`. `ovh.WaitTask(ctx, get)`
  polls until a task's `Status` is `done`. `ovh.WaitGone(ctx, get)` polls until a 404, after a
  delete. `get` is a closure over the generated GET:

  ```go
  inst, err := ovh.WaitReady(ctx, func(ctx context.Context) (*publiccloud.PublicCloudInstanceInstance, error) {
  	return pc.GetInstance(ctx, projectID, id)
  })
  ```

- A failed status (`ERROR`, a task in error or waiting for an action) returns an error matching
  `ovh.ErrWaitFailed`. The wait gives up after 30 minutes: `ovh.WaitTimeout(d)` changes that.
  `ovh.Wait(ctx, get, done)` takes any condition.

## Handle errors

- An error from the API is an `*ovh.APIError`. Its message is the API's own.
- Match common statuses with `errors.Is`: `ovh.ErrNotFound`, `ovh.ErrForbidden`,
  `ovh.ErrUnauthorized`, `ovh.ErrBadRequest`, `ovh.ErrConflict`, `ovh.ErrTooManyRequests`.
- Use `errors.As` to read `StatusCode`, `Class`, `Message` and `QueryID` (for support), and
  `Operation`, the method's name.
- A 403 means the service account lacks an IAM action. The error's message ends with the
  actions the call needs (`requires the IAM action vrack:apiovh:get`), also in `IAMActions`:
  report them to the user, who can grant them. Retrying does not help.
- A request the client is configured to refuse returns an `*ovh.NotAllowedError`, matching
  `ovh.ErrNotAllowed`, and nothing is sent. Do not work around it: ask the user.
- With `ovh.WithDryRun()`, a call that would change something returns an `*ovh.DryRunError`,
  matching `ovh.ErrDryRun`. It is not a failure: it tells what would have been sent.

## Configure the client

- To explore an account safely, use a read-only client: `ovh.WithReadOnly()` sends only `GET`
  and `HEAD`. `ovh.WithReadOnly("cloud.RebootProjectInstance")` also allows that one operation.
  `ovh.WithAllowedOperations("vrack.*", "*.Get*")` sends only the operations whose name
  matches. A request must pass both when both are set.
- To plan a change before making it, use `ovh.WithDryRun()`. Reads are sent, so you see the
  real state. Any other call is not sent: it returns an `*ovh.DryRunError` (`ovh.ErrDryRun`)
  with the `Method`, the full `URL`, the `Header` and the JSON `Body` it would have sent. Show these to the
  user, then make the change with a normal client once they agree.
- To log or veto calls, use `ovh.WithBeforeRequest(hook)` (return an error to stop the call)
  and `ovh.WithAfterRequest(hook)` (sees every call, with its status, `QueryID`, error,
  duration and attempts). The hooks get the operation, so `call.Operation.Name` and
  `call.Operation.ReadOnly()` tell what the call does. Each page of an `AllX` iterator is a
  separate call. Calls made at the same time run a hook at the same time, so a hook must be
  safe for concurrent use.

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
