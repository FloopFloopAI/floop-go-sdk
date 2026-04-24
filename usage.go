package floop

import "context"

// UsageSummary is the /api/v1/usage/summary response.
type UsageSummary struct {
	Plan struct {
		Name            string `json:"name"`
		DisplayName     string `json:"displayName"`
		MonthlyCredits  int    `json:"monthlyCredits"`
		MaxProjects     int    `json:"maxProjects"`
		MaxStorageMb    int    `json:"maxStorageMb"`
		MaxBandwidthMb  int    `json:"maxBandwidthMb"`
	} `json:"plan"`
	Credits struct {
		CurrentCredits       int     `json:"currentCredits"`
		RolledOverCredits    int     `json:"rolledOverCredits"`
		LifetimeCreditsUsed  int     `json:"lifetimeCreditsUsed"`
		RolloverExpiresAt    *string `json:"rolloverExpiresAt"`
	} `json:"credits"`
	CurrentPeriod struct {
		Start             string `json:"start"`
		End               string `json:"end"`
		ProjectsCreated   int    `json:"projectsCreated"`
		BuildsUsed        int    `json:"buildsUsed"`
		RefinementsUsed   int    `json:"refinementsUsed"`
		StorageUsedMb     int    `json:"storageUsedMb"`
		BandwidthUsedMb   int    `json:"bandwidthUsedMb"`
	} `json:"currentPeriod"`
}

// Usage is the resource namespace for plan + usage reporting.
type Usage struct {
	client *Client
}

// Summary returns the authenticated user's plan + current-period usage.
func (u *Usage) Summary(ctx context.Context) (*UsageSummary, error) {
	var out UsageSummary
	if err := u.client.request(ctx, "GET", "/api/v1/usage/summary", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
