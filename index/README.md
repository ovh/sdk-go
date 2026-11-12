# Operation index

One file per API section, `<section>.jsonl`, with one JSON object per line: each method
of the section's `Service` and the route it calls. It is generated from the SDK's code at
each release, so it always matches it.

Find the method for a route:

```shell
grep -h '"path":"/v1/vrack/{serviceName}"' index/*.jsonl
```

Fields:

| Field | Meaning |
|---|---|
| `section` | The package under `pkg/`. Call `<section>.New(client).<method>`. |
| `method` | The Go method name. |
| `kind` | `call`: one request. `pages`: an iterator over a paginated list. `each`: an iterator that lists IDs with `lists`, then fetches each object with `fetches`. |
| `http`, `path` | The route, with its API version. For `each`, the route of the list. |
| `summary` | The API's one-line description, when it has one. |
| `signature` | The method's Go signature. Types are those of the section's package. |
| `deprecated` | Present and true when the API has deprecated the operation. |
