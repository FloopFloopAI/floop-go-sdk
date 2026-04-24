package floop

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSubdomains_Check(t *testing.T) {
	var seenURL string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenURL = r.URL.String()
		w.Write([]byte(`{"data":{"slug":"hello","available":true}}`))
	})
	out, err := c.Subdomains.Check(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seenURL, "slug=hello+world") && !strings.Contains(seenURL, "slug=hello%20world") {
		t.Errorf("slug not url-encoded: %s", seenURL)
	}
	if out.Slug != "hello" || !out.Available {
		t.Errorf("decode: %+v", out)
	}
}

func TestSubdomains_Suggest(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "prompt=") {
			t.Errorf("missing prompt query: %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"data":{"slug":"cat-cafe"}}`))
	})
	out, err := c.Subdomains.Suggest(context.Background(), "a cat cafe landing page")
	if err != nil {
		t.Fatal(err)
	}
	if out.Slug != "cat-cafe" {
		t.Errorf("slug: %q", out.Slug)
	}
}

func TestSecrets_List(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects/p_1/secrets" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"data":{"secrets":[{"name":"STRIPE_KEY"},{"name":"DB_URL"}]}}`))
	})
	out, err := c.Secrets.List(context.Background(), "p_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Name != "STRIPE_KEY" {
		t.Errorf("List: %+v", out)
	}
}

func TestSecrets_Set(t *testing.T) {
	var seenMethod, seenBody string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf)
		seenBody = string(buf)
		w.Write([]byte(`{"data":{"success":true}}`))
	})
	if err := c.Secrets.Set(context.Background(), "p_1", "STRIPE_KEY", "sk_xxx"); err != nil {
		t.Fatal(err)
	}
	if seenMethod != "POST" {
		t.Errorf("method: %s", seenMethod)
	}
	if !strings.Contains(seenBody, `"STRIPE_KEY"`) || !strings.Contains(seenBody, `"sk_xxx"`) {
		t.Errorf("body: %s", seenBody)
	}
}

func TestSecrets_Remove(t *testing.T) {
	var seenMethod, seenPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		w.Write([]byte(`{"data":{"success":true,"existed":true}}`))
	})
	if err := c.Secrets.Remove(context.Background(), "p_1", "STRIPE_KEY"); err != nil {
		t.Fatal(err)
	}
	if seenMethod != "DELETE" {
		t.Errorf("method: %s", seenMethod)
	}
	if seenPath != "/api/v1/projects/p_1/secrets/STRIPE_KEY" {
		t.Errorf("path: %s", seenPath)
	}
}

func TestUser_Me(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user/me" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"data":{"id":"u_1","email":"pim@example.com","name":"Pim","plan":"business"}}`))
	})
	u, err := c.User.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "u_1" {
		t.Errorf("ID: %s", u.ID)
	}
	if u.Email == nil || *u.Email != "pim@example.com" {
		t.Errorf("Email: %v", u.Email)
	}
}
