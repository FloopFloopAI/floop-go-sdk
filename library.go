package floop

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
)

// LibraryProject is an entry in the public project library.
type LibraryProject struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Subdomain   *string `json:"subdomain"`
	BotType     *string `json:"botType"`
	CloneCount  int     `json:"cloneCount"`
	CreatedAt   string  `json:"createdAt"`
}

// LibraryListOptions are the query filters for Library.List.
type LibraryListOptions struct {
	BotType string
	Search  string
	// Sort accepts "popular" or "newest". Empty = server default.
	Sort  string
	Page  int
	Limit int
}

// ClonedProject is returned by Library.Clone.
type ClonedProject struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Subdomain *string `json:"subdomain"`
	Status    string  `json:"status"`
}

// Library is the resource namespace for the public project library.
type Library struct {
	client *Client
}

// List returns library projects filtered by the given options. The backend
// paginates; pass Page / Limit to page through results.
func (l *Library) List(ctx context.Context, opts LibraryListOptions) ([]LibraryProject, error) {
	params := url.Values{}
	if opts.BotType != "" {
		params.Set("botType", opts.BotType)
	}
	if opts.Search != "" {
		params.Set("search", opts.Search)
	}
	if opts.Sort != "" {
		params.Set("sort", opts.Sort)
	}
	if opts.Page > 0 {
		params.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	path := "/api/v1/library"
	if qs := params.Encode(); qs != "" {
		path += "?" + qs
	}

	// The backend is currently permissive about the response shape: it
	// can be a bare array OR an {items: [...]} object. Decode into a
	// RawMessage + probe, matching the Node SDK's behaviour.
	var raw json.RawMessage
	if err := l.client.request(ctx, "GET", path, nil, &raw); err != nil {
		return nil, err
	}

	var arr []LibraryProject
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	var wrapped struct {
		Items []LibraryProject `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		return wrapped.Items, nil
	}
	return nil, &Error{
		Code:    "UNKNOWN",
		Message: "library list: unrecognised response shape",
	}
}

// CloneLibraryProjectInput is the payload for Library.Clone.
type CloneLibraryProjectInput struct {
	Subdomain string `json:"subdomain"`
}

// Clone duplicates a public library project into the user's account under
// the given subdomain.
func (l *Library) Clone(ctx context.Context, projectID string, input CloneLibraryProjectInput) (*ClonedProject, error) {
	var out ClonedProject
	path := "/api/v1/library/" + url.PathEscape(projectID) + "/clone"
	if err := l.client.request(ctx, "POST", path, input, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
