package floop

import (
	"context"
	"net/url"
)

// SubdomainCheckResult is returned by Subdomains.Check.
type SubdomainCheckResult struct {
	Slug      string `json:"slug"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// SubdomainSuggestResult is returned by Subdomains.Suggest.
type SubdomainSuggestResult struct {
	Slug string `json:"slug"`
}

// Subdomains is the resource namespace for subdomain availability helpers.
type Subdomains struct {
	client *Client
}

// Check asks the backend whether a subdomain slug is free to claim.
func (s *Subdomains) Check(ctx context.Context, slug string) (*SubdomainCheckResult, error) {
	var out SubdomainCheckResult
	path := "/api/v1/subdomains/check?slug=" + url.QueryEscape(slug)
	if err := s.client.request(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Suggest asks the backend to generate a slug for a natural-language prompt.
func (s *Subdomains) Suggest(ctx context.Context, prompt string) (*SubdomainSuggestResult, error) {
	var out SubdomainSuggestResult
	path := "/api/v1/subdomains/suggest?prompt=" + url.QueryEscape(prompt)
	if err := s.client.request(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
