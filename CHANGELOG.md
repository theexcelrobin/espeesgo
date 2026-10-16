# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/). Until v1.0.0 the API can change
between minor versions.

## [0.1.0] - 2026-10-16

### Added
- Merchant API: `CreateProduct`, `CreatePayment`, `PaymentURL`, `ConfirmPayment`,
  `WaitForPayment`, and `PaymentStatus` helpers including `Matches`.
- Vending API: `CreateVendingToken`, `Vend`, `VendEspees`, `GenerateVendingHash`.
- Typed `*APIError` with `errors.Is` sentinels, covering failures reported in
  the `statusCode` field of a 200 response.
- Client options for API key, hosts, HTTP client, timeout and 429 retries.
