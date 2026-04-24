package floop

import (
	"context"
	"fmt"
	"net/url"
)

// ApiKeySummary is a masked view of an API key. Values in `Scopes` are
// left as json.RawMessage equivalent (`any`) because the shape is still
// evolving server-side — callers that care can assert into a concrete
// type.
type ApiKeySummary struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	KeyPrefix  string  `json:"keyPrefix"`
	Scopes     any     `json:"scopes"`
	LastUsedAt *string `json:"lastUsedAt"`
	CreatedAt  string  `json:"createdAt"`
}

// IssuedApiKey is the /api/v1/api-keys POST response. `RawKey` is the
// *only* time the full secret is returned — surface it to the user
// exactly once and never persist it.
type IssuedApiKey struct {
	ID        string `json:"id"`
	RawKey    string `json:"rawKey"`
	KeyPrefix string `json:"keyPrefix"`
}

// ApiKeys is the resource namespace for programmatic API-key management.
// Minting new keys requires the Business plan.
type ApiKeys struct {
	client *Client
}

type apiKeysListResponse struct {
	Keys []ApiKeySummary `json:"keys"`
}

// List returns every API key the caller has issued.
func (a *ApiKeys) List(ctx context.Context) ([]ApiKeySummary, error) {
	var out apiKeysListResponse
	if err := a.client.request(ctx, "GET", "/api/v1/api-keys", nil, &out); err != nil {
		return nil, err
	}
	return out.Keys, nil
}

// CreateApiKeyInput is the payload for ApiKeys.Create.
type CreateApiKeyInput struct {
	Name string `json:"name"`
}

// Create mints a new API key. The returned IssuedApiKey is the *only*
// time the full secret is exposed — present it to the user, then forget.
func (a *ApiKeys) Create(ctx context.Context, input CreateApiKeyInput) (*IssuedApiKey, error) {
	var out IssuedApiKey
	if err := a.client.request(ctx, "POST", "/api/v1/api-keys", input, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type apiKeysDeleteResponse struct {
	Success bool `json:"success"`
}

// Remove revokes an API key by id or human-readable name. It lists first
// to accept either form, then calls DELETE with the resolved id.
func (a *ApiKeys) Remove(ctx context.Context, idOrName string) error {
	all, err := a.List(ctx)
	if err != nil {
		return err
	}
	var id string
	for i := range all {
		if all[i].ID == idOrName || all[i].Name == idOrName {
			id = all[i].ID
			break
		}
	}
	if id == "" {
		return &Error{
			Code:    "NOT_FOUND",
			Status:  404,
			Message: fmt.Sprintf("API key not found: %s", idOrName),
		}
	}
	var out apiKeysDeleteResponse
	return a.client.request(ctx, "DELETE", "/api/v1/api-keys/"+url.PathEscape(id), nil, &out)
}
