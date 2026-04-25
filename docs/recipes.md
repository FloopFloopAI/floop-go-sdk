# Cookbook

Concrete `github.com/FloopFloopAI/floop-go-sdk` patterns you can copy-paste. Every snippet uses only the SDK's public surface — no undocumented endpoints, no private helpers.

For the basics (install, client setup, resource tour) see the [README](../README.md). This file is the **"I know the basics, now how do I actually build X"** layer.

These recipes mirror the [Node](https://github.com/FloopFloopAI/floop-node-sdk/blob/main/docs/recipes.md) and [Python](https://github.com/FloopFloopAI/floop-python-sdk/blob/main/docs/recipes.md) cookbooks, translated to Go idioms (context-first, callback-based streaming, `errors.As` for typed error inspection).

---

## 1. Ship a project from prompt to live URL

The canonical one-call flow: create, wait, done. `WaitForLive` returns a `*floop.Error` with `Code == "BUILD_FAILED"` / `"BUILD_CANCELLED"` / `"TIMEOUT"` on non-success terminals, so plain `errors.As` is enough.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/FloopFloopAI/floop-go-sdk"
)

func ship(ctx context.Context, client *floop.Client, prompt, subdomain string) (string, error) {
	created, err := client.Projects.Create(ctx, floop.CreateProjectInput{
		Prompt:    prompt,
		Subdomain: subdomain,
		BotType:   "site",
	})
	if err != nil {
		return "", fmt.Errorf("create: %w", err)
	}

	// Polls status every 2s; bounds the total wait to 10 minutes so a
	// stuck build doesn't hang forever.
	live, err := client.Projects.WaitForLive(ctx, created.Project.ID, &floop.StreamOptions{
		Interval: 2 * time.Second,
		MaxWait:  10 * time.Minute,
	})
	if err != nil {
		var fe *floop.Error
		if errors.As(err, &fe) && fe.Code == "BUILD_FAILED" {
			log.Printf("build failed: %s", fe.Message)
		}
		return "", err
	}
	if live.URL == nil {
		return "", fmt.Errorf("project is live but has no URL yet")
	}
	return *live.URL, nil
}

func main() {
	client, err := floop.NewClient("flp_...")
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	url, err := ship(ctx, client, "A single-page portfolio for a landscape photographer", "landscape-portfolio")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Live at", url)
}
```

**Wall-clock timeout via context.** Prefer `context.WithTimeout` over `MaxWait` when your caller already has a deadline — `WaitForLive` honours both:

```go
ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
defer cancel()
live, err := client.Projects.WaitForLive(ctx, id, nil)
```

**When to prefer `Stream` over `WaitForLive`:** when you want to show progress to a user. `WaitForLive` only returns at the end — no visibility into what the build is doing.

---

## 2. Watch a build progress in real time

`Projects.Stream(ctx, ref, opts, handler)` calls `handler` for every unique status transition and returns when the project reaches a terminal state (live / failed / cancelled) or the context deadline fires. Events are de-duplicated on `(status, step, progress, queuePosition)` so handler doesn't fire on every poll.

```go
err := client.Projects.Stream(ctx, "recipe-blog", nil, func(ev floop.StatusEvent) error {
	progress := ""
	if ev.Progress != nil {
		progress = fmt.Sprintf(" %.0f%%", *ev.Progress)
	}
	fmt.Printf("[%s]%s step=%d/%d — %s\n", ev.Status, progress, ev.Step, ev.TotalSteps, ev.Message)
	return nil  // non-nil to stop polling early
})

var fe *floop.Error
switch {
case err == nil:
	// Reached "live" cleanly.
	done, _ := client.Projects.Get(ctx, "recipe-blog", floop.ListProjectsOptions{})
	if done.URL != nil {
		fmt.Println("Live at", *done.URL)
	}
case errors.As(err, &fe) && fe.Code == "BUILD_FAILED":
	log.Fatalf("build failed: %s", fe.Message)
case errors.As(err, &fe) && fe.Code == "TIMEOUT":
	log.Fatalf("build stalled past MaxWait")
default:
	log.Fatalf("stream: %v", err)
}
```

**Early abort.** Return a non-nil error from the handler to stop polling. The SDK returns it verbatim so callers can tell their own sentinel apart from the SDK's terminal errors:

```go
var errEnough = errors.New("seen enough progress")

err := client.Projects.Stream(ctx, ref, nil, func(ev floop.StatusEvent) error {
	if ev.Progress != nil && *ev.Progress >= 50 {
		return errEnough
	}
	return nil
})

if errors.Is(err, errEnough) {
	// graceful caller-initiated exit
}
```

---

## 3. Refine a project, even when it's mid-build

`Projects.Refine` returns a `*RefineResult` with three mutually-exclusive pointer fields. Exactly one is non-nil:

- `Queued` — project is currently deploying; your message is queued and will be processed when the current build finishes.
- `Processing` — your message triggered a new build immediately.
- `SavedOnly` — the message was saved as a conversation entry without triggering a build.

```go
res, err := client.Projects.Refine(ctx, "recipe-blog", floop.RefineInput{
	Message: "Add a search bar to the header",
})
if err != nil {
	log.Fatal(err)
}

switch {
case res.Processing != nil:
	fmt.Printf("Build started (deployment %s)\n", res.Processing.DeploymentID)
	_, err = client.Projects.WaitForLive(ctx, "recipe-blog", nil)
case res.Queued != nil:
	fmt.Printf("Queued behind current build (message %s)\n", res.Queued.MessageID)
	// Poll the project once — when it's back to "live", your queued
	// message has already been picked up and processed.
	_, err = client.Projects.WaitForLive(ctx, "recipe-blog", nil)
case res.SavedOnly != nil:
	fmt.Println("Saved as a chat message, no build triggered")
}
if err != nil {
	log.Fatal(err)
}
```

**Why a nil-pointer discriminated union instead of an interface?** Go doesn't have sum types; the alternatives (a single flag field with dead sibling fields, or an interface with three concrete types) each trade off one of type-safety, zero-value sanity, or ergonomics. Nil-pointer discrimination is the least-worst match for the JSON shape the backend actually returns.

---

## 4. Upload an image and refine with it as context

Uploads are two-step: `Uploads.Create` presigns an S3 URL and does the direct PUT, returning an `*UploadedAttachment`. **There's a type-shape gotcha:** `RefineInput.Attachments` is `[]RefineAttachment`, not `[]UploadedAttachment`. The fields are the same but `FileSize` is `int` on one and `int64` on the other, so you need a one-line conversion.

```go
package main

import (
	"context"
	"log"
	"os"

	"github.com/FloopFloopAI/floop-go-sdk"
)

func attachFromFile(ctx context.Context, client *floop.Client, fileName, path string) (floop.RefineAttachment, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return floop.RefineAttachment{}, err
	}
	up, err := client.Uploads.Create(ctx, floop.CreateUploadInput{
		FileName: fileName,
		Bytes:    bytes,
		// FileType: "image/png",   // optional — guessed from extension
	})
	if err != nil {
		return floop.RefineAttachment{}, err
	}
	return floop.RefineAttachment{
		Key:      up.Key,
		FileName: up.FileName,
		FileType: up.FileType,
		FileSize: int(up.FileSize),  // int64 → int
	}, nil
}

func main() {
	client, _ := floop.NewClient(os.Getenv("FLOOP_API_KEY"))
	ctx := context.Background()

	att, err := attachFromFile(ctx, client, "mockup.png", "./mockup.png")
	if err != nil {
		log.Fatal(err)
	}

	_, err = client.Projects.Refine(ctx, "recipe-blog", floop.RefineInput{
		Message:     "Make the homepage look like this mockup.",
		Attachments: []floop.RefineAttachment{att},
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

**Supported types:** `png`, `jpg/jpeg`, `gif`, `svg`, `webp`, `ico`, `pdf`, `txt`, `csv`, `doc`, `docx`. Max 5 MB per upload. The SDK validates client-side before hitting the network, so bad inputs return `*floop.Error{Code: "VALIDATION_ERROR"}` with no round-trip.

**Streaming large files.** For files you don't want fully buffered in memory, pass `File` (an `io.Reader`) + `Size` instead of `Bytes`:

```go
f, err := os.Open(path)
if err != nil { return err }
defer f.Close()

stat, _ := f.Stat()
up, err := client.Uploads.Create(ctx, floop.CreateUploadInput{
	FileName: fileName,
	File:     f,
	Size:     stat.Size(),
})
```

Attachments only flow through `Refine` today — `Create` doesn't accept them via the SDK. If you need to anchor a brand-new project against images, create with a prompt first, then refine with the attachments as a follow-up.

---

## 5. Rotate an API key from a CI job

Three-step rotation: create the new key, write it to your secret store, then revoke the old one. The order matters — you must revoke with a **different** key than the one making the call (the backend returns `400 VALIDATION_ERROR` if you try to revoke the key you're authenticated with).

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/FloopFloopAI/floop-go-sdk"
)

func rotate(ctx context.Context, victimName string) error {
	// Use a long-lived bootstrap key (stored as a CI secret) to do the
	// rotation. Don't use the key we're about to revoke — that hits the
	// self-revoke guard.
	bootstrap, err := floop.NewClient(os.Getenv("FLOOP_BOOTSTRAP_KEY"))
	if err != nil {
		return err
	}

	// 1. Find the key we want to rotate by its name. (Each name is
	//    unique per account because the dashboard enforces it; matching
	//    by name is more reliable than matching the prefix substring.)
	keys, err := bootstrap.ApiKeys.List(ctx)
	if err != nil {
		return err
	}
	var victim *floop.ApiKeySummary
	for i := range keys {
		if keys[i].Name == victimName {
			victim = &keys[i]
			break
		}
	}
	if victim == nil {
		return fmt.Errorf("key not found: %s", victimName)
	}

	// 2. Mint the replacement.
	fresh, err := bootstrap.ApiKeys.Create(ctx, floop.CreateApiKeyInput{Name: victimName + "-new"})
	if err != nil {
		return err
	}
	if err := writeSecret("FLOOP_API_KEY", fresh.RawKey); err != nil {
		return err
	}

	// 3. Revoke the old one. Remove() accepts an id OR a name.
	return bootstrap.ApiKeys.Remove(ctx, victim.ID)
}

// writeSecret wires into your CI platform's secret store — AWS Secrets
// Manager, Vault, GitHub Actions `gh secret set`, etc.
func writeSecret(name, value string) error { /* ... */ return nil }
```

**Can't I just reuse the bootstrap key forever?** Technically yes — if it's tightly scoped and audited. In practice, a single long-lived "rotator key" is a common compromise: it only has permission to mint/list/revoke keys, never appears in application traffic, and itself gets rotated manually on a rare cadence (annually, or on compromise).

The 5-keys-per-account cap applies to active keys, so make sure to revoke old rotations rather than accumulating them.

---

## 6. Retry with backoff on `RATE_LIMITED` and `NETWORK_ERROR`

`*floop.Error` carries everything you need to implement backoff correctly:

- `RetryAfter time.Duration` — populated from the `Retry-After` header on 429s (parsed from delta-seconds OR HTTP-date). Zero when the server didn't set it.
- `Code string` — distinguishes retryable (`RATE_LIMITED`, `NETWORK_ERROR`, `TIMEOUT`, `SERVICE_UNAVAILABLE`, `SERVER_ERROR`) from permanent (`UNAUTHORIZED`, `FORBIDDEN`, `VALIDATION_ERROR`, `NOT_FOUND`, `CONFLICT`, `BUILD_FAILED`, `BUILD_CANCELLED`).

```go
package main

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"time"

	"github.com/FloopFloopAI/floop-go-sdk"
)

var retryable = map[string]bool{
	"RATE_LIMITED":        true,
	"NETWORK_ERROR":       true,
	"TIMEOUT":             true,
	"SERVICE_UNAVAILABLE": true,
	"SERVER_ERROR":        true,
}

// WithRetry runs fn with exponential backoff on retryable FloopError
// codes, respecting Retry-After when the server set it.
func WithRetry[T any](ctx context.Context, maxAttempts int, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	for attempt := 1; ; attempt++ {
		out, err := fn(ctx)
		if err == nil {
			return out, nil
		}

		var fe *floop.Error
		if !errors.As(err, &fe) || !retryable[fe.Code] {
			return zero, err
		}
		if attempt >= maxAttempts {
			return zero, err
		}

		// Prefer the server's hint; fall back to exponential backoff
		// with jitter capped at 30 s.
		wait := fe.RetryAfter
		if wait == 0 {
			wait = min(30*time.Second, time.Duration(250<<attempt)*time.Millisecond)
		}
		wait += time.Duration(rand.Intn(250)) * time.Millisecond

		log.Printf("floop: %s (attempt %d/%d), retrying in %s (request %s)",
			fe.Code, attempt, maxAttempts, wait, fe.RequestID)

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// Usage:
//   projects, err := WithRetry(ctx, 5, func(ctx context.Context) ([]floop.Project, error) {
//       return client.Projects.List(ctx, floop.ListProjectsOptions{})
//   })
```

**Don't retry everything.** `VALIDATION_ERROR`, `UNAUTHORIZED`, and `FORBIDDEN` are not going to fix themselves between attempts — retrying them just burns rate-limit budget and delays the real error reaching your logs.

**`ctx.Done()` on the sleep** is non-negotiable: without it, a cancelled context waits out the full backoff before the caller sees the cancellation. The `select` pattern above makes cancellation responsive.

---

## Got a pattern worth adding?

Open an issue at [FloopFloopAI/floop-go-sdk/issues](https://github.com/FloopFloopAI/floop-go-sdk/issues) describing the use case. Recipes live in this file, not in the package, so they're easy to update without an SDK release.
