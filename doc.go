// Package espeesgo is a Go client for the Espees API.
//
// It covers the Merchant API, for charging users in Espees (ESP), and the
// Vending API, for liquidity partners who vend Espees to user wallets. It uses
// only the standard library.
//
//	client := espeesgo.New()
//
//	payment, err := client.CreatePayment(ctx, espeesgo.Product{
//		SKU:            "ORDER-1001",
//		Narration:      "Order 1001",
//		Price:          0.1,
//		MerchantWallet: merchantWallet,
//		SuccessURL:     "https://example.com/success",
//		FailURL:        "https://example.com/failure",
//	})
//	// redirect the buyer to payment.URL, then:
//	status, err := client.ConfirmPayment(ctx, payment.Ref)
//
// Always confirm a payment on your server before delivering goods, and check
// the result with PaymentStatus.Matches.
//
// Failed calls return an *APIError. Espees also reports failures in a
// statusCode field of a 200 response, and those are returned as *APIError too.
package espeesgo
