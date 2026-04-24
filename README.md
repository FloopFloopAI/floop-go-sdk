# floop-go-sdk

[![Go Reference](https://pkg.go.dev/badge/github.com/FloopFloopAI/floop-go-sdk.svg)](https://pkg.go.dev/github.com/FloopFloopAI/floop-go-sdk)
[![CI](https://img.shields.io/github/actions/workflow/status/FloopFloopAI/floop-go-sdk/ci.yml?branch=main&logo=github&label=ci)](https://github.com/FloopFloopAI/floop-go-sdk/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/FloopFloopAI/floop-go-sdk?logo=go&logoColor=white)](./go.mod)
[![License: MIT](https://img.shields.io/github/license/FloopFloopAI/floop-go-sdk)](./LICENSE)

Official Go SDK for the [FloopFloop](https://www.floopfloop.com) API. Build a project, refine it, manage secrets and subdomains from any Go 1.22+ codebase.

## Install

```bash
go get github.com/FloopFloopAI/floop-go-sdk@latest
```

## Quickstart

Grab an API key: `floop keys create my-sdk` (via the [floop CLI](https://github.com/FloopFloopAI/floop-cli)) or the dashboard → Account → API Keys. Business plan required to mint new keys.

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    floop "github.com/FloopFloopAI/floop-go-sdk"
)

func main() {
    client, err := floop.NewClient(os.Getenv("FLOOP_API_KEY"))
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()

    // Create a project and wait for it to go live.
    created, err := client.Projects.Create(ctx, floop.CreateProjectInput{
        Prompt:    "A landing page for a cat cafe with a sign-up form",
        Name:      "Cat Cafe",
        Subdomain: "cat-cafe",
        BotType:   "site",
    })
    if err != nil {
        log.Fatal(err)
    }

    live, err := client.Projects.WaitForLive(ctx, created.Project.ID, nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Live at:", *live.URL)
}
```

## Polling progress manually

```go
for {
    ev, err := client.Projects.Status(ctx, projectID)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("%s (%d/%d) — %s\n", ev.Status, ev.Step, ev.TotalSteps, ev.Message)
    if ev.Status == "live" || ev.Status == "failed" || ev.Status == "cancelled" {
        break
    }
    time.Sleep(2 * time.Second)
}
```

## Error handling

Every call returns `*floop.Error` on non-2xx. Type-assert via `errors.As` and switch on `.Code`:

```go
var fe *floop.Error
if errors.As(err, &fe) {
    switch fe.Code {
    case "RATE_LIMITED":
        time.Sleep(fe.RetryAfter)
    case "UNAUTHORIZED":
        log.Fatal("Check your FLOOP_API_KEY.")
    default:
        log.Printf("[%s] %s (request %s)", fe.Code, fe.Message, fe.RequestID)
    }
}
```

Known `.Code` values mirror the CLI and Node/Python SDKs: `UNAUTHORIZED`, `FORBIDDEN`, `VALIDATION_ERROR`, `RATE_LIMITED`, `NOT_FOUND`, `CONFLICT`, `SERVICE_UNAVAILABLE`, `SERVER_ERROR`, `NETWORK_ERROR`, `TIMEOUT`, `BUILD_FAILED`, `BUILD_CANCELLED`, `UNKNOWN`. Unknown server codes pass through verbatim — use a `default:` branch.

## Resources

| Namespace           | Methods |
|---|---|
| `client.Projects`   | `Create`, `List`, `Get`, `Status`, `Refine`, `WaitForLive` |
| `client.Subdomains` | `Check`, `Suggest` |
| `client.Secrets`    | `List`, `Set`, `Remove` |
| `client.Library`    | `List`, `Clone` |
| `client.Usage`      | `Summary` |
| `client.ApiKeys`    | `List`, `Create`, `Remove` |
| `client.Uploads`    | `Create` (returns an `UploadedAttachment` you pass into `Projects.Refine`) |
| `client.User`       | `Me` |

Surface parity with the Node and Python SDKs is complete. The only gap left is the streaming iterator equivalent of `projects.stream()` — use the `Projects.Status` polling loop shown above, or `Projects.WaitForLive` for the blocking variant, until that lands.

## Uploading an attachment

```go
data, _ := os.ReadFile("./screenshot.png")
att, err := client.Uploads.Create(ctx, floop.CreateUploadInput{
    FileName: "screenshot.png",
    Bytes:    data,
})
if err != nil { log.Fatal(err) }

_, err = client.Projects.Refine(ctx, "my-project", floop.RefineInput{
    Message:     "Re-do the landing page based on this screenshot.",
    Attachments: []floop.RefineAttachment{{
        Key: att.Key, FileName: att.FileName, FileType: att.FileType, FileSize: att.FileSize,
    }},
})
```

Streams are supported too — pass `File: io.Reader` + `Size: int64` instead of `Bytes` for large files (max 5 MB). Allowed types: png, jpg, gif, svg, webp, ico, pdf, txt, csv, doc, docx.

## Configuration

```go
client, _ := floop.NewClient(apiKey,
    floop.WithBaseURL("https://staging.floopfloop.com"),  // staging or local dev
    floop.WithTimeout(60*time.Second),                    // default 30s
    floop.WithUserAgent("myapp/1.2"),                     // appended after floop-go-sdk/<v>
    floop.WithHTTPClient(myHTTPClient),                   // bring your own *http.Client
)
```

Every method takes a `context.Context` — cancel it to abort an in-flight request. `WaitForLive` also honours the context's deadline.

## Versioning

Follows [Semantic Versioning](https://semver.org/). Breaking changes in `0.x` are called out in [CHANGELOG.md](./CHANGELOG.md) and a new tag is cut with `v<version>` (the plain-`v` prefix is required by Go modules — `go get github.com/FloopFloopAI/floop-go-sdk@v0.1.0-alpha.1` would fail with any other prefix).

## License

MIT
