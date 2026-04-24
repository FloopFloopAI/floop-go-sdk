package floop

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestLibrary_ListBareArray(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/library" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"data":[{"id":"p_1","name":"Cat Cafe","description":null,"subdomain":"cat","botType":"site","cloneCount":42,"createdAt":"2026"}]}`))
	})
	out, err := c.Library.List(context.Background(), LibraryListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != "p_1" || out[0].CloneCount != 42 {
		t.Errorf("decode: %+v", out)
	}
}

func TestLibrary_ListWrappedItems(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"items":[{"id":"p_2","name":"RSI","description":"Crypto dashboard","subdomain":"rsi","botType":"app","cloneCount":7,"createdAt":"2026"}],"page":1}}`))
	})
	out, err := c.Library.List(context.Background(), LibraryListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != "p_2" {
		t.Errorf("decode: %+v", out)
	}
}

func TestLibrary_ListQueryParams(t *testing.T) {
	var seenURL string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenURL = r.URL.String()
		w.Write([]byte(`{"data":[]}`))
	})
	_, err := c.Library.List(context.Background(), LibraryListOptions{
		BotType: "site", Search: "cat cafe", Sort: "popular", Page: 2, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	// URL encoding: space → +
	for _, want := range []string{"botType=site", "search=cat+cafe", "sort=popular", "page=2", "limit=10"} {
		if !strings.Contains(seenURL, want) {
			t.Errorf("url missing %q: %q", want, seenURL)
		}
	}
}

func TestLibrary_Clone(t *testing.T) {
	var seenPath, seenBody string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf)
		seenBody = string(buf)
		w.Write([]byte(`{"data":{"id":"p_new","name":"Cat Cafe","subdomain":"my-cafe","status":"queued"}}`))
	})
	out, err := c.Library.Clone(context.Background(), "p_1", CloneLibraryProjectInput{Subdomain: "my-cafe"})
	if err != nil {
		t.Fatal(err)
	}
	if seenPath != "/api/v1/library/p_1/clone" {
		t.Errorf("path: %s", seenPath)
	}
	if !strings.Contains(seenBody, `"my-cafe"`) {
		t.Errorf("body: %s", seenBody)
	}
	if out.ID != "p_new" {
		t.Errorf("ID: %s", out.ID)
	}
}

func TestUsage_Summary(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/usage/summary" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"data":{
			"plan":{"name":"business","displayName":"Business","monthlyCredits":10000,"maxProjects":100,"maxStorageMb":5000,"maxBandwidthMb":10000},
			"credits":{"currentCredits":5000,"rolledOverCredits":500,"lifetimeCreditsUsed":25000,"rolloverExpiresAt":null},
			"currentPeriod":{"start":"2026-04-01","end":"2026-05-01","projectsCreated":3,"buildsUsed":12,"refinementsUsed":40,"storageUsedMb":200,"bandwidthUsedMb":50}
		}}`))
	})
	out, err := c.Usage.Summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Plan.Name != "business" || out.Plan.MonthlyCredits != 10000 {
		t.Errorf("Plan: %+v", out.Plan)
	}
	if out.Credits.CurrentCredits != 5000 || out.Credits.RolloverExpiresAt != nil {
		t.Errorf("Credits: %+v", out.Credits)
	}
	if out.CurrentPeriod.BuildsUsed != 12 {
		t.Errorf("CurrentPeriod.BuildsUsed: %d", out.CurrentPeriod.BuildsUsed)
	}
}

func TestApiKeys_List(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/api-keys" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"data":{"keys":[{"id":"k_1","name":"my-sdk","keyPrefix":"flp_abcd","scopes":null,"lastUsedAt":null,"createdAt":"2026"}]}}`))
	})
	out, err := c.ApiKeys.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Name != "my-sdk" {
		t.Errorf("decode: %+v", out)
	}
}

func TestApiKeys_Create(t *testing.T) {
	var seenMethod, seenBody string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf)
		seenBody = string(buf)
		w.Write([]byte(`{"data":{"id":"k_new","rawKey":"flp_secretsecret","keyPrefix":"flp_secre"}}`))
	})
	out, err := c.ApiKeys.Create(context.Background(), CreateApiKeyInput{Name: "my-script"})
	if err != nil {
		t.Fatal(err)
	}
	if seenMethod != "POST" {
		t.Errorf("method: %s", seenMethod)
	}
	if !strings.Contains(seenBody, `"my-script"`) {
		t.Errorf("body: %s", seenBody)
	}
	if out.RawKey != "flp_secretsecret" {
		t.Errorf("RawKey: %s", out.RawKey)
	}
}

func TestApiKeys_RemoveByID(t *testing.T) {
	var seenDeletePath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/api-keys":
			w.Write([]byte(`{"data":{"keys":[{"id":"k_1","name":"foo","keyPrefix":"flp_","scopes":null,"lastUsedAt":null,"createdAt":"2026"}]}}`))
		case r.Method == "DELETE":
			seenDeletePath = r.URL.Path
			w.Write([]byte(`{"data":{"success":true}}`))
		default:
			t.Fatalf("unexpected: %s %s", r.Method, r.URL.Path)
		}
	})
	if err := c.ApiKeys.Remove(context.Background(), "k_1"); err != nil {
		t.Fatal(err)
	}
	if seenDeletePath != "/api/v1/api-keys/k_1" {
		t.Errorf("DELETE path: %s", seenDeletePath)
	}
}

func TestApiKeys_RemoveByName(t *testing.T) {
	var seenDeletePath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			w.Write([]byte(`{"data":{"keys":[{"id":"k_7","name":"my-script","keyPrefix":"flp_","scopes":null,"lastUsedAt":null,"createdAt":"2026"}]}}`))
		case "DELETE":
			seenDeletePath = r.URL.Path
			w.Write([]byte(`{"data":{"success":true}}`))
		}
	})
	if err := c.ApiKeys.Remove(context.Background(), "my-script"); err != nil {
		t.Fatal(err)
	}
	if seenDeletePath != "/api/v1/api-keys/k_7" {
		t.Errorf("DELETE path: %s", seenDeletePath)
	}
}

func TestApiKeys_RemoveNotFound(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"keys":[]}}`))
	})
	err := c.ApiKeys.Remove(context.Background(), "ghost")
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %v", err)
	}
}
