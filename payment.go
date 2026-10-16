package espeesgo

import (
	"context"
	"math"
	"net/url"
	"strings"
	"time"
)

const (
	productPath = "/v2/payment/product"
	confirmPath = "/v2/payment/confirm/"

	defaultPollInterval = 3 * time.Second
	minPollInterval     = time.Second
)

// Product describes what a buyer is charged for.
type Product struct {
	// SKU is a unique ID for this charge. Generate it in your backend.
	SKU       string
	Narration string
	// Price is the amount in Espees (ESP).
	Price          float64
	MerchantWallet string
	// SuccessURL and FailURL are where the buyer lands after paying.
	SuccessURL string
	FailURL    string
	// UserData is optional metadata returned when the payment is confirmed.
	UserData map[string]any
}

type productRequest struct {
	SKU            string         `json:"product_sku"`
	Narration      string         `json:"narration"`
	Price          float64        `json:"price"`
	MerchantWallet string         `json:"merchant_wallet"`
	SuccessURL     string         `json:"success_url"`
	FailURL        string         `json:"fail_url"`
	UserData       map[string]any `json:"user_data,omitempty"`
}

type productResponse struct {
	PaymentRef string `json:"payment_ref"`
	Message    string `json:"message"`
}

// Payment is a created charge waiting for the buyer.
type Payment struct {
	// Ref identifies the payment. Keep it to confirm the payment later.
	Ref string
	// URL is the checkout page to send the buyer to.
	URL string
}

// CreateProduct registers a charge and returns its payment reference.
func (c *Client) CreateProduct(ctx context.Context, p Product) (string, error) {
	req, err := p.request()
	if err != nil {
		return "", err
	}
	var res productResponse
	if err := c.post(ctx, productPath, req, &res); err != nil {
		return "", err
	}
	if res.PaymentRef == "" {
		return "", &APIError{Status: 200, Message: "response has no payment_ref"}
	}
	return res.PaymentRef, nil
}

// CreatePayment registers a charge and returns it with its checkout URL.
func (c *Client) CreatePayment(ctx context.Context, p Product) (*Payment, error) {
	ref, err := c.CreateProduct(ctx, p)
	if err != nil {
		return nil, err
	}
	return &Payment{Ref: ref, URL: c.PaymentURL(ref)}, nil
}

// PaymentURL is the checkout page for a payment reference.
func (c *Client) PaymentURL(ref string) string {
	return c.checkoutURL + "/pay/" + url.PathEscape(ref)
}

func (p Product) request() (*productRequest, error) {
	r := &productRequest{
		SKU:            strings.TrimSpace(p.SKU),
		Narration:      strings.TrimSpace(p.Narration),
		Price:          p.Price,
		MerchantWallet: strings.TrimSpace(p.MerchantWallet),
		SuccessURL:     strings.TrimSpace(p.SuccessURL),
		FailURL:        strings.TrimSpace(p.FailURL),
		UserData:       p.UserData,
	}
	switch {
	case r.SKU == "":
		return nil, invalidArg("product SKU is required")
	case r.Narration == "":
		return nil, invalidArg("narration is required")
	case !validAmount(r.Price):
		return nil, invalidArg("price must be greater than zero")
	case r.MerchantWallet == "":
		return nil, invalidArg("merchant wallet is required")
	case !validHTTPURL(r.SuccessURL):
		return nil, invalidArg("success URL must be an http or https URL")
	case !validHTTPURL(r.FailURL):
		return nil, invalidArg("fail URL must be an http or https URL")
	}
	return r, nil
}

func validAmount(v float64) bool {
	return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v)
}

func validHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// TransactionStatus is the state of a payment.
type TransactionStatus string

const (
	StatusApproved TransactionStatus = "APPROVED"
	StatusDeclined TransactionStatus = "DECLINE"
	StatusPending  TransactionStatus = "PENDING"
	StatusNotFound TransactionStatus = "NOT FOUND"
)

// PaymentStatus is the result of confirming a payment.
type PaymentStatus struct {
	CustomerUsername  string            `json:"customer_username"`
	ProductSKU        string            `json:"product_sku"`
	Narration         string            `json:"narration"`
	Price             float64           `json:"price"`
	TransactionStatus TransactionStatus `json:"transaction_status"`
	StatusDetails     string            `json:"status_details"`
	// TransactionDate is the date as Espees sends it, DD-MM-YYYY H:MM:SS.
	TransactionDate string         `json:"transaction_date"`
	UserData        map[string]any `json:"user_data"`
}

func (s PaymentStatus) is(want TransactionStatus) bool {
	return strings.EqualFold(strings.TrimSpace(string(s.TransactionStatus)), string(want))
}

// Approved reports that the payment was successful.
func (s PaymentStatus) Approved() bool { return s.is(StatusApproved) }

// Declined reports that the payment failed.
func (s PaymentStatus) Declined() bool { return s.is(StatusDeclined) }

// Pending reports that the payment is still processing.
func (s PaymentStatus) Pending() bool { return s.is(StatusPending) }

// NotFound reports that no payment attempt exists yet.
func (s PaymentStatus) NotFound() bool { return s.is(StatusNotFound) }

// Final reports that the payment can no longer change.
func (s PaymentStatus) Final() bool { return s.Approved() || s.Declined() }

// Matches reports whether the confirmed payment is for the given SKU and
// price. Check it before delivering goods, so a payment for a cheaper product
// cannot be passed off as yours.
func (s PaymentStatus) Matches(sku string, price float64) bool {
	return s.ProductSKU == sku && math.Abs(s.Price-price) < 1e-9
}

// TransactionTime parses TransactionDate. Espees does not state a time zone,
// so the time is read as UTC.
func (s PaymentStatus) TransactionTime() (time.Time, bool) {
	t, err := time.Parse("02-01-2006 15:04:05", strings.TrimSpace(s.TransactionDate))
	return t, err == nil
}

// ConfirmPayment asks Espees for the state of a payment.
func (c *Client) ConfirmPayment(ctx context.Context, ref string) (*PaymentStatus, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, invalidArg("payment ref is required")
	}
	var st PaymentStatus
	body := map[string]string{"payment_ref": ref}
	if err := c.post(ctx, confirmPath, body, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// WaitForPayment confirms a payment every interval until it is approved or
// declined. A payment that is pending or not found yet keeps being polled. When
// the context ends it returns the last status seen, which can be nil, with the
// context error. An interval below one second is raised to one second and zero
// means three seconds.
func (c *Client) WaitForPayment(ctx context.Context, ref string, interval time.Duration) (*PaymentStatus, error) {
	if interval == 0 {
		interval = defaultPollInterval
	}
	interval = max(interval, minPollInterval)

	var last *PaymentStatus
	for {
		st, err := c.ConfirmPayment(ctx, ref)
		if err != nil {
			return last, err
		}
		last = st
		if st.Final() {
			return st, nil
		}

		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(interval):
		}
	}
}
