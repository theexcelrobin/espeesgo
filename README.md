# espeesgo

[![Go Reference](https://pkg.go.dev/badge/github.com/theexcelrobin/espeesgo.svg)](https://pkg.go.dev/github.com/theexcelrobin/espeesgo)
[![CI](https://github.com/theexcelrobin/espeesgo/actions/workflows/ci.yml/badge.svg)](https://github.com/theexcelrobin/espeesgo/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A Go client for the [Espees](https://developers.espees.org/admin/documentation) API.
Standard library only, Go 1.22 or newer.

| Suite | Methods |
|---|---|
| Merchant payments | `CreatePayment`, `CreateProduct`, `PaymentURL`, `ConfirmPayment`, `WaitForPayment` |
| Vending (liquidity partners) | `VendEspees`, `CreateVendingToken`, `Vend`, `GenerateVendingHash` |

## Install

```sh
go get github.com/theexcelrobin/espeesgo
```

## Accept a payment

```go
client := espeesgo.New(espeesgo.WithAPIKey(os.Getenv("ESPEES_API_KEY")))

payment, err := client.CreatePayment(ctx, espeesgo.Product{
	SKU:            "ORDER-1001",
	Narration:      "Order 1001",
	Price:          0.1, // ESP
	MerchantWallet: merchantWallet,
	SuccessURL:     "https://example.com/success",
	FailURL:        "https://example.com/failure",
})
// save payment.Ref, then redirect the buyer to payment.URL
```

When the buyer returns, confirm the payment on your server:

```go
status, err := client.ConfirmPayment(ctx, ref)
if err != nil { ... }

if status.Approved() && status.Matches("ORDER-1001", 0.1) {
	// deliver the goods
}
```

Never trust the redirect to `success_url` on its own, and always check `Matches`:
a buyer could otherwise pay for a cheap product and claim an expensive one.

Statuses are `APPROVED`, `DECLINE`, `PENDING` and `NOT FOUND`. Use `Approved()`,
`Declined()`, `Pending()`, `NotFound()` and `Final()`, or call
`WaitForPayment(ctx, ref, interval)` to poll until the payment is approved or
declined. Pass a context with a deadline.

## Vend Espees

For liquidity partners. Each vend needs a fresh single-use token, and
`VendEspees` handles both steps:

```go
res, err := client.VendEspees(ctx, agentWallet, agentPIN, userWallet, 10)
// res.TransactionID
```

Or do it step by step with `GenerateVendingHash`, `CreateVendingToken` and `Vend`.

A vend is never retried automatically. If it fails with a network error, check
the wallet before vending again, or you may pay twice. The wallet PIN is
redacted from `String()`, `%v` and `%#v` output of `VendingTokenRequest`.

## Errors

```go
status, err := client.ConfirmPayment(ctx, ref)

if errors.Is(err, espeesgo.ErrUnauthorized) { ... }

var apiErr *espeesgo.APIError
if errors.As(err, &apiErr) {
	log.Println(apiErr.Status, apiErr.Message)
}
```

Espees puts a `statusCode` in its response bodies, so a failure can arrive
inside an HTTP 200. Those come back as an `*APIError` as well, so
one check covers both. `WithMaxRetries(n)` retries 429 responses only.

## Notes

- Keep the API key on your server. Do not put it in a browser or mobile app.
- Prices are `float64`, matching the API. Compare with `Matches`, not `==`.
- The documentation does not describe error responses or a test host, so error
  parsing is defensive (it reads `message`, `error`, `msg` or `detail`, or falls
  back to the raw body). Point the client at another host with `WithAPIURL` and
  `WithCheckoutURL` if Espees gives you a sandbox.
- `TransactionTime()` reads Espees' `DD-MM-YYYY H:MM:SS` date as UTC, as no
  zone is documented.

The package is `v0.x`: the API can change between minor versions.
See the [changelog](CHANGELOG.md).

## License

[MIT](LICENSE)
