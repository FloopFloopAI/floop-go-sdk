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

// WaitForLiveOptions configures the polling behaviour.
type WaitForLiveOptions struct {
	// Interval between polls. Defaults to 2s.
	Interval time.Duration
	// MaxWait bounds the total polling duration. Defaults to 10 minutes.
	MaxWait time.Duration
}

// WaitForLive blocks until the project reaches a terminal state (live,
// failed, cancelled). Returns the final Project on "live", or a typed
// Error with Code "BUILD_FAILED" / "BUILD_CANCELLED" otherwise. The
// context's deadline is honoured.
//
// Pass nil for opts to use defaults (2s poll, 10min ceiling).
func (p *Projects) WaitForLive(ctx context.Context, ref string, opts *WaitForLiveOptions) (*Project, error) {
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

	var last *StatusEvent
	for {
		ev, err := p.Status(deadlineCtx, ref)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, &Error{
					Code:    "TIMEOUT",
					Message: fmt.Sprintf("WaitForLive: project %s did not reach a terminal state within %s", ref, maxWait),
				}
			}
			return nil, err
		}
		last = ev

		switch ev.Status {
		case "live":
			// Fetch the fully-hydrated Project for the URL, etc.
			return p.Get(ctx, ref, ListProjectsOptions{})
		case "failed":
			return nil, &Error{
				Code:    "BUILD_FAILED",
				Message: nonEmptyOr(ev.Message, "build failed"),
			}
		case "cancelled":
			return nil, &Error{
				Code:    "BUILD_CANCELLED",
				Message: nonEmptyOr(ev.Message, "build cancelled"),
			}
		}

		select {
		case <-time.After(interval):
			// loop
		case <-deadlineCtx.Done():
			msg := "WaitForLive: polling deadline exceeded"
			if last != nil {
				msg = fmt.Sprintf("%s (last status: %s)", msg, last.Status)
			}
			return nil, &Error{Code: "TIMEOUT", Message: msg}
		}
	}
}

func nonEmptyOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
