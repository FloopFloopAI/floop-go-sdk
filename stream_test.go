package floop

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestStream_YieldsEachUniqueEventIncludingTerminal(t *testing.T) {
	var call int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&call, 1)
		switch n {
		case 1:
			w.Write([]byte(`{"data":{"step":1,"totalSteps":3,"status":"queued","message":""}}`))
		case 2:
			w.Write([]byte(`{"data":{"step":2,"totalSteps":3,"status":"generating","message":"","progress":0.3}}`))
		case 3:
			// Duplicate → should be deduped.
			w.Write([]byte(`{"data":{"step":2,"totalSteps":3,"status":"generating","message":"","progress":0.3}}`))
		case 4:
			w.Write([]byte(`{"data":{"step":2,"totalSteps":3,"status":"generating","message":"","progress":0.6}}`))
		default:
			w.Write([]byte(`{"data":{"step":3,"totalSteps":3,"status":"live","message":""}}`))
		}
	})

	var seen []string
	err := c.Projects.Stream(context.Background(), "p_1",
		&StreamOptions{Interval: 2 * time.Millisecond},
		func(ev StatusEvent) error {
			seen = append(seen, ev.Status)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	// queued, generating(0.3), generating(0.6), live — dedupe drops
	// the second identical generating(0.3).
	wantSeq := []string{"queued", "generating", "generating", "live"}
	if len(seen) != len(wantSeq) {
		t.Fatalf("seen=%v, want %v", seen, wantSeq)
	}
	for i, s := range wantSeq {
		if seen[i] != s {
			t.Errorf("seen[%d]=%q, want %q", i, seen[i], s)
		}
	}
}

func TestStream_TerminalFailedReturnsTypedError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"step":1,"totalSteps":1,"status":"failed","message":"typecheck failed"}}`))
	})
	err := c.Projects.Stream(context.Background(), "p_1",
		&StreamOptions{Interval: 1 * time.Millisecond},
		func(StatusEvent) error { return nil })
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != "BUILD_FAILED" {
		t.Fatalf("expected BUILD_FAILED, got %v", err)
	}
	if fe.Message != "typecheck failed" {
		t.Errorf("message: %q", fe.Message)
	}
}

func TestStream_HandlerErrorStopsEarly(t *testing.T) {
	var call int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&call, 1)
		w.Write([]byte(`{"data":{"step":1,"totalSteps":3,"status":"queued","message":""}}`))
	})

	sentinel := errors.New("caller aborted")
	err := c.Projects.Stream(context.Background(), "p_1",
		&StreamOptions{Interval: 2 * time.Millisecond},
		func(StatusEvent) error { return sentinel })

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel, got %v", err)
	}
	// Exactly one Status call — handler's error short-circuits the loop
	// before the second poll.
	if got := atomic.LoadInt32(&call); got != 1 {
		t.Errorf("Status polled %d times, want 1", got)
	}
}

func TestStream_NilHandlerIsAllowed(t *testing.T) {
	var call int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&call, 1)
		if n < 3 {
			w.Write([]byte(`{"data":{"step":1,"totalSteps":2,"status":"queued","message":""}}`))
		} else {
			w.Write([]byte(`{"data":{"step":2,"totalSteps":2,"status":"live","message":""}}`))
		}
	})
	// WaitForLive passes nil as handler — must not panic.
	err := c.Projects.Stream(context.Background(), "p_1",
		&StreamOptions{Interval: 2 * time.Millisecond}, nil)
	if err != nil {
		t.Fatalf("Stream(nil handler): %v", err)
	}
}

func TestStream_MaxWaitTimeoutReturnsTypedError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Always queued — will never terminate naturally.
		w.Write([]byte(`{"data":{"step":1,"totalSteps":3,"status":"queued","message":""}}`))
	})
	err := c.Projects.Stream(context.Background(), "p_1",
		&StreamOptions{Interval: 5 * time.Millisecond, MaxWait: 20 * time.Millisecond},
		func(StatusEvent) error { return nil })
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != "TIMEOUT" {
		t.Fatalf("expected TIMEOUT, got %v", err)
	}
}

func TestStream_PowersWaitForLive(t *testing.T) {
	// Regression: WaitForLive is now implemented on top of Stream —
	// ensure it still behaves the same way (success case).
	var call int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/projects/p_1/status":
			n := atomic.AddInt32(&call, 1)
			if n < 2 {
				w.Write([]byte(`{"data":{"step":1,"totalSteps":2,"status":"queued","message":""}}`))
			} else {
				w.Write([]byte(`{"data":{"step":2,"totalSteps":2,"status":"live","message":""}}`))
			}
		case "/api/v1/projects":
			w.Write([]byte(`{"data":[{"id":"p_1","name":"","subdomain":"s","status":"live","botType":null,"url":"https://s.floop.tech","amplifyAppUrl":null,"isPublic":true,"isAuthProtected":false,"teamId":null,"createdAt":"","updatedAt":""}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})
	proj, err := c.Projects.WaitForLive(context.Background(), "p_1",
		&WaitForLiveOptions{Interval: 2 * time.Millisecond})
	if err != nil {
		t.Fatalf("WaitForLive: %v", err)
	}
	if proj.URL == nil || *proj.URL != "https://s.floop.tech" {
		t.Errorf("URL: %v", proj.URL)
	}
}
