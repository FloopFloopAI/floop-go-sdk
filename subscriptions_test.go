package floop

import (
	"context"
	"net/http"
	"testing"
)

func TestSubscriptions_Current_Populated(t *testing.T) {
	var seenPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Write([]byte(`{"data":{
			"subscription":{
				"status":"active",
				"billingPeriod":"monthly",
				"currentPeriodStart":"2026-04-01T00:00:00Z",
				"currentPeriodEnd":"2026-05-01T00:00:00Z",
				"canceledAt":null,
				"planName":"pro",
				"planDisplayName":"Pro",
				"priceMonthly":29,
				"priceAnnual":290,
				"monthlyCredits":500,
				"maxProjects":50,
				"maxStorageMb":5000,
				"maxBandwidthMb":50000,
				"creditRolloverMonths":1,
				"features":{"teams":true}
			},
			"credits":{
				"current":423,
				"rolledOver":50,
				"total":473,
				"rolloverExpiresAt":"2026-05-01T00:00:00Z",
				"lifetimeUsed":1234
			}
		}}`))
	})

	out, err := c.Subscriptions.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if seenPath != "/api/v1/subscriptions/current" {
		t.Errorf("path: %s", seenPath)
	}
	if out.Subscription == nil {
		t.Fatal("Subscription: got nil, want populated")
	}
	if out.Subscription.PlanName != "pro" {
		t.Errorf("PlanName: got %q, want pro", out.Subscription.PlanName)
	}
	if out.Subscription.MonthlyCredits != 500 {
		t.Errorf("MonthlyCredits: got %d, want 500", out.Subscription.MonthlyCredits)
	}
	if out.Credits == nil {
		t.Fatal("Credits: got nil, want populated")
	}
	if out.Credits.Total != 473 {
		t.Errorf("Total: got %d, want 473", out.Credits.Total)
	}
	if out.Credits.RolloverExpiresAt == nil || *out.Credits.RolloverExpiresAt != "2026-05-01T00:00:00Z" {
		t.Errorf("RolloverExpiresAt: got %v", out.Credits.RolloverExpiresAt)
	}
}

func TestSubscriptions_Current_BothNull(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"subscription":null,"credits":null}}`))
	})
	out, err := c.Subscriptions.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if out.Subscription != nil {
		t.Errorf("Subscription: got %+v, want nil", out.Subscription)
	}
	if out.Credits != nil {
		t.Errorf("Credits: got %+v, want nil", out.Credits)
	}
}
