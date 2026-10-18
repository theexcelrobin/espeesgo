package espeesgo

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	ErrNoAPIKey        = errors.New("espeesgo: api key is not configured")
	ErrInvalidArgument = errors.New("espeesgo: invalid argument")
)

// APIError is returned when Espees reports a failure, either through the HTTP
// status or through the statusCode field of the response body.
type APIError struct {
	Status     int
	Message    string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return "espeesgo: status " + strconv.Itoa(e.Status)
	}
	return fmt.Sprintf("espeesgo: status %d: %s", e.Status, e.Message)
}

// Is lets errors.Is match an APIError against the sentinels below by status.
func (e *APIError) Is(target error) bool {
	t, ok := target.(*APIError)
	return ok && t.Status != 0 && t.Status == e.Status
}

var (
	ErrBadRequest   = &APIError{Status: http.StatusBadRequest}
	ErrUnauthorized = &APIError{Status: http.StatusUnauthorized}
	ErrForbidden    = &APIError{Status: http.StatusForbidden}
	ErrNotFound     = &APIError{Status: http.StatusNotFound}
	ErrRateLimited  = &APIError{Status: http.StatusTooManyRequests}
)

type envelope struct {
	StatusCode   json.RawMessage `json:"statusCode"`
	Message      string          `json:"message"`
	Error        json.RawMessage `json:"error"`
	Msg          string          `json:"msg"`
	Detail       string          `json:"detail"`
	ErrorType    string          `json:"errorType"`
	ErrorMessage string          `json:"errorMessage"`
}

func (e envelope) status() (int, bool) {
	s := strings.Trim(strings.TrimSpace(string(e.StatusCode)), `"`)
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func (e envelope) text() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Msg != "" {
		return e.Msg
	}
	if e.Detail != "" {
		return e.Detail
	}
	var s string
	if json.Unmarshal(e.Error, &s) == nil {
		return s
	}
	return ""
}

func newAPIError(status int, header http.Header, body []byte) *APIError {
	e := &APIError{Status: status}

	var env envelope
	if err := json.Unmarshal(body, &env); err == nil {
		e.Message = env.text()
	} else {
		e.Message = snippet(body)
	}

	if secs, err := strconv.Atoi(header.Get("Retry-After")); err == nil && secs > 0 {
		e.RetryAfter = time.Duration(secs) * time.Second
	}
	return e
}

func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

func invalidArg(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidArgument, fmt.Sprintf(format, args...))
}
