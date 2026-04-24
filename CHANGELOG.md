# Changelog

All notable changes to `github.com/FloopFloopAI/floop-go-sdk` are documented
in this file. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This SDK follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0-alpha.3] — 2026-04-24

### Added
- `client.Projects.Stream(ctx, ref, opts, handler)` — polls the project
  status endpoint and calls `handler` on each de-duplicated event
  (status / step / progress / queuePosition tuple) until a terminal
  state (live / failed / cancelled), the context deadline fires,
  `opts.MaxWait` elapses, or the handler returns an error. Closes the
  last parity gap vs the Node / Python SDKs.
- `StreamOptions` (`Interval`, `MaxWait`) and `StreamHandler` types.
  `WaitForLiveOptions` is now a type alias for `StreamOptions` —
  existing alpha.1 / alpha.2 callers keep compiling.

### Changed
- `Projects.WaitForLive` is now implemented on top of `Projects.Stream`.
  Identical public behaviour; the private polling loop is no longer
  duplicated. Regression tests verify the refactor.
- `Version` constant bumped to `0.1.0-alpha.3`.

### Tests
- 6 new cases covering Stream's dedupe behaviour, terminal-failure
  reporting, early handler-error exit, nil-handler tolerance (what
  WaitForLive passes), MaxWait timeout, and a regression test for the
  refactored WaitForLive. Total 53.

## [0.1.0-alpha.2] — 2026-04-24

### Added
- `client.Uploads.Create(ctx, CreateUploadInput)` — presigns an S3 slot,
  PUTs the file directly, returns an `UploadedAttachment` you can drop
  into `Projects.Refine`'s `Attachments` slice.
- `CreateUploadInput` accepts either `Bytes: []byte` or `File: io.Reader`
  + `Size: int64` (streaming). 5 MB cap, same extension allowlist as the
  Node/Python SDKs (png, jpg, gif, svg, webp, ico, pdf, txt, csv, doc,
  docx). Unknown MIME types and over-size files return
  `*floop.Error{Code: "VALIDATION_ERROR"}` before the presign call.
- `GuessMimeType(fileName)` exported helper.
- 9 new unit tests (two-server `httptest` harness stubs both the
  FloopFloop presign API and S3). Total test count now 47.

### Changed
- `Version` constant bumped to `0.1.0-alpha.2`.

### Deferred to a future release
- Streaming / async iterator equivalent of the Node SDK's
  `projects.stream()`. Use `Projects.Status` in a loop or
  `Projects.WaitForLive` until it lands.

## [0.1.0-alpha.1] — 2026-04-24

### Added
- `floop.Client` with bearer auth, configurable base URL, per-request
  timeout (default 30s), injectable `*http.Client` for proxies/tests,
  and customisable User-Agent suffix.
- Typed `floop.Error` with `Code`, `Status`, `Message`, `RequestID`, and
  `RetryAfter` (RFC 7231-compliant parsing of both delta-seconds and
  HTTP-date formats, matching the Node/Python SDKs).
- Resources: `Projects` (`Create`, `List`, `Get`, `Status`, `Refine`,
  `WaitForLive`), `Subdomains` (`Check`, `Suggest`), `Secrets` (`List`,
  `Set`, `Remove`), `Library` (`List`, `Clone`), `Usage` (`Summary`),
  `ApiKeys` (`List`, `Create`, `Remove`), `User` (`Me`).
- `WaitForLive` polls every 2s by default, honours both `Options.MaxWait`
  and the caller's `context.Context` deadline, and returns a typed
  `BUILD_FAILED` / `BUILD_CANCELLED` `*Error` for non-live terminal
  states.
- `Projects.Refine` discriminates the three backend response shapes
  (queued / saved-only / processing) into separate nullable fields on
  `RefineResult` so callers can switch on presence.
- `Library.List` tolerates both response shapes the backend can emit
  (bare array or `{items: [...]}` envelope), matching the Node SDK.
- `ApiKeys.Remove` accepts either an id or a human-readable name and
  resolves to the id via a preflight List — same ergonomic shortcut as
  the Node SDK's `apiKeys.remove`.
- `testing`-friendly transport: `httptest.NewServer` + `WithBaseURL` is
  the canonical test setup. 38 unit tests cover transport, options,
  every resource, and the polling loop.

### Deferred to a future release
- Uploads (S3 presign + direct PUT — needs careful binary-body handling). *(Landed in 0.1.0-alpha.2.)*
- Streaming / async iterator equivalent of the Node SDK's
  `projects.stream()`.
