# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/). Until v1.0.0 the API can change
between minor versions.

## [0.2.0] - 2026-10-18

### Added
- `WithAPIVersion` with `APIv1` and `APIv2`. APIv1 is the default and uses
  `/payment/product` and `/payment/confirm`, which work without an API key.
  APIv2 keeps the `/v2/payment/...` endpoints from the documentation and still
  requires a key.
- `PaymentStatus` reads `price` whether Espees sends a number or a string.

### Changed
- Merchant payment calls no longer fail with `ErrNoAPIKey` on APIv1. The key is
  sent when one is set. Vending and APIv2 still require it.
- Creating a product accepts the reference as `productid` (v1) or `payment_ref`
  (v2). Confirming sends `product_id` on v1 and `payment_ref` on v2.
- A failure Espees reports inside a 200 response as `errorMessage` and
  `errorType` is returned as an `*APIError` with status 502.

### Fixed
- Payments failed with 403 Forbidden because the v2 endpoints rejected keys that
  the v1 endpoints never needed.

## [0.1.1] - 2026-10-16

### Fixed
- Satisfy staticcheck (S1016) so CI is green. No behaviour change.

## [0.1.0] - 2026-10-16

### Added
- Merchant API: `CreateProduct`, `CreatePayment`, `PaymentURL`, `ConfirmPayment`,
  `WaitForPayment`, and `PaymentStatus` helpers including `Matches`.
- Vending API: `CreateVendingToken`, `Vend`, `VendEspees`, `GenerateVendingHash`.
- Typed `*APIError` with `errors.Is` sentinels, covering failures reported in
  the `statusCode` field of a 200 response.
- Client options for API key, hosts, HTTP client, timeout and 429 retries.
