package espeesgo

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequiresAPIKey(t *testing.T) {
	ctx := context.Background()
	if _, err := New(WithAPIVersion(APIv2)).ConfirmPayment(ctx, "ref"); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("v2 confirm: got %v", err)
	}
	if _, err := New(WithAPIVersion(APIv2)).CreateProduct(ctx, validProduct()); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("v2 create: got %v", err)
	}
	if _, err := New().VendEspees(ctx, "wallet", "1234", "user", 1); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("vending: got %v", err)
	}
}

func TestRequestHeaders(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("x-api-key") != "test-key" ||
			r.Header.Get("Content-Type") != "application/json" || r.Header.Get("User-Agent") != "espeesgo/"+Version {
			t.Errorf("bad request: %s %v", r.Method, r.Header)
		}
		writeJSON(w, 200, `{"transaction_status":"PENDING"}`)
	})
	if _, err := c.ConfirmPayment(context.Background(), "ref"); err != nil {
		t.Fatal(err)
	}
}

func TestErrorParsing(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		header map[string]string
		is     error
		isNot  error
		msg    string
		retry  time.Duration
	}{
		{"http 401 message", 401, `{"message":"Invalid api key"}`, nil, ErrUnauthorized, ErrForbidden, "Invalid api key", 0},
		{"error string", 400, `{"error":"bad sku"}`, nil, ErrBadRequest, ErrNotFound, "bad sku", 0},
		{"body status code in 200", 200, `{"statusCode":403,"message":"Not allowed"}`, nil, ErrForbidden, ErrUnauthorized, "Not allowed", 0},
		{"body status code as string", 200, `{"statusCode":"404","message":"Missing"}`, nil, ErrNotFound, ErrForbidden, "Missing", 0},
		{"plain text", 500, `upstream exploded`, nil, nil, ErrNotFound, "upstream exploded", 0},
		{"rate limit", 429, `{"message":"slow down"}`, map[string]string{"Retry-After": "7"}, ErrRateLimited, ErrForbidden, "slow down", 7 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tt.header {
					w.Header().Set(k, v)
				}
				writeJSON(w, tt.status, tt.body)
			})
			_, err := c.ConfirmPayment(context.Background(), "ref")

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("want *APIError, got %v", err)
			}
			if apiErr.Message != tt.msg || apiErr.RetryAfter != tt.retry {
				t.Fatalf("unexpected error %+v", apiErr)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Fatalf("errors.Is(%v) = false", tt.is)
			}
			if errors.Is(err, tt.isNot) {
				t.Fatalf("must not match %v", tt.isNot)
			}
		})
	}
}

func TestSuccessStatusCodeInBodyIsNotAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"statusCode":200,"payment_ref":"abc","message":"Successfully Done"}`)
	})
	ref, err := c.CreateProduct(context.Background(), validProduct())
	if err != nil || ref != "abc" {
		t.Fatalf("ref=%q err=%v", ref, err)
	}
}

func TestRetryOn429(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			writeJSON(w, 429, `{}`)
			return
		}
		writeJSON(w, 200, `{"transaction_status":"APPROVED"}`)
	}, WithMaxRetries(1))

	st, err := c.ConfirmPayment(context.Background(), "ref")
	if err != nil || !st.Approved() || calls.Load() != 2 {
		t.Fatalf("st=%+v err=%v calls=%d", st, err, calls.Load())
	}
}

func TestNoRetryForServerErrors(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, 500, `{}`)
	}, WithMaxRetries(3))

	if _, err := c.ConfirmPayment(context.Background(), "ref"); err == nil || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestInvalidJSONResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, `<html>`) })
	if _, err := c.ConfirmPayment(context.Background(), "ref"); err == nil {
		t.Fatal("want decode error")
	}
}
