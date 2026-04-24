package floop

import "context"

// User describes the authenticated caller.
type User struct {
	ID    string  `json:"id"`
	Email *string `json:"email"`
	Name  *string `json:"name"`
	Plan  *string `json:"plan,omitempty"`
}

// UserAPI is the resource namespace for account-level calls.
//
// We use UserAPI (not User) for the namespace because User is the payload
// type and Go doesn't let a struct method collide with a field type name.
type UserAPI struct {
	client *Client
}

// Me returns the currently-authenticated user.
func (u *UserAPI) Me(ctx context.Context) (*User, error) {
	var out User
	if err := u.client.request(ctx, "GET", "/api/v1/user/me", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
