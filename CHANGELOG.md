# Changelog

Notable changes to the SDK. Behavioural changes are called out explicitly.

## 0.3.0 (2026-11-16)

Compared with 0.2.1.

### New

- `backupservices`: the package `backupservices`
- `licensehycu`: the package `licensehycu`
- `publiccloud`: `PublicCloudStorageFileFileStorageAttachment`
- `publiccloud`: `PublicCloudStorageFileFileStorageStatusEnum`
- `publiccloud`: `PublicCloudStorageFileFileStorageCurrentState.Attachments`
- `publiccloud`: `PublicCloudStorageFileFileStorageCurrentState.Status`
- `veeamveeamenterprise`: the package `veeamveeamenterprise`

### SDK core

- The operation index (`index/<section>.jsonl`) gives the IAM actions each method requires, in
  `iam`: what to grant a service account that gets a 403. For an `AllX` iterator over a v1
  list, those of the list and of the get it calls for each object.

<details>
<summary>Every change to the API contract, as oasdiff reports it (10)</summary>

#### `publiccloud`

- `AllShares`: added the optional property `items/currentState/allOf[#/components/schemas/publicCloud.storage.file.FileStorageCurrentState]/attachments` to the response with the `200` status (GET /v2/publicCloud/project/{projectId}/storage/file/share)
- `AllShares`: added the optional property `items/currentState/allOf[#/components/schemas/publicCloud.storage.file.FileStorageCurrentState]/status` to the response with the `200` status (GET /v2/publicCloud/project/{projectId}/storage/file/share)
- `CreateShare`: added the optional property `currentState/allOf[#/components/schemas/publicCloud.storage.file.FileStorageCurrentState]/attachments` to the response with the `200` status (POST /v2/publicCloud/project/{projectId}/storage/file/share)
- `CreateShare`: added the optional property `currentState/allOf[#/components/schemas/publicCloud.storage.file.FileStorageCurrentState]/status` to the response with the `200` status (POST /v2/publicCloud/project/{projectId}/storage/file/share)
- `DeleteShare`: added the optional property `currentState/allOf[#/components/schemas/publicCloud.storage.file.FileStorageCurrentState]/attachments` to the response with the `200` status (DELETE /v2/publicCloud/project/{projectId}/storage/file/share/{fileStorageId})
- `DeleteShare`: added the optional property `currentState/allOf[#/components/schemas/publicCloud.storage.file.FileStorageCurrentState]/status` to the response with the `200` status (DELETE /v2/publicCloud/project/{projectId}/storage/file/share/{fileStorageId})
- `EditShare`: added the optional property `currentState/allOf[#/components/schemas/publicCloud.storage.file.FileStorageCurrentState]/attachments` to the response with the `200` status (PUT /v2/publicCloud/project/{projectId}/storage/file/share/{fileStorageId})
- `EditShare`: added the optional property `currentState/allOf[#/components/schemas/publicCloud.storage.file.FileStorageCurrentState]/status` to the response with the `200` status (PUT /v2/publicCloud/project/{projectId}/storage/file/share/{fileStorageId})
- `GetShare`: added the optional property `currentState/allOf[#/components/schemas/publicCloud.storage.file.FileStorageCurrentState]/attachments` to the response with the `200` status (GET /v2/publicCloud/project/{projectId}/storage/file/share/{fileStorageId})
- `GetShare`: added the optional property `currentState/allOf[#/components/schemas/publicCloud.storage.file.FileStorageCurrentState]/status` to the response with the `200` status (GET /v2/publicCloud/project/{projectId}/storage/file/share/{fileStorageId})

</details>

## 0.2.1 (2026-11-12)

First release.

Generated from the public OVHcloud API surface only.

- `ovh.NewClient()` reads OAuth2 service-account credentials and the region from
  `OVH_CLIENT_ID`, `OVH_CLIENT_SECRET` and `OVH_REGION`; options override them. One client
  serves every section: `vrack.New(client).ListVracks(ctx)`.
- Targets the public API hosts: `https://api.eu.ovhcloud.com`, `https://api.ca.ovhcloud.com`
  and `https://api.us.ovhcloud.com`. `ovh.WithEndpoint` sets another one.
- Only the operations the API marks production are generated: beta and alpha ones can change or
  disappear without notice, so call them with `Client.Do`. Deprecated operations stay until the
  API removes them.
- Names follow the Go conventions: packages are lower case (`pkg/publiccloud`), initialisms are
  in one case (`ID`, `IP`, `URL`, `SSH`: `ListSSHKeys`, `CloudProjectInstance.ID`), and enum
  constants are named after their type and value in mixed caps
  (`AbuseReportCategoryEnumNetworkAttack`).
- Operations are named from their routes (`ListVracks`, `GetVrack`, `CreateCloudProject`);
  each method's documentation states its route, then the API's description of the operation,
  each argument, the IAM actions it requires, and whether it is deprecated. Optional
  parameters are a trailing `XxxParams` struct that can be omitted.
- Lists paginated by cursor (most v2 lists) are only iterators, `AllX`: they fetch the next
  page when the loop reaches it, and nothing collects every page implicitly.
- `AllX` iterators (`iter.Seq2[*T, error]`, one object per step): for v2 lists, fetching the
  next page only when needed; for v1 lists that return identifiers, fetching each object with
  its `GetX` operation (a 404 in between is skipped). They replace the `XExpanded` methods: the
  v1 expanded listing they relied on is not available on the public API cluster.
- Errors are `*ovh.APIError`, whose message is the API's own. `errors.Is` matches them with
  `ovh.ErrNotFound`, `ovh.ErrForbidden`, `ovh.ErrUnauthorized`, `ovh.ErrBadRequest`,
  `ovh.ErrConflict` and `ovh.ErrTooManyRequests`.
- An empty path parameter is refused before the request is sent, rather than calling another
  route.
- `Client.Do` calls any endpoint directly. Its `Request.Path` must start with `/`: it is
  appended to the endpoint as text, so anything else could send the request, bearer token
  included, to another host.
- `index/<section>.jsonl` lists every method with its route, kind, summary and signature, one
  JSON object per line, to find the method for a route with `grep`. It is generated from the
  code at each release.
- `AGENTS.md` and `llms.txt` at the root of the module give AI coding agents the SDK's
  conventions and where to look things up.
- Options: `WithTimeout`, opt-in `WithRetry`, `WithMaxResponseBytes`, custom CAs, mutual TLS,
  explicit proxy.
- Retries (`WithRetry`) are bounded: at most 3 attempts (`MaxRetryAttempts`), never sooner than
  3 s apart (`MinRetryDelay`, also applied to a shorter `Retry-After`), a 60 s cap per wait
  (`MaxRetryDelay`) and 2 minutes of waiting per call (`MaxRetryWait`). A policy outside them
  makes `NewClient` fail. Only `GET`, `HEAD` and `DELETE` are retried, on 429, 503 and 504:
  `PUT` is not, since on this API a `PUT` can start a task or place an order.
- Every request sends `User-Agent: ovhcloud-go-sdk/<version> (<go version>; <os>/<arch>)`.
  `WithUserAgent` appends the caller's product to it; it cannot be replaced. `ovh.Version`
  holds the SDK version.
- Debug output redacts credential-bearing headers and omits bodies unless `WithDebugBodies`.
