package floop

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient wires the Client at the httptest server's URL and returns
// both together so tests can enqueue responses + inspect requests.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewClient("flp_test", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, srv
}

func TestNewClient_RequiresAPIKey(t *testing.T) {
	if _, err := NewClient(""); err == nil {
		t.Fatal("expected error for empty apiKey, got nil")
	}
}

func TestRequest_BearerAndUserAgent(t *testing.T) {
	var seenAuth, seenUA, seenAccept string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenUA = r.Header.Get("User-Agent")
		seenAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"ok":true}}`))
	})
	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.request(context.Background(), "GET", "/api/v1/ping", nil, &out); err != nil {
		t.Fatalf("request: %v", err)
	}
	if seenAuth != "Bearer flp_test" {
		t.Errorf("Authorization: got %q, want Bearer flp_test", seenAuth)
	}
	if !strings.HasPrefix(seenUA, "floop-go-sdk/") {
		t.Errorf("User-Agent: got %q, want floop-go-sdk/*", seenUA)
	}
	if seenAccept != "application/json" {
		t.Errorf("Accept: got %q", seenAccept)
	}
	if !out.OK {
		t.Errorf("did not unwrap data envelope: out=%+v", out)
	}
}

func TestRequest_PostBodyAndContentType(t *testing.T) {
	var seenCT string
	var seenBody []byte
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenCT = r.Header.Get("Content-Type")
		seenBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(`{"data":{}}`))
	})
	if err := c.request(context.Background(), "POST", "/x", map[string]any{"foo": 1}, nil); err != nil {
		t.Fatalf("request: %v", err)
	}
	if seenCT != "application/json" {
		t.Errorf("Content-Type: got %q", seenCT)
	}
	if !strings.Contains(string(seenBody), `"foo":1`) {
		t.Errorf("body mismatch: got %q", string(seenBody))
	}
}

func TestRequest_ErrorEnvelopeBecomesTypedError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req_abc")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"no such project"}}`))
	})
	err := c.request(context.Background(), "GET", "/x", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var fe *Error
	if !errors.As(err, &fe) {
		t.Fatalf("expected *Error, got %T (%v)", err, err)
	}
	if fe.Code != "NOT_FOUND" {
		t.Errorf("Code: got %q", fe.Code)
	}
	if fe.Status != 404 {
		t.Errorf("Status: got %d", fe.Status)
	}
	if fe.RequestID != "req_abc" {
		t.Errorf("RequestID: got %q", fe.RequestID)
	}
	if fe.Message != "no such project" {
		t.Errorf("Message: got %q", fe.Message)
	}
}

func TestRequest_RetryAfterSeconds(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":"RATE_LIMITED","message":"slow down"}}`))
	})
	err := c.request(context.Background(), "GET", "/x", nil, nil)
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != "RATE_LIMITED" {
		t.Fatalf("expected RATE_LIMITED, got %v", err)
	}
	if fe.RetryAfter != 5*time.Second {
		t.Errorf("RetryAfter: got %v, want 5s", fe.RetryAfter)
	}
}

func TestRequest_RetryAfterHTTPDate(t *testing.T) {
	future := time.Now().Add(3 * time.Second).UTC().Format(http.TimeFormat)
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", future)
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":"RATE_LIMITED","message":"slow"}}`))
	})
	err := c.request(context.Background(), "GET", "/x", nil, nil)
	var fe *Error
	if !errors.As(err, &fe) {
		t.Fatalf("expected *Error, got %v", err)
	}
	// Second-granularity, with clock skew tolerance.
	if fe.RetryAfter < 500*time.Millisecond || fe.RetryAfter > 4*time.Second {
		t.Errorf("RetryAfter: got %v, want ~3s", fe.RetryAfter)
	}
}

func TestRequest_ServerErrorFallbackCode(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("upstream crashed, not even json"))
	})
	err := c.request(context.Background(), "GET", "/x", nil, nil)
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != "SERVER_ERROR" {
		t.Fatalf("expected SERVER_ERROR, got %v", err)
	}
}

func TestRequest_ContextCancelIsNetworkErrorNotPanic(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Hold the connection open long enough for the caller's context
		// to cancel.
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"data":{}}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	err := c.request(ctx, "GET", "/x", nil, nil)
	var fe *Error
	if !errors.As(err, &fe) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if fe.Code != "NETWORK_ERROR" && fe.Code != "TIMEOUT" {
		t.Errorf("Code: got %q, want NETWORK_ERROR or TIMEOUT", fe.Code)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"garbage", 0},
		{"0", 0},
		{"3", 3 * time.Second},
		{"1.5", 1500 * time.Millisecond},
		{"-1", 0},
	}
	for _, tc := range cases {
		if got := parseRetryAfter(tc.in); got != tc.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestErrorFormatting(t *testing.T) {
	cases := []struct {
		e    *Error
		want string
	}{
		{
			&Error{Code: "RATE_LIMITED", Status: 429, Message: "slow", RequestID: "r1"},
			"floop: [RATE_LIMITED 429] slow (request r1)",
		},
		{
			&Error{Code: "NETWORK_ERROR", Status: 0, Message: "boom"},
			"floop: [NETWORK_ERROR] boom",
		},
		{
			&Error{Code: "VALIDATION_ERROR", Status: 422, Message: "bad"},
			"floop: [VALIDATION_ERROR 422] bad",
		},
	}
	for _, tc := range cases {
		if got := tc.e.Error(); got != tc.want {
			t.Errorf("Error() = %q, want %q", got, tc.want)
		}
	}
}

func TestOptions_WithBaseURLStripsTrailingSlash(t *testing.T) {
	c, err := NewClient("flp_test", WithBaseURL("https://staging.floopfloop.com/"))
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "https://staging.floopfloop.com" {
		t.Errorf("baseURL: got %q", c.baseURL)
	}
}

func TestOptions_WithUserAgentAppendsSuffix(t *testing.T) {
	c, err := NewClient("flp_test", WithUserAgent("myapp/1.2"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(c.userAgent, " myapp/1.2") {
		t.Errorf("userAgent: got %q, want trailing ' myapp/1.2'", c.userAgent)
	}
}
