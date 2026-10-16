package espeesgo

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
)

const (
	vendingTokenPath = "/agents/vending/createtoken"
	vendPath         = "/v2/vending/vend"

	vendingHashLength = 16
	hashAlphabet      = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// VendingTokenRequest creates a single-use vending token.
type VendingTokenRequest struct {
	WalletAddress string
	// WalletPIN is the vending wallet's PIN. It never appears in the
	// String, GoString or error output of this package.
	WalletPIN string
	// Hash must be a unique 16-character alphanumeric string. Use
	// GenerateVendingHash.
	Hash string
}

func (r VendingTokenRequest) String() string {
	return fmt.Sprintf("VendingTokenRequest{WalletAddress:%q WalletPIN:[REDACTED] Hash:%q}", r.WalletAddress, r.Hash)
}

func (r VendingTokenRequest) GoString() string { return r.String() }

type vendingTokenWire struct {
	WalletAddress string `json:"vending_wallet_address"`
	WalletPIN     string `json:"vending_wallet_pin"`
	Hash          string `json:"vending_hash"`
}

// VendingToken is a single-use token that authorises one vend.
type VendingToken struct {
	Token   string `json:"vending_token"`
	Message string `json:"message"`
}

// GenerateVendingHash returns a random 16-character alphanumeric string
// suitable for VendingTokenRequest.Hash.
func GenerateVendingHash() (string, error) {
	const limit = 256 - 256%len(hashAlphabet)

	out := make([]byte, 0, vendingHashLength)
	buf := make([]byte, vendingHashLength*2)
	for len(out) < vendingHashLength {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) >= limit {
				continue
			}
			out = append(out, hashAlphabet[int(b)%len(hashAlphabet)])
			if len(out) == vendingHashLength {
				break
			}
		}
	}
	return string(out), nil
}

func validVendingHash(h string) bool {
	if len(h) != vendingHashLength {
		return false
	}
	for _, r := range h {
		if !strings.ContainsRune(hashAlphabet, r) {
			return false
		}
	}
	return true
}

// CreateVendingToken creates a token for one vend. Tokens are single use, so
// create a new one per transaction.
func (c *Client) CreateVendingToken(ctx context.Context, r VendingTokenRequest) (*VendingToken, error) {
	switch {
	case strings.TrimSpace(r.WalletAddress) == "":
		return nil, invalidArg("vending wallet address is required")
	case r.WalletPIN == "":
		return nil, invalidArg("vending wallet PIN is required")
	case !validVendingHash(r.Hash):
		return nil, invalidArg("vending hash must be 16 alphanumeric characters")
	}

	wire := vendingTokenWire{WalletAddress: r.WalletAddress, WalletPIN: r.WalletPIN, Hash: r.Hash}
	var tok VendingToken
	if err := c.post(ctx, vendingTokenPath, wire, &tok); err != nil {
		return nil, err
	}
	if tok.Token == "" {
		return nil, &APIError{Status: 200, Message: "response has no vending_token"}
	}
	return &tok, nil
}

// VendRequest sends Espees from the vending wallet to a user.
type VendRequest struct {
	VendingToken string  `json:"vending_token"`
	UserWallet   string  `json:"user_wallet"`
	Amount       float64 `json:"amount_in_espees"`
}

// VendResult is the outcome of a vend.
type VendResult struct {
	TransactionID string `json:"transaction_id"`
	Message       string `json:"message"`
}

// Vend spends a vending token to send Espees to a user wallet. A vend is not
// retried automatically, so after a network error check the wallet before
// vending again.
func (c *Client) Vend(ctx context.Context, r VendRequest) (*VendResult, error) {
	switch {
	case strings.TrimSpace(r.VendingToken) == "":
		return nil, invalidArg("vending token is required")
	case strings.TrimSpace(r.UserWallet) == "":
		return nil, invalidArg("user wallet is required")
	case !validAmount(r.Amount):
		return nil, invalidArg("amount must be greater than zero")
	}

	var res VendResult
	if err := c.post(ctx, vendPath, r, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// VendEspees creates a fresh vending token and vends in one call.
func (c *Client) VendEspees(ctx context.Context, agentWallet, agentPIN, userWallet string, amount float64) (*VendResult, error) {
	if !validAmount(amount) {
		return nil, invalidArg("amount must be greater than zero")
	}
	hash, err := GenerateVendingHash()
	if err != nil {
		return nil, err
	}
	tok, err := c.CreateVendingToken(ctx, VendingTokenRequest{WalletAddress: agentWallet, WalletPIN: agentPIN, Hash: hash})
	if err != nil {
		return nil, err
	}
	return c.Vend(ctx, VendRequest{VendingToken: tok.Token, UserWallet: userWallet, Amount: amount})
}
