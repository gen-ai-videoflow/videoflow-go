package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRuntimePolicyRejectsUnsafeBoundsAndMargins(t *testing.T) {
	policy := runtimePolicyFromConfig(Config{CreditsPerSecond: 10, StoryCredits: 2, AssemblyCredits: 2, ProviderMaxCostMicros: 25000, StoryMaxCostMicros: 5000, ProviderDailyBudgetMicros: 100000, PremiumPriceCents: 900})
	policy.StoryModels = []string{"reviewed/story"}
	policy.VideoModels = []string{"reviewed/video"}
	if err := validateRuntimePolicy(policy); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*RuntimePolicy){func(p *RuntimePolicy) { p.MaxActiveJobs = 100 }, func(p *RuntimePolicy) { p.PremiumPriceCents = 1 }, func(p *RuntimePolicy) { p.StoryCredits = 1; p.StoryMaxCostMicros = 100000 }, func(p *RuntimePolicy) { p.ProviderMaxCostMicros = 100000000 }, func(p *RuntimePolicy) { p.VideoModels = []string{"unknown:free"} }} {
		bad := policy
		change(&bad)
		if validateRuntimePolicy(bad) == nil {
			t.Fatal("unsafe policy accepted")
		}
	}
}
func TestRuntimePolicyCASAndAuditLeaveExistingPurchasesUnchanged(t *testing.T) {
	store, u := testPlatform(t)
	store.DB.Exec(`DELETE FROM cloud_runtime_policy`)
	store.DB.Exec(`DELETE FROM cloud_runtime_policy_versions`)
	t.Cleanup(func() {
		store.DB.Exec(`DELETE FROM cloud_runtime_policy`)
		store.DB.Exec(`DELETE FROM cloud_runtime_policy_versions`)
	})
	ctx := context.Background()
	base := Config{CreditsPerSecond: 10, StoryCredits: 2, AssemblyCredits: 2, ProviderMaxCostMicros: 25000, StoryMaxCostMicros: 5000, ProviderDailyBudgetMicros: 100000, PremiumPriceCents: 900}
	project, err := store.CreateProject(ctx, u, "Frozen policy", " ")
	if err != nil {
		t.Fatal(err)
	}
	frozenQuote, err := store.CreateQuote(ctx, u, QuoteInput{ProjectID: project.ID, Prompt: "An original story", Duration: 5, AspectRatio: "16:9"}, base.CreditsPerSecond)
	if err != nil {
		t.Fatal(err)
	}
	paidThrough := time.Now().Add(24 * time.Hour)
	if _, err = store.DB.Exec(`INSERT INTO cloud_premium_subscriptions(user_id,provider_subscription,status,paid_through)VALUES($1,$2,'active',$3)`, u.ID, "sub_runtime_"+u.ID, paidThrough); err != nil {
		t.Fatal(err)
	}
	policy := runtimePolicyFromConfig(base)
	record, err := store.saveRuntimePolicy(ctx, u.ID, 0, policy, "reviewed bootstrap")
	if err != nil || record.Revision != 1 {
		t.Fatalf("save %+v %v", record, err)
	}
	policy.PremiumPriceCents = 1200
	policy.CreditsPerSecond = 20
	policy.MaxActiveJobs = 1
	if _, err = store.saveRuntimePolicy(ctx, u.ID, 0, policy, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale policy %v", err)
	}
	record, err = store.saveRuntimePolicy(ctx, u.ID, 1, policy, "new sales price")
	if err != nil || record.Revision != 2 {
		t.Fatal(err)
	}
	cfg, err := store.EffectiveConfig(ctx, base)
	if err != nil || cfg.PremiumPriceCents != 1200 || base.PremiumPriceCents != 900 {
		t.Fatalf("effective %+v %v", cfg, err)
	}
	newQuote, err := store.CreateQuote(ctx, u, QuoteInput{ProjectID: project.ID, Prompt: "A new original story", Duration: 5, AspectRatio: "16:9"}, cfg.CreditsPerSecond)
	if err != nil || newQuote.Credits != 100 {
		t.Fatalf("new quote did not use effective pricing: %+v %v", newQuote, err)
	}
	if err = store.GrantCredits(ctx, u.WorkspaceID, "runtime-fixture", 1000, "fixture"); err != nil {
		t.Fatal(err)
	}
	job, err := store.SubmitJob(ctx, u, frozenQuote.ID, "runtime-frozen-quote")
	if err != nil || job.Credits != 50 {
		t.Fatalf("frozen quote submit %+v %v", job, err)
	}
	if _, err = store.SubmitJob(ctx, u, newQuote.ID, "runtime-quota-new"); !errors.Is(err, ErrConflict) {
		t.Fatalf("active quota bypass: %v", err)
	}
	if again, err := store.SubmitJob(ctx, u, frozenQuote.ID, "runtime-frozen-quote"); err != nil || again.ID != job.ID {
		t.Fatalf("quota broke same-key recovery: %v", err)
	}
	var frozenCredits int64
	if err = store.DB.QueryRow(`SELECT credits FROM cloud_quotes WHERE id=$1`, frozenQuote.ID).Scan(&frozenCredits); err != nil || frozenCredits != 50 {
		t.Fatalf("confirmed quote repriced: %d %v", frozenCredits, err)
	}
	if active, err := store.HasPremium(ctx, u.ID); err != nil || !active {
		t.Fatalf("existing subscription invalidated: %v %v", active, err)
	}
	var count int
	if err = store.DB.QueryRow(`SELECT count(*) FROM cloud_admin_audit WHERE actor=$1 AND action='runtime_policy'`, u.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("audit %d %v", count, err)
	}
	var versions int
	if err = store.DB.QueryRow(`SELECT count(*) FROM cloud_runtime_policy_versions WHERE updated_by=$1`, u.ID).Scan(&versions); err != nil || versions != 2 {
		t.Fatalf("immutable version history %d %v", versions, err)
	}
	raw, _ := json.Marshal(record)
	if string(raw) == "" {
		t.Fatal("empty policy")
	}
}

func TestRuntimePolicyUnknownModelsFailClosed(t *testing.T) {
	store, _ := testPlatform(t)
	policy := runtimePolicyFromConfig(Config{CreditsPerSecond: 10, StoryCredits: 2, AssemblyCredits: 2, ProviderMaxCostMicros: 25000, StoryMaxCostMicros: 5000, ProviderDailyBudgetMicros: 100000})
	policy.StoryModels = []string{"invented/unsupported-model"}
	if err := store.validateRuntimeModels(context.Background(), policy); err == nil {
		t.Fatal("unknown story model accepted")
	}
	policy.StoryModels = []string{}
	policy.VideoModels = []string{"invented/unsupported-model"}
	if err := store.validateRuntimeModels(context.Background(), policy); err == nil {
		t.Fatal("unknown video model accepted")
	}
}

func TestRuntimePremiumPendingCheckoutRetainsFrozenPrice(t *testing.T) {
	store, u := testPlatform(t)
	store.DB.Exec(`DELETE FROM cloud_runtime_policy`)
	store.DB.Exec(`DELETE FROM cloud_runtime_policy_versions`)
	t.Cleanup(func() {
		store.DB.Exec(`DELETE FROM cloud_runtime_policy`)
		store.DB.Exec(`DELETE FROM cloud_runtime_policy_versions`)
	})
	cfg := Config{BillingEnabled: true, StripeSecret: "test", StripeWebhookSecret: "test", CreditsPerSecond: 10, StoryCredits: 2, AssemblyCredits: 2, ProviderMaxCostMicros: 25000, StoryMaxCostMicros: 5000, ProviderDailyBudgetMicros: 100000, PremiumPriceCents: 900}
	server := NewServer(store, cfg)
	defer server.Close()
	calls := 0
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"cs_frozen_` + u.ID + `","url":"https://checkout.stripe.com/c/pay/frozen"}`))}, nil
	})}
	first, err := server.startPremiumCheckout(context.Background(), u, "runtime-frozen-premium")
	if err != nil {
		t.Fatal(err)
	}
	p := runtimePolicyFromConfig(cfg)
	p.PremiumPriceCents = 1200
	if _, err = store.saveRuntimePolicy(context.Background(), u.ID, 0, p, "new sales price"); err != nil {
		t.Fatal(err)
	}
	second, err := server.startPremiumCheckout(context.Background(), u, "runtime-frozen-premium")
	if err != nil || second != first || calls != 1 {
		t.Fatalf("pending checkout repriced or repeated provider call: %v %d", err, calls)
	}
	var price int64
	if err = store.DB.QueryRow(`SELECT price_cents FROM cloud_premium_checkouts WHERE user_id=$1`, u.ID).Scan(&price); err != nil || price != 900 {
		t.Fatalf("frozen checkout price %d %v", price, err)
	}
}
