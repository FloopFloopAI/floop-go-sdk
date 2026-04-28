package floop

import "context"

// SubscriptionPlan is the plan + billing snapshot returned by
// /api/v1/subscriptions/current. Sensitive fields (Stripe customer /
// subscription IDs, invoice metadata) are deliberately omitted from the
// wire shape on the backend.
type SubscriptionPlan struct {
	Status               string  `json:"status"`
	BillingPeriod        *string `json:"billingPeriod"`
	CurrentPeriodStart   string  `json:"currentPeriodStart"`
	CurrentPeriodEnd     string  `json:"currentPeriodEnd"`
	CanceledAt           *string `json:"canceledAt"`
	PlanName             string  `json:"planName"`
	PlanDisplayName      string  `json:"planDisplayName"`
	PriceMonthly         int     `json:"priceMonthly"`
	PriceAnnual          int     `json:"priceAnnual"`
	MonthlyCredits       int     `json:"monthlyCredits"`
	MaxProjects          int     `json:"maxProjects"`
	MaxStorageMb         int     `json:"maxStorageMb"`
	MaxBandwidthMb       int     `json:"maxBandwidthMb"`
	CreditRolloverMonths int     `json:"creditRolloverMonths"`
	// Features is a free-form object; decoded as map[string]any so callers
	// can inspect feature flags without us cutting a release each time the
	// backend grows a new key.
	Features map[string]any `json:"features"`
}

// SubscriptionCredits is the credit-balance half of the
// /api/v1/subscriptions/current response.
type SubscriptionCredits struct {
	Current           int     `json:"current"`
	RolledOver        int     `json:"rolledOver"`
	Total             int     `json:"total"`
	RolloverExpiresAt *string `json:"rolloverExpiresAt"`
	LifetimeUsed      int     `json:"lifetimeUsed"`
}

// CurrentSubscription is the response envelope for
// Subscriptions.Current. Both fields are independently nullable: a user
// may exist without an active subscription (mid-signup, cancelled with no
// grace credits remaining). Treat nil as "no active subscription data"
// rather than an error.
type CurrentSubscription struct {
	Subscription *SubscriptionPlan    `json:"subscription"`
	Credits      *SubscriptionCredits `json:"credits"`
}

// Subscriptions is the resource namespace for plan + credit-balance.
//
// Distinct from Usage — Usage.Summary returns current-period consumption
// (credits remaining + builds used + storage), while
// Subscriptions.Current returns the plan tier itself (price, billing
// period, cancel state). They overlap on MonthlyCredits and MaxProjects
// but serve different audiences: usage for "am I about to hit my limits?",
// current for "what plan am I on, and when does it renew?".
type Subscriptions struct {
	client *Client
}

// Current fetches the authenticated user's current subscription + credit
// snapshot. Read-only; cheap to call.
func (s *Subscriptions) Current(ctx context.Context) (*CurrentSubscription, error) {
	var out CurrentSubscription
	if err := s.client.request(ctx, "GET", "/api/v1/subscriptions/current", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
