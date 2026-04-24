package floop

import (
	"context"
	"net/url"
)

// SecretSummary is a single entry returned by Secrets.List. FloopFloop
// never returns secret values — only names. Rotate by re-setting.
type SecretSummary struct {
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// Secrets is the resource namespace for per-project environment secrets.
type Secrets struct {
	client *Client
}

type secretsListResponse struct {
	Secrets []SecretSummary `json:"secrets"`
}

// List returns the secret keys set on a project.
func (s *Secrets) List(ctx context.Context, ref string) ([]SecretSummary, error) {
	var out secretsListResponse
	if err := s.client.request(ctx, "GET", "/api/v1/projects/"+url.PathEscape(ref)+"/secrets", nil, &out); err != nil {
		return nil, err
	}
	return out.Secrets, nil
}

type setSecretRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Set creates or overwrites a secret on a project.
func (s *Secrets) Set(ctx context.Context, ref, name, value string) error {
	return s.client.request(ctx, "POST", "/api/v1/projects/"+url.PathEscape(ref)+"/secrets", setSecretRequest{Name: name, Value: value}, nil)
}

// Remove deletes a secret from a project.
func (s *Secrets) Remove(ctx context.Context, ref, name string) error {
	return s.client.request(ctx, "DELETE", "/api/v1/projects/"+url.PathEscape(ref)+"/secrets/"+url.PathEscape(name), nil, nil)
}
