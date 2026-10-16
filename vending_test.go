package espeesgo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestGenerateVendingHash(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		h, err := GenerateVendingHash()
		if err != nil {
			t.Fatal(err)
		}
		if !validVendingHash(h) {
			t.Fatalf("invalid hash %q", h)
		}
		seen[h] = true
	}
	if len(seen) != 200 {
		t.Fatalf("hashes collided: %d unique of 200", len(seen))
	}
}

func TestCreateVendingToken(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b := readJSON(t, r)
		if r.URL.Path != "/agents/vending/createtoken" || b["vending_wallet_address"] != "agent-wallet" ||
			b["vending_wallet_pin"] != "1234" || b["vending_hash"] != "ABCDEFGH12345678" {
			t.Errorf("unexpected %s %v", r.URL.Path, b)
		}
		writeJSON(w, 200, `{"statusCode":200,"vending_token":"tok1","message":"Successfully Done"}`)
	})
	tok, err := c.CreateVendingToken(context.Background(), VendingTokenRequest{
		WalletAddress: "agent-wallet", WalletPIN: "1234", Hash: "ABCDEFGH12345678",
	})
	if err != nil || tok.Token != "tok1" {
		t.Fatalf("tok=%+v err=%v", tok, err)
	}
}

func TestCreateVendingTokenValidation(t *testing.T) {
	c := New(WithAPIKey("k"))
	good := VendingTokenRequest{WalletAddress: "w", WalletPIN: "1", Hash: "ABCDEFGH12345678"}
	cases := map[string]VendingTokenRequest{
		"no wallet":  {WalletPIN: "1", Hash: good.Hash},
		"no pin":     {WalletAddress: "w", Hash: good.Hash},
		"short hash": {WalletAddress: "w", WalletPIN: "1", Hash: "short"},
		"bad chars":  {WalletAddress: "w", WalletPIN: "1", Hash: "ABCDEFGH1234567!"},
	}
	for name, r := range cases {
		if _, err := c.CreateVendingToken(context.Background(), r); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestVendingTokenRequestNeverPrintsPIN(t *testing.T) {
	r := VendingTokenRequest{WalletAddress: "w", WalletPIN: "987654", Hash: "ABCDEFGH12345678"}
	for _, out := range []string{fmt.Sprint(r), fmt.Sprintf("%v", r), fmt.Sprintf("%+v", r), fmt.Sprintf("%#v", r), fmt.Sprint(&r)} {
		if strings.Contains(out, "987654") || !strings.Contains(out, "REDACTED") {
			t.Errorf("leaked or unredacted: %s", out)
		}
	}
}

func TestErrorsNeverContainPIN(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 400, `{"message":"wrong pin"}`) })
	_, err := c.CreateVendingToken(context.Background(), VendingTokenRequest{WalletAddress: "w", WalletPIN: "987654", Hash: "ABCDEFGH12345678"})
	if err == nil || strings.Contains(err.Error(), "987654") {
		t.Fatalf("err = %v", err)
	}
}

func TestVend(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b := readJSON(t, r)
		if r.URL.Path != "/v2/vending/vend" || b["vending_token"] != "tok1" || b["user_wallet"] != "user-w" || b["amount_in_espees"] != float64(10) {
			t.Errorf("unexpected %s %v", r.URL.Path, b)
		}
		writeJSON(w, 200, `{"statusCode":200,"message":"Vend successful","transaction_id":"abc123xyz"}`)
	})
	res, err := c.Vend(context.Background(), VendRequest{VendingToken: "tok1", UserWallet: "user-w", Amount: 10})
	if err != nil || res.TransactionID != "abc123xyz" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestVendValidation(t *testing.T) {
	c := New(WithAPIKey("k"))
	for name, r := range map[string]VendRequest{
		"no token":    {UserWallet: "w", Amount: 1},
		"no wallet":   {VendingToken: "t", Amount: 1},
		"zero amount": {VendingToken: "t", UserWallet: "w"},
	} {
		if _, err := c.Vend(context.Background(), r); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestVendFailureInBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"statusCode":400,"message":"Token already used"}`)
	})
	_, err := c.Vend(context.Background(), VendRequest{VendingToken: "t", UserWallet: "w", Amount: 1})
	if !errors.Is(err, ErrBadRequest) || !strings.Contains(err.Error(), "Token already used") {
		t.Fatalf("got %v", err)
	}
}

func TestVendEspees(t *testing.T) {
	var hash string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agents/vending/createtoken":
			hash, _ = readJSON(t, r)["vending_hash"].(string)
			writeJSON(w, 200, `{"vending_token":"tok-fresh"}`)
		case "/v2/vending/vend":
			if readJSON(t, r)["vending_token"] != "tok-fresh" {
				t.Error("vend must use the token just created")
			}
			writeJSON(w, 200, `{"transaction_id":"tx1"}`)
		}
	})
	res, err := c.VendEspees(context.Background(), "agent-w", "1234", "user-w", 2.5)
	if err != nil || res.TransactionID != "tx1" || !validVendingHash(hash) {
		t.Fatalf("res=%+v err=%v hash=%q", res, err, hash)
	}
}

func TestVendEspeesDoesNotVendWhenTokenFails(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/vending/vend" {
			t.Error("vend must not be called")
		}
		writeJSON(w, 401, `{"message":"bad pin"}`)
	})
	if _, err := c.VendEspees(context.Background(), "a", "1", "u", 1); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
	if _, err := c.VendEspees(context.Background(), "a", "1", "u", 0); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("got %v", err)
	}
}
