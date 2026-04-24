# Changelog

All notable changes to `github.com/FloopFloopAI/floop-go-sdk` are documented
in this file. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This SDK follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
- Uploads (S3 presign + direct PUT — needs careful binary-body handling).
- Streaming / async iterator equivalent of the Node SDK's
  `projects.stream()`.
