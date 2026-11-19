# Examples

Small programs that use the OVHcloud Go SDK. Each one is a single `main.go` in its own
folder. Read its header comment first: it lists the environment variables it needs.

| Program | What it shows | Changes the account |
|---|---|---|
| [account](account/main.go) | `GET /v1/me`: the details of your account. Start here. | no |
| [list-and-get](list-and-get/main.go) | list with an `AllX` iterator, then get one object | no |
| [create-and-wait](create-and-wait/main.go) | create a Public Cloud network, `ovh.WaitReady`, delete it, `ovh.WaitGone` | yes |
| [dns-record](dns-record/main.go) | create a DNS record, then refresh the zone | yes |
| [cart-order](cart-order/main.go) | order with a cart: add a domain, read the price, stop before payment | no order is placed |
| [errors](errors/main.go) | `errors.Is(err, ovh.ErrNotFound)`, `ovh.ErrForbidden`, `*ovh.APIError` | no |
| [vrack](vrack/main.go) | every vRack, with the Public Cloud projects in it | no |

## Run one

Credentials come from the environment. Never write them in the code.

```shell
export OVH_CLIENT_ID=...  OVH_CLIENT_SECRET=...
export OVH_REGION=eu          # or ca, us
go run ./examples/account     # from the root of this repository
```

`account` is the plain quick start. The other read-only examples create their client with
`ovh.WithReadOnly()`: they cannot change anything, whatever the service account may do. The
examples that change the account run on a test project or a test zone.

## Build them

They are part of the module, so `go build ./...` and `go vet ./...` build them with the SDK:

```shell
go vet ./examples/...
```
