package espeesgo_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/theexcelrobin/espeesgo"
)

func newClient() *espeesgo.Client {
	return espeesgo.New(espeesgo.WithAPIKey(os.Getenv("ESPEES_API_KEY")))
}

func ExampleClient_CreatePayment() {
	payment, err := newClient().CreatePayment(context.Background(), espeesgo.Product{
		SKU:            "ORDER-1001",
		Narration:      "Order 1001",
		Price:          0.1,
		MerchantWallet: os.Getenv("ESPEES_MERCHANT_WALLET"),
		SuccessURL:     "https://example.com/success",
		FailURL:        "https://example.com/failure",
		UserData:       map[string]any{"order_id": 1001},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("send the buyer to", payment.URL)
}

func ExampleClient_PaymentURL() {
	client := espeesgo.New(espeesgo.WithAPIKey("key"))
	fmt.Println(client.PaymentURL("61a28ca663b0f2e92297f110c99176c9"))
	// Output: https://payment.espees.org/pay/61a28ca663b0f2e92297f110c99176c9
}

func ExampleClient_ConfirmPayment() {
	// Call this from your success_url handler, never trust the redirect alone.
	status, err := newClient().ConfirmPayment(context.Background(), "payment-ref-you-saved")
	if err != nil {
		log.Fatal(err)
	}
	if status.Approved() && status.Matches("ORDER-1001", 0.1) {
		fmt.Println("paid by", status.CustomerUsername)
	}
}

func ExampleClient_WaitForPayment() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	status, err := newClient().WaitForPayment(ctx, "payment-ref-you-saved", 5*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(status.TransactionStatus)
}

func ExampleClient_VendEspees() {
	res, err := newClient().VendEspees(context.Background(),
		os.Getenv("ESPEES_AGENT_WALLET"), os.Getenv("ESPEES_AGENT_PIN"), "user-wallet-address", 10)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("vended, transaction", res.TransactionID)
}

func ExampleClient_Vend() {
	client := newClient()
	ctx := context.Background()

	hash, err := espeesgo.GenerateVendingHash()
	if err != nil {
		log.Fatal(err)
	}
	token, err := client.CreateVendingToken(ctx, espeesgo.VendingTokenRequest{
		WalletAddress: os.Getenv("ESPEES_AGENT_WALLET"),
		WalletPIN:     os.Getenv("ESPEES_AGENT_PIN"),
		Hash:          hash,
	})
	if err != nil {
		log.Fatal(err)
	}
	res, err := client.Vend(ctx, espeesgo.VendRequest{VendingToken: token.Token, UserWallet: "user-wallet-address", Amount: 10})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res.TransactionID)
}

func ExampleAPIError() {
	_, err := newClient().ConfirmPayment(context.Background(), "ref")

	var apiErr *espeesgo.APIError
	switch {
	case err == nil:
	case errors.Is(err, espeesgo.ErrUnauthorized):
		fmt.Println("check your API key")
	case errors.As(err, &apiErr):
		fmt.Println(apiErr.Status, apiErr.Message)
	default:
		log.Fatal(err)
	}
}

func ExampleNew() {
	_ = espeesgo.New(
		espeesgo.WithAPIKey(os.Getenv("ESPEES_API_KEY")),
		espeesgo.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
		espeesgo.WithMaxRetries(2),
	)
}
