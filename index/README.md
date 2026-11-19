# Operation index

One file per API section, `<section>.jsonl` (the section's package name), with one JSON object
per line: each method of the SDK and the route it calls. 6962 methods in
80 files. It is generated with the SDK's code at each release, from the
same data, so it always matches it.

Find the method of a route:

```shell
grep -h '"path":"/v1/me"' index/*.jsonl
```

To list what a program needs, collect the `iam` of the methods it calls: those are the
actions to grant its service account. Find the methods that need one action:

```shell
grep -h '"account:apiovh:me/get"' index/*.jsonl
```

The types a signature names are in the section's package, with their fields and the enum
values. `go doc` shows one: `go doc github.com/ovh/sdk-go/pkg/me.NichandleNichandle`.

A request body takes the `<Type>Writable` form of a type when the type has read-only fields:
the same type without them.

Fields:

| Field | Meaning |
|---|---|
| `section` | The API section: its package under `pkg/` (`publiccloud`). |
| `method` | The method name. Call `<section>.New(client).<method>(ctx, ...)`. |
| `kind` | `call`: one request. `pages`: an iterator over a list paginated by cursor. `each`: an iterator that lists IDs with `lists`, then gets each object with `fetches`. |
| `http`, `path` | The route, with its API version. For `each`, the route of the list. |
| `summary` | The API's one-line description, when it has one. |
| `signature` | The method's Go signature. Its types are those of the section's package. |
| `iam` | The IAM actions the call requires: a 403 means the service account's policy lacks one. For `each`, those of the list and of the fetch. Absent when the API declares none. |
| `deprecated` | Present and true when the API has deprecated the operation. |
| `id` | `<section>.<method>`. For `call` and `pages`, the operation's name: what errors show, and what `WithReadOnly` and `WithAllowedOperations` match. An `each` iterator is not an operation: it sends `<section>.<lists>` and `<section>.<fetches>`, so errors show those names, and `WithAllowedOperations` must allow both. |
| `import` | The section's package. Import it from there: `github.com/ovh/sdk-go/pkg/publiccloud`. |
