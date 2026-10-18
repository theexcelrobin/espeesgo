package espeesgo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func validProduct() Product {
	return Product{
		SKU: "SKU123ABC", Narration: "Sample", Price: 0.1, MerchantWallet: "wallet-1",
		SuccessURL: "https://example.com/success ", FailURL: "https://example.com/failure",
		UserData: map[string]any{"param1": "value1"},
	}
}

func TestCreatePaymentV2(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/payment/product" {
			t.Errorf("path = %s", r.URL.Path)
		}
		want := map[string]any{
			"product_sku": "SKU123ABC", "narration": "Sample", "price": 0.1, "merchant_wallet": "wallet-1",
			"success_url": "https://example.com/success", "fail_url": "https://example.com/failure",
			"user_data": map[string]any{"param1": "value1"},
		}
		if got := readJSON(t, r); !reflect.DeepEqual(got, want) {
			t.Errorf("body = %v", got)
		}
		writeJSON(w, 200, `{"statusCode":200,"payment_ref":"61a2","message":"Successfully Done"}`)
	}, WithAPIVersion(APIv2))

	p, err := c.CreatePayment(context.Background(), validProduct())
	if err != nil {
		t.Fatal(err)
	}
	if p.Ref != "61a2" || p.URL != "https://payment.espees.org/pay/61a2" {
		t.Fatalf("payment = %+v", p)
	}
}

func TestCreateProductOmitsEmptyUserData(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, ok := readJSON(t, r)["user_data"]; ok {
			t.Error("user_data must be omitted")
		}
		writeJSON(w, 200, `{"payment_ref":"r"}`)
	})
	p := validProduct()
	p.UserData = nil
	if _, err := c.CreateProduct(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func TestCreateProductMissingRef(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, `{"message":"ok"}`) })
	if _, err := c.CreateProduct(context.Background(), validProduct()); err == nil {
		t.Fatal("want error for missing payment_ref")
	}
}

func TestProductValidation(t *testing.T) {
	mutate := func(f func(*Product)) Product { p := validProduct(); f(&p); return p }
	cases := map[string]Product{
		"no sku":         mutate(func(p *Product) { p.SKU = " " }),
		"no narration":   mutate(func(p *Product) { p.Narration = "" }),
		"zero price":     mutate(func(p *Product) { p.Price = 0 }),
		"negative price": mutate(func(p *Product) { p.Price = -1 }),
		"no wallet":      mutate(func(p *Product) { p.MerchantWallet = "" }),
		"bad success":    mutate(func(p *Product) { p.SuccessURL = "javascript:alert(1)" }),
		"relative fail":  mutate(func(p *Product) { p.FailURL = "/failure" }),
	}
	c := New(WithAPIKey("k"))
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := c.CreateProduct(context.Background(), p); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestPaymentURLEscapesRef(t *testing.T) {
	c := New(WithCheckoutURL("https://pay.test/"))
	if got := c.PaymentURL("a/b c"); got != "https://pay.test/pay/a%2Fb%20c" {
		t.Fatalf("got %s", got)
	}
}

const approvedBody = `{"customer_username":"john_doe","product_sku":"SKU123ABC","narration":"Product description","price":0.1,
"transaction_status":"APPROVED","status_details":"Successfully Done","transaction_date":"18-01-2023 5:50:23","user_data":{"param1":"value1"}}`

func TestConfirmPaymentV2(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/payment/confirm/" || readJSON(t, r)["payment_ref"] != "ref1" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		writeJSON(w, 200, approvedBody)
	}, WithAPIVersion(APIv2))
	st, err := c.ConfirmPayment(context.Background(), "ref1")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Approved() || !st.Final() || st.Pending() || st.CustomerUsername != "john_doe" || st.UserData["param1"] != "value1" {
		t.Fatalf("status = %+v", st)
	}
	if !st.Matches("SKU123ABC", 0.1) || st.Matches("OTHER", 0.1) || st.Matches("SKU123ABC", 0.01) {
		t.Fatal("Matches misbehaves")
	}
	ts, ok := st.TransactionTime()
	if !ok || !ts.Equal(time.Date(2023, 1, 18, 5, 50, 23, 0, time.UTC)) {
		t.Fatalf("time = %v ok=%v", ts, ok)
	}
	if _, err := c.ConfirmPayment(context.Background(), " "); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("got %v", err)
	}
}

func TestStatusHelpers(t *testing.T) {
	tests := []struct {
		raw                                    TransactionStatus
		approved, declined, pending, nf, final bool
	}{
		{"APPROVED", true, false, false, false, true},
		{"DECLINE", false, true, false, false, true},
		{"PENDING", false, false, true, false, false},
		{"NOT FOUND", false, false, false, true, false},
		{" approved ", true, false, false, false, true},
		{"", false, false, false, false, false},
	}
	for _, tt := range tests {
		s := PaymentStatus{TransactionStatus: tt.raw}
		if s.Approved() != tt.approved || s.Declined() != tt.declined || s.Pending() != tt.pending ||
			s.NotFound() != tt.nf || s.Final() != tt.final {
			t.Errorf("%q: unexpected helpers", tt.raw)
		}
	}
	if _, ok := (PaymentStatus{TransactionDate: "garbage"}).TransactionTime(); ok {
		t.Error("garbage date must not parse")
	}
}

func TestWaitForPayment(t *testing.T) {
	var calls atomic.Int32
	statuses := []string{"NOT FOUND", "PENDING", "DECLINE"}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"transaction_status":"`+statuses[calls.Add(1)-1]+`"}`)
	})
	st, err := c.WaitForPayment(context.Background(), "ref", time.Millisecond)
	if err != nil || !st.Declined() || calls.Load() != 3 {
		t.Fatalf("st=%+v err=%v calls=%d", st, err, calls.Load())
	}
}

func TestWaitForPaymentContextCancel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"transaction_status":"PENDING"}`)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	st, err := c.WaitForPayment(ctx, "ref", time.Hour)
	if !errors.Is(err, context.DeadlineExceeded) || st == nil || !st.Pending() {
		t.Fatalf("st=%+v err=%v", st, err)
	}
}

func TestWaitForPaymentStopsOnAPIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 401, `{"message":"bad key"}`) })
	if _, err := c.WaitForPayment(context.Background(), "ref", time.Millisecond); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
}

func TestCreatePaymentV1NeedsNoKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/payment/product" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "" {
			t.Errorf("no key was configured, but x-api-key = %q", got)
		}
		if got := readJSON(t, r); got["product_sku"] != "SKU123ABC" || got["merchant_wallet"] != "wallet-1" {
			t.Errorf("body = %v", got)
		}
		writeJSON(w, 200, `{"statusCode": 200, "productid": "45ce48d7", "message": "Successfully Done"}`)
	}))
	defer srv.Close()

	p, err := New(WithAPIURL(srv.URL)).CreatePayment(context.Background(), validProduct())
	if err != nil {
		t.Fatal(err)
	}
	if p.Ref != "45ce48d7" || p.URL != "https://payment.espees.org/pay/45ce48d7" {
		t.Fatalf("payment = %+v", p)
	}
}

func TestV1SendsTheKeyWhenOneIsSet(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("a configured key should still be sent")
		}
		writeJSON(w, 200, `{"productid": "r"}`)
	})
	if _, err := c.CreateProduct(context.Background(), validProduct()); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmPaymentV1ReadsStringPrices(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/payment/confirm" || readJSON(t, r)["product_id"] != "ref1" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		writeJSON(w, 200, `{"customer_username":"john_doe","product_sku":"SKU123ABC","narration":"n","price":"0.10","transaction_status":"APPROVED","status_details":"Successfully Done","transaction_date":"18-01-2023 5:50:23","user_data":{}}`)
	})
	st, err := c.ConfirmPayment(context.Background(), "ref1")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Approved() || st.Price != 0.1 || !st.Matches("SKU123ABC", 0.1) {
		t.Fatalf("status = %+v", st)
	}
}

func TestConfirmPaymentV1NotFoundHasNoOtherFields(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, `{"transaction_status": "NOT FOUND"}`) })
	st, err := c.ConfirmPayment(context.Background(), "ref1")
	if err != nil {
		t.Fatal(err)
	}
	if !st.NotFound() || st.Final() || st.Price != 0 {
		t.Fatalf("status = %+v", st)
	}
}

func TestPriceMayBeANumberOrMissing(t *testing.T) {
	for body, want := range map[string]float64{
		`{"price": 1.5}`:      1.5,
		`{"price": "2.25"}`:   2.25,
		`{"price": null}`:     0,
		`{"price": ""}`:       0,
		`{"product_sku":"x"}`: 0,
	} {
		var st PaymentStatus
		if err := json.Unmarshal([]byte(body), &st); err != nil || st.Price != want {
			t.Errorf("%s: price %v, err %v", body, st.Price, err)
		}
	}
	var st PaymentStatus
	if err := json.Unmarshal([]byte(`{"price": "abc"}`), &st); err == nil {
		t.Error("a price that is not a number must be an error")
	}
}

func TestEspeesCrashInsideA200BecomesAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"errorMessage": "'product_id'", "errorType": "KeyError", "stackTrace": ["x"]}`)
	})
	_, err := c.ConfirmPayment(context.Background(), "ref1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadGateway || apiErr.Message != "KeyError: 'product_id'" {
		t.Fatalf("got %v", err)
	}
}

func TestVersionOptionIgnoresUnknownValues(t *testing.T) {
	if New().version != APIv1 || New(WithAPIVersion(7)).version != APIv1 || New(WithAPIVersion(APIv2)).version != APIv2 {
		t.Fatal("v1 is the default and only 1 or 2 are accepted")
	}
}
