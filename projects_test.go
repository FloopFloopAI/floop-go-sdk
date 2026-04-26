package floop

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProjects_Create(t *testing.T) {
	var seenPath, seenMethod string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenMethod = r.Method
		w.Write([]byte(`{"data":{"project":{"id":"p_1","name":"Cat","subdomain":"cat","status":"queued","botType":null,"url":null,"amplifyAppUrl":null,"isPublic":false,"isAuthProtected":false,"teamId":null,"createdAt":"2026","updatedAt":"2026"},"deployment":{"id":"d_1","status":"queued","version":1}}}`))
	})
	out, err := c.Projects.Create(context.Background(), CreateProjectInput{Prompt: "a cat cafe"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if seenMethod != "POST" || seenPath != "/api/v1/projects" {
		t.Errorf("request: %s %s", seenMethod, seenPath)
	}
	if out.Project.ID != "p_1" {
		t.Errorf("Project.ID: got %q", out.Project.ID)
	}
	if out.Deployment.Version != 1 {
		t.Errorf("Deployment.Version: got %d", out.Deployment.Version)
	}
}

func TestProjects_ListWithTeamID(t *testing.T) {
	var seenURL string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenURL = r.URL.String()
		w.Write([]byte(`{"data":[]}`))
	})
	if _, err := c.Projects.List(context.Background(), ListProjectsOptions{TeamID: "team 1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seenURL, "teamId=team+1") && !strings.Contains(seenURL, "teamId=team%201") {
		t.Errorf("URL missing encoded teamId: %q", seenURL)
	}
}

func TestProjects_GetByIDOrSubdomain(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[
			{"id":"p_1","name":"A","subdomain":"alpha","status":"live","botType":null,"url":null,"amplifyAppUrl":null,"isPublic":true,"isAuthProtected":false,"teamId":null,"createdAt":"","updatedAt":""},
			{"id":"p_2","name":"B","subdomain":"beta","status":"live","botType":null,"url":null,"amplifyAppUrl":null,"isPublic":true,"isAuthProtected":false,"teamId":null,"createdAt":"","updatedAt":""}
		]}`))
	})
	p, err := c.Projects.Get(context.Background(), "beta", ListProjectsOptions{})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.ID != "p_2" {
		t.Errorf("Get by subdomain: got ID %q, want p_2", p.ID)
	}
	p, err = c.Projects.Get(context.Background(), "p_1", ListProjectsOptions{})
	if err != nil {
		t.Fatalf("Get by id: %v", err)
	}
	if p.Subdomain == nil || *p.Subdomain != "alpha" {
		t.Errorf("Get by id: got subdomain %v", p.Subdomain)
	}
}

func TestProjects_GetNotFound(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[]}`))
	})
	_, err := c.Projects.Get(context.Background(), "ghost", ListProjectsOptions{})
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %v", err)
	}
}

func TestProjects_Status(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects/p_1/status" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"data":{"step":2,"totalSteps":5,"status":"generating","message":"working","progress":0.4}}`))
	})
	ev, err := c.Projects.Status(context.Background(), "p_1")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Status != "generating" || ev.Step != 2 || ev.TotalSteps != 5 {
		t.Errorf("status unpack: %+v", ev)
	}
	if ev.Progress == nil || *ev.Progress != 0.4 {
		t.Errorf("progress: got %v", ev.Progress)
	}
}

func TestProjects_Cancel(t *testing.T) {
	var seenMethod, seenPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		w.Write([]byte(`{"data":{}}`))
	})
	if err := c.Projects.Cancel(context.Background(), "p_1"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if seenMethod != "POST" || seenPath != "/api/v1/projects/p_1/cancel" {
		t.Errorf("request: %s %s", seenMethod, seenPath)
	}
}

func TestProjects_Reactivate(t *testing.T) {
	var seenMethod, seenPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		w.Write([]byte(`{"data":{}}`))
	})
	if err := c.Projects.Reactivate(context.Background(), "p_1"); err != nil {
		t.Fatalf("Reactivate: %v", err)
	}
	if seenMethod != "POST" || seenPath != "/api/v1/projects/p_1/reactivate" {
		t.Errorf("request: %s %s", seenMethod, seenPath)
	}
}

func TestProjects_Conversations(t *testing.T) {
	var seenPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.String()
		w.Write([]byte(`{"data":{"messages":[{"id":"m_1","projectId":"p_1","role":"user","content":"hi","metadata":null,"status":"sent","position":1,"createdAt":""},{"id":"m_2","projectId":"p_1","role":"assistant","content":"hello","metadata":null,"status":"sent","position":2,"createdAt":""}],"queued":[],"latestVersion":3}}`))
	})
	out, err := c.Projects.Conversations(context.Background(), "p_1", ConversationsOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if seenPath != "/api/v1/projects/p_1/conversations?limit=10" {
		t.Errorf("path: %s", seenPath)
	}
	if len(out.Messages) != 2 || out.LatestVersion != 3 {
		t.Errorf("decode: %+v", out)
	}
	if out.Messages[1].Role != "assistant" || out.Messages[1].Content != "hello" {
		t.Errorf("message[1]: %+v", out.Messages[1])
	}
}

func TestProjects_ConversationsWithoutLimit(t *testing.T) {
	var seenPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.String()
		w.Write([]byte(`{"data":{"messages":[],"queued":[],"latestVersion":0}}`))
	})
	_, err := c.Projects.Conversations(context.Background(), "p_1", ConversationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// No ?limit= query param when opts.Limit is zero (server default).
	if seenPath != "/api/v1/projects/p_1/conversations" {
		t.Errorf("path: %s", seenPath)
	}
}

func TestProjects_RefineQueued(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"queued":true,"messageId":"m_1"}}`))
	})
	res, err := c.Projects.Refine(context.Background(), "p_1", RefineInput{Message: "change X"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued == nil || res.Queued.MessageID != "m_1" {
		t.Errorf("Queued variant: got %+v", res)
	}
	if res.SavedOnly != nil || res.Processing != nil {
		t.Errorf("other variants should be nil: %+v", res)
	}
}

func TestProjects_RefineSavedOnly(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"queued":false}}`))
	})
	res, err := c.Projects.Refine(context.Background(), "p_1", RefineInput{Message: "code-only tweak"})
	if err != nil {
		t.Fatal(err)
	}
	if res.SavedOnly == nil || res.Queued != nil || res.Processing != nil {
		t.Errorf("SavedOnly variant: got %+v", res)
	}
}

func TestProjects_RefineProcessing(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"processing":true,"deploymentId":"d_7","queuePriority":3}}`))
	})
	res, err := c.Projects.Refine(context.Background(), "p_1", RefineInput{Message: "big change"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Processing == nil || res.Processing.DeploymentID != "d_7" || res.Processing.QueuePriority != 3 {
		t.Errorf("Processing variant: got %+v", res.Processing)
	}
}

func TestProjects_WaitForLive_Success(t *testing.T) {
	var statusCalls int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/projects/p_1/status":
			n := atomic.AddInt32(&statusCalls, 1)
			switch n {
			case 1:
				w.Write([]byte(`{"data":{"step":1,"totalSteps":3,"status":"queued","message":""}}`))
			case 2:
				w.Write([]byte(`{"data":{"step":2,"totalSteps":3,"status":"generating","message":""}}`))
			default:
				w.Write([]byte(`{"data":{"step":3,"totalSteps":3,"status":"live","message":""}}`))
			}
		case "/api/v1/projects":
			// Serves Get() after status reaches "live".
			w.Write([]byte(`{"data":[{"id":"p_1","name":"","subdomain":"cat","status":"live","botType":null,"url":"https://cat.floop.tech","amplifyAppUrl":null,"isPublic":true,"isAuthProtected":false,"teamId":null,"createdAt":"","updatedAt":""}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})
	proj, err := c.Projects.WaitForLive(context.Background(), "p_1", &WaitForLiveOptions{Interval: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("WaitForLive: %v", err)
	}
	if proj.URL == nil || *proj.URL != "https://cat.floop.tech" {
		t.Errorf("URL: %v", proj.URL)
	}
	if atomic.LoadInt32(&statusCalls) != 3 {
		t.Errorf("status polled %d times, want 3", statusCalls)
	}
}

func TestProjects_WaitForLive_FailedReturnsTypedError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"step":1,"totalSteps":1,"status":"failed","message":"bad vibes"}}`))
	})
	_, err := c.Projects.WaitForLive(context.Background(), "p_1", &WaitForLiveOptions{Interval: 5 * time.Millisecond})
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != "BUILD_FAILED" {
		t.Fatalf("expected BUILD_FAILED, got %v", err)
	}
	if fe.Message != "bad vibes" {
		t.Errorf("message: %q", fe.Message)
	}
}

// Pre-fix the case-statement only matched live/failed/cancelled, so
// streaming an `archived` project looped until MaxWait. Node / Python
// / Swift / Kotlin all treat archived as a non-error terminal; Go
// now matches.
func TestProjects_Stream_ArchivedTerminatesCleanlyLikeLive(t *testing.T) {
	statusCalls := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/projects/p_1/status" {
			statusCalls++
			w.Write([]byte(`{"data":{"step":3,"totalSteps":3,"status":"archived","message":""}}`))
			return
		}
		// Serves Get() after stream returns; archived projects come back
		// from list() with status:archived.
		w.Write([]byte(`{"data":[{"id":"p_1","name":"","subdomain":"cat","status":"archived","botType":null,"url":null,"amplifyAppUrl":null,"isPublic":true,"isAuthProtected":false,"teamId":null,"createdAt":"","updatedAt":""}]}`))
	})
	proj, err := c.Projects.WaitForLive(context.Background(), "p_1", &WaitForLiveOptions{Interval: 5 * time.Millisecond, MaxWait: 5 * time.Second})
	if err != nil {
		t.Fatalf("archived should be a clean terminal, not a max_wait timeout — got %v", err)
	}
	if proj == nil || proj.Status != "archived" {
		t.Errorf("expected archived project, got %+v", proj)
	}
	if statusCalls != 1 {
		t.Errorf("status polled %d times, want 1 (archived terminates immediately)", statusCalls)
	}
}
