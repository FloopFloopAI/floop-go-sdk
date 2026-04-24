package floop

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"
)

// Project represents a FloopFloop project as returned by /api/v1/projects.
type Project struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Subdomain       *string `json:"subdomain"`
	Status          string  `json:"status"`
	BotType         *string `json:"botType"`
	URL             *string `json:"url"`
	AmplifyAppURL   *string `json:"amplifyAppUrl"`
	IsPublic        bool    `json:"isPublic"`
	IsAuthProtected bool    `json:"isAuthProtected"`
	TeamID          *string `json:"teamId"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
	ThumbnailURL    *string `json:"thumbnailUrl,omitempty"`
}

// CreateProjectInput is the payload for Projects.Create.
type CreateProjectInput struct {
	Prompt          string `json:"prompt"`
	Name            string `json:"name,omitempty"`
	Subdomain       string `json:"subdomain,omitempty"`
	BotType         string `json:"botType,omitempty"`
	IsAuthProtected *bool  `json:"isAuthProtected,omitempty"`
	TeamID          string `json:"teamId,omitempty"`
}

// CreatedProject is the /api/v1/projects POST response.
type CreatedProject struct {
	Project    Project `json:"project"`
	Deployment struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Version int    `json:"version"`
	} `json:"deployment"`
}

// ListProjectsOptions scopes a Projects.List call to a team.
type ListProjectsOptions struct {
	TeamID string
}

// StatusEvent is the /api/v1/projects/{ref}/status response shape.
type StatusEvent struct {
	Step          int    `json:"step"`
	TotalSteps    int    `json:"totalSteps"`
	Status        string `json:"status"`
	Message       string `json:"message"`
	Progress      *float64 `json:"progress,omitempty"`
	QueuePosition *int     `json:"queuePosition,omitempty"`
}

// TerminalProjectStatuses — Projects.WaitForLive stops polling once status
// is one of these. Mirrors the Node/Python SDK sets.
var TerminalProjectStatuses = map[string]bool{
	"live":      true,
	"failed":    true,
	"cancelled": true,
}

// RefineInput is the payload for Projects.Refine.
type RefineInput struct {
	Message       string              `json:"message"`
	Attachments   []RefineAttachment  `json:"attachments,omitempty"`
	CodeEditOnly  bool                `json:"codeEditOnly,omitempty"`
}

// RefineAttachment references a previously-uploaded file.
type RefineAttachment struct {
	Key      string `json:"key"`
	FileName string `json:"fileName"`
	FileType string `json:"fileType"`
	FileSize int    `json:"fileSize"`
}

// RefineResult is a discriminated-union-ish response body. Exactly one of
// Queued / SavedOnly / Processing will be set per the backend's contract.
type RefineResult struct {
	Queued        *RefineQueued     `json:"-"`
	SavedOnly     *RefineSavedOnly  `json:"-"`
	Processing    *RefineProcessing `json:"-"`
	rawBody       []byte            // populated by Refine for custom unmarshalling
}

type RefineQueued struct {
	Queued    bool   `json:"queued"`
	MessageID string `json:"messageId"`
}

type RefineSavedOnly struct {
	Queued bool `json:"queued"` // false
}

type RefineProcessing struct {
	Processing    bool `json:"processing"`
	DeploymentID  string `json:"deploymentId"`
	QueuePriority int    `json:"queuePriority"`
}

// Projects is the resource namespace for project CRUD + refinement.
type Projects struct {
	client *Client
}

// Create kicks off a new project build.
func (p *Projects) Create(ctx context.Context, input CreateProjectInput) (*CreatedProject, error) {
	var out CreatedProject
	if err := p.client.request(ctx, "POST", "/api/v1/projects", input, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns every project the authenticated user has access to, optionally
// scoped to a team.
func (p *Projects) List(ctx context.Context, opts ListProjectsOptions) ([]Project, error) {
	path := "/api/v1/projects"
	if opts.TeamID != "" {
		path += "?teamId=" + url.QueryEscape(opts.TeamID)
	}
	var out []Project
	if err := p.client.request(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Get fetches a single project by id or subdomain. There is no server-side
// /api/v1/projects/:id endpoint — this filters List locally, matching the
// behaviour of the Node and Python SDKs.
func (p *Projects) Get(ctx context.Context, ref string, opts ListProjectsOptions) (*Project, error) {
	all, err := p.List(ctx, opts)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == ref || (all[i].Subdomain != nil && *all[i].Subdomain == ref) {
			return &all[i], nil
		}
	}
	return nil, &Error{
		Code:    "NOT_FOUND",
		Status:  404,
		Message: fmt.Sprintf("project not found: %s", ref),
	}
}

// Status returns the current build/deploy status for a project.
func (p *Projects) Status(ctx context.Context, ref string) (*StatusEvent, error) {
	var out StatusEvent
	if err := p.client.request(ctx, "GET", "/api/v1/projects/"+url.PathEscape(ref)+"/status", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Refine sends a refinement instruction to an existing project. The backend
// responds with one of three shapes — this method inspects the raw body and
// populates the correct field on RefineResult.
func (p *Projects) Refine(ctx context.Context, ref string, input RefineInput) (*RefineResult, error) {
	// Use the map-probe trick from the envelope parser: unmarshal into a
	// generic map, then decide which concrete shape to materialise.
	var raw map[string]any
	if err := p.client.request(ctx, "POST", "/api/v1/projects/"+url.PathEscape(ref)+"/refine", input, &raw); err != nil {
		return nil, err
	}
	res := &RefineResult{}
	if v, ok := raw["queued"].(bool); ok {
		if v {
			msgID, _ := raw["messageId"].(string)
			res.Queued = &RefineQueued{Queued: true, MessageID: msgID}
		} else {
			res.SavedOnly = &RefineSavedOnly{Queued: false}
		}
		return res, nil
	}
	if v, ok := raw["processing"].(bool); ok && v {
		depID, _ := raw["deploymentId"].(string)
		prio := 0
		if f, ok := raw["queuePriority"].(float64); ok {
			prio = int(f)
		}
		res.Processing = &RefineProcessing{Processing: true, DeploymentID: depID, QueuePriority: prio}
		return res, nil
	}
	return nil, &Error{
		Code:    "UNKNOWN",
		Message: "refine: unrecognised response shape",
	}
}

// StreamOptions configures Projects.Stream and Projects.WaitForLive
// polling behaviour.
type StreamOptions struct {
	// Interval between polls. Defaults to 2s.
	Interval time.Duration
	// MaxWait bounds the total polling duration. Defaults to 10 minutes.
	MaxWait time.Duration
}

// WaitForLiveOptions is an alias for StreamOptions kept for backwards
// compatibility with 0.1.0-alpha.1 / alpha.2 callers. Prefer
// StreamOptions in new code.
type WaitForLiveOptions = StreamOptions

// StreamHandler is called for every de-duplicated status snapshot —
// including the terminal event (live / failed / cancelled) — as Projects.Stream
// polls. Return a non-nil error to stop polling early; that error is
// returned verbatim from Stream so callers can distinguish it from the
// transport / terminal errors the SDK generates.
type StreamHandler func(ev StatusEvent) error

// Stream polls the project's status endpoint, invoking handler on each
// unique event, until the project reaches a terminal state (live /
// failed / cancelled), the context deadline fires, opts.MaxWait
// elapses, or the handler returns an error.
//
// Return values:
//   - nil            — project reached "live" cleanly
//   - *floop.Error{Code: "BUILD_FAILED"} / "BUILD_CANCELLED" — terminal fail
//   - *floop.Error{Code: "TIMEOUT"}      — MaxWait exceeded
//   - the handler's error verbatim       — handler stopped early
//   - any other error from the transport  — e.g. NETWORK_ERROR
//
// Events are de-duplicated on the (status, step, progress, queuePosition)
// tuple so callers don't see dozens of identical "queued" snapshots
// while a build waits — matches the Node and Python SDKs.
//
// Pass nil for opts to use defaults (2 s interval, 10 min ceiling).
//
// Example:
//
//	err := client.Projects.Stream(ctx, "my-project", nil, func(ev floop.StatusEvent) error {
//	    fmt.Printf("%s (%d/%d): %s\n", ev.Status, ev.Step, ev.TotalSteps, ev.Message)
//	    return nil
//	})
//	var fe *floop.Error
//	if errors.As(err, &fe) && fe.Code == "BUILD_FAILED" {
//	    // ...
//	}
func (p *Projects) Stream(ctx context.Context, ref string, opts *StreamOptions, handler StreamHandler) error {
	interval := 2 * time.Second
	maxWait := 10 * time.Minute
	if opts != nil {
		if opts.Interval > 0 {
			interval = opts.Interval
		}
		if opts.MaxWait > 0 {
			maxWait = opts.MaxWait
		}
	}

	deadlineCtx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()

	var lastKey string
	for {
		ev, err := p.Status(deadlineCtx, ref)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return &Error{
					Code:    "TIMEOUT",
					Message: fmt.Sprintf("Stream: project %s did not reach a terminal state within %s", ref, maxWait),
				}
			}
			return err
		}

		key := streamEventKey(ev)
		if key != lastKey {
			lastKey = key
			if handler != nil {
				if herr := handler(*ev); herr != nil {
					return herr
				}
			}
		}

		switch ev.Status {
		case "live":
			return nil
		case "failed":
			return &Error{
				Code:    "BUILD_FAILED",
				Message: nonEmptyOr(ev.Message, "build failed"),
			}
		case "cancelled":
			return &Error{
				Code:    "BUILD_CANCELLED",
				Message: nonEmptyOr(ev.Message, "build cancelled"),
			}
		}

		select {
		case <-time.After(interval):
			// loop
		case <-deadlineCtx.Done():
			return &Error{
				Code:    "TIMEOUT",
				Message: fmt.Sprintf("Stream: polling deadline exceeded (last status: %s)", ev.Status),
			}
		}
	}
}

// WaitForLive blocks until the project reaches a terminal state (live,
// failed, cancelled). Returns the final Project on "live", or a typed
// Error with Code "BUILD_FAILED" / "BUILD_CANCELLED" otherwise. The
// context's deadline is honoured.
//
// Pass nil for opts to use defaults (2s poll, 10min ceiling).
//
// Implemented on top of Projects.Stream. Callers who want to observe
// intermediate events should use Stream directly.
func (p *Projects) WaitForLive(ctx context.Context, ref string, opts *WaitForLiveOptions) (*Project, error) {
	if err := p.Stream(ctx, ref, opts, nil); err != nil {
		return nil, err
	}
	return p.Get(ctx, ref, ListProjectsOptions{})
}

// streamEventKey produces the de-duplication key for two consecutive
// status snapshots. Must match the keys used by the Node and Python
// SDKs so all three see the same event stream.
func streamEventKey(ev *StatusEvent) string {
	progress := ""
	if ev.Progress != nil {
		progress = fmt.Sprintf("%g", *ev.Progress)
	}
	queuePos := ""
	if ev.QueuePosition != nil {
		queuePos = fmt.Sprintf("%d", *ev.QueuePosition)
	}
	return fmt.Sprintf("%s|%d|%s|%s", ev.Status, ev.Step, progress, queuePos)
}

func nonEmptyOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
