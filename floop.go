// Package floop is the official Go SDK for the FloopFloop API
// (https://www.floopfloop.com). It wraps the same /api/v1/* surface the
// floop CLI and the Node/Python SDKs consume.
//
// # Quickstart
//
//	client, err := floop.NewClient(os.Getenv("FLOOP_API_KEY"))
//	if err != nil { log.Fatal(err) }
//
//	created, err := client.Projects.Create(ctx, floop.CreateProjectInput{
//	    Prompt:    "a crypto RSI dashboard for BTC",
//	    Subdomain: "rsi-btc",
//	})
//	if err != nil { log.Fatal(err) }
//
//	live, err := client.Projects.WaitForLive(ctx, created.Project.ID, nil)
//	fmt.Println("Live at:", live.URL)
package floop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Version is the SDK's semver tag (kept in sync with the latest `v*`
// git tag on the repo — Go modules require a plain `v` prefix, not
// `sdk-v*` like the Node SDK).
const Version = "0.1.0-alpha.4"

const (
	defaultBaseURL = "https://www.floopfloop.com"
	defaultTimeout = 30 * time.Second
)

// Client is the entry point. Construct it once with NewClient and reuse it
// across goroutines; it is safe for concurrent use.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	userAgent  string

	// Resources — attached in NewClient so users can do client.Projects.Create(...).
	Projects   *Projects
	Subdomains *Subdomains
	Secrets    *Secrets
	Library    *Library
	Usage      *Usage
	ApiKeys    *ApiKeys
	Uploads    *Uploads
	User       *UserAPI
}

// ClientOption configures the Client at construction time.
type ClientOption func(*Client)

// WithBaseURL points the client at a staging or local dev server.
// Defaults to https://www.floopfloop.com.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) { c.baseURL = strings.TrimRight(baseURL, "/") }
}

// WithHTTPClient installs a caller-supplied *http.Client (for proxies,
// custom transports, tests). The client should itself carry a sensible
// per-request timeout. Defaults to a new http.Client with a 30-second
// Timeout.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) { c.httpClient = hc }
}

// WithTimeout sets the overall per-request timeout. Ignored when
// WithHTTPClient is also supplied — configure the timeout on the
// custom client yourself in that case.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		if c.httpClient == nil {
			c.httpClient = &http.Client{Timeout: d}
		}
	}
}

// WithUserAgent appends a suffix to the default "floop-go-sdk/<v>"
// User-Agent header. Useful for per-app attribution.
func WithUserAgent(suffix string) ClientOption {
	return func(c *Client) {
		if suffix != "" {
			c.userAgent = fmt.Sprintf("floop-go-sdk/%s %s", Version, suffix)
		}
	}
}

// NewClient constructs a Client with the given bearer token. Options
// override the defaults. An error is returned if apiKey is empty.
func NewClient(apiKey string, opts ...ClientOption) (*Client, error) {
	if apiKey == "" {
		return nil, errors.New("floop: apiKey is required")
	}

	c := &Client{
		apiKey:    apiKey,
		baseURL:   defaultBaseURL,
		userAgent: fmt.Sprintf("floop-go-sdk/%s", Version),
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: defaultTimeout}
	}

	c.Projects = &Projects{client: c}
	c.Subdomains = &Subdomains{client: c}
	c.Secrets = &Secrets{client: c}
	c.Library = &Library{client: c}
	c.Usage = &Usage{client: c}
	c.ApiKeys = &ApiKeys{client: c}
	c.Uploads = &Uploads{client: c}
	c.User = &UserAPI{client: c}

	return c, nil
}

// Error is returned for every non-2xx response and for transport-level
// failures. Callers can type-assert with errors.As to inspect Code,
// Status, etc:
//
//	var fe *floop.Error
//	if errors.As(err, &fe) && fe.Code == "RATE_LIMITED" {
//	    time.Sleep(fe.RetryAfter)
//	}
type Error struct {
	// Code is the backend's error code (e.g. "NOT_FOUND", "RATE_LIMITED")
	// or one of "NETWORK_ERROR" / "TIMEOUT" / "UNKNOWN" for transport
	// issues. Unknown server codes pass through verbatim so callers can
	// switch on new codes without us cutting a release first.
	Code string

	// Status is the HTTP status code. 0 for transport-level failures.
	Status int

	// Message is the backend-provided human-readable error message (or a
	// synthetic one for transport failures).
	Message string

	// RequestID is the x-request-id header value, if the server set one.
	// Include it in bug reports for faster triage.
	RequestID string

	// RetryAfter is parsed from the Retry-After response header on 429
	// responses. Zero when not present or unparseable. Honours both
	// delta-seconds and HTTP-date formats per RFC 7231.
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *Error) Error() string {
	parts := []string{"floop: "}
	if e.Code != "" {
		parts = append(parts, "[", e.Code)
		if e.Status != 0 {
			parts = append(parts, " ", strconv.Itoa(e.Status))
		}
		parts = append(parts, "] ")
	}
	parts = append(parts, e.Message)
	if e.RequestID != "" {
		parts = append(parts, " (request ", e.RequestID, ")")
	}
	return strings.Join(parts, "")
}

// apiSuccess is the {"data": ...} envelope every successful response uses.
type apiSuccess[T any] struct {
	Data T `json:"data"`
}

// apiError is the {"error": {"code":..., "message":...}} envelope.
type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// request performs an HTTP request and decodes the `data` envelope into out.
// Pass nil for out to discard the response body.
func (c *Client) request(ctx context.Context, method, path string, body, out any) error {
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return &Error{Code: "UNKNOWN", Message: "failed to marshal request body: " + err.Error()}
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return &Error{Code: "UNKNOWN", Message: "failed to build request: " + err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		code := "NETWORK_ERROR"
		msg := fmt.Sprintf("could not reach %s (%s)", c.baseURL, err.Error())
		if errors.Is(err, context.DeadlineExceeded) {
			code = "TIMEOUT"
			msg = "request timed out"
		} else if errors.Is(err, context.Canceled) {
			msg = "request cancelled"
		}
		return &Error{Code: code, Message: msg}
	}
	defer resp.Body.Close()

	requestID := resp.Header.Get("X-Request-Id")
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return &Error{
			Code:      "NETWORK_ERROR",
			Status:    resp.StatusCode,
			Message:   "failed to read response body: " + err.Error(),
			RequestID: requestID,
		}
	}

	if resp.StatusCode >= 400 {
		e := &Error{
			Code:       defaultCodeForStatus(resp.StatusCode),
			Status:     resp.StatusCode,
			Message:    fmt.Sprintf("request failed (%d)", resp.StatusCode),
			RequestID:  requestID,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
		if len(raw) > 0 {
			var envelope apiError
			if jsonErr := json.Unmarshal(raw, &envelope); jsonErr == nil {
				if envelope.Error.Code != "" {
					e.Code = envelope.Error.Code
				}
				if envelope.Error.Message != "" {
					e.Message = envelope.Error.Message
				}
			}
		}
		return e
	}

	if out == nil {
		return nil
	}
	// Try to unwrap the envelope; fall back to decoding the whole body
	// if the response is bare (no "data" key).
	var probe map[string]json.RawMessage
	if jsonErr := json.Unmarshal(raw, &probe); jsonErr == nil {
		if data, ok := probe["data"]; ok {
			if unErr := json.Unmarshal(data, out); unErr != nil {
				return &Error{
					Code:      "UNKNOWN",
					Status:    resp.StatusCode,
					Message:   "failed to decode response data: " + unErr.Error(),
					RequestID: requestID,
				}
			}
			return nil
		}
	}
	if unErr := json.Unmarshal(raw, out); unErr != nil {
		return &Error{
			Code:      "UNKNOWN",
			Status:    resp.StatusCode,
			Message:   "failed to decode response: " + unErr.Error(),
			RequestID: requestID,
		}
	}
	return nil
}

func defaultCodeForStatus(status int) string {
	switch {
	case status == 401:
		return "UNAUTHORIZED"
	case status == 403:
		return "FORBIDDEN"
	case status == 404:
		return "NOT_FOUND"
	case status == 409:
		return "CONFLICT"
	case status == 422:
		return "VALIDATION_ERROR"
	case status == 429:
		return "RATE_LIMITED"
	case status == 503:
		return "SERVICE_UNAVAILABLE"
	case status >= 500:
		return "SERVER_ERROR"
	default:
		return "UNKNOWN"
	}
}

// parseRetryAfter honours both delta-seconds and HTTP-date forms per
// RFC 7231. Returns 0 if the header is empty / unparseable. Matches the
// behaviour of the Node and Python SDKs.
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	if secs, err := strconv.ParseFloat(header, 64); err == nil && secs >= 0 {
		return time.Duration(secs * float64(time.Second))
	}
	if when, err := http.ParseTime(header); err == nil {
		d := time.Until(when)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}
