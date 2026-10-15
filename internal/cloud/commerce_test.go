package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAllocateRewardsCapsAndDeterministicRemainders(t *testing.T) {
	got, err := AllocateRewards(10001, 2500, 2000, map[string]int64{"b": 1, "a": 1, "c": 1})
	if err != nil || got["a"] != 667 || got["b"] != 667 || got["c"] != 666 {
		t.Fatalf("%v %v", got, err)
	}
	zero, err := AllocateRewards(10000, 0, 2000, map[string]int64{"a": 2})
	if err != nil || len(zero) != 0 {
		t.Fatalf("disabled %v %v", zero, err)
	}
	if _, err = AllocateRewards(10, 10001, 20, map[string]int64{"a": 1}); err == nil {
		t.Fatal("invalid policy accepted")
	}
	if _, err = AllocateRewards(10, 100, 20, map[string]int64{"a": -1}); err == nil {
		t.Fatal("negative weight accepted")
	}
}
func TestAllocateRewardsAvoidsIntegerOverflow(t *testing.T) {
	max := int64(^uint64(0) >> 1)
	got, err := AllocateRewards(max, 10000, max, map[string]int64{"a": max, "b": max})
	if err != nil || got["a"]+got["b"] != max || got["a"] != got["b"]+1 {
		t.Fatalf("%v %v", got, err)
	}
}

type commerceRoundTrip func(*http.Request) (*http.Response, error)

func (f commerceRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMembershipInvoiceAndRefundAreDurableIdempotent(t *testing.T) {
	store, creator := testPlatform(t)
	_, viewer := testPlatform(t)
	ctx := context.Background()
	id := "commerce-" + viewer.ID
	_, err := store.DB.ExecContext(ctx, `INSERT INTO cloud_membership_checkouts(id,user_id,channel_id,price_cents,currency,commission_bps,provider_subscription)VALUES($1,$2,$3,1000,'usd',2000,$4)`, id, viewer.ID, creator.ChannelID, "sub_"+id)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(store, Config{StripeSecret: "sk_test"})
	end := time.Now().Add(30 * 24 * time.Hour).Unix()
	currentStatus := "active"
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		body := `{"id":"sub_` + id + `","status":"` + currentStatus + `","metadata":{"membership_checkout_id":"` + id + `"},"current_period_end":` + intString(end) + `}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	invoice := json.RawMessage(`{"id":"in_` + id + `","subscription":"sub_` + id + `","payment_intent":"pi_` + id + `","amount_paid":1000,"currency":"usd","lines":{"data":[{"period":{"end":` + intString(end) + `}}]}}`)
	errors := make(chan error, 3)
	for _, event := range []string{"evt_1_" + id, "evt_1_" + id, "evt_2_" + id} {
		go func(event string) { errors <- server.processMembershipWebhook(ctx, event, "invoice.paid", invoice) }(event)
	}
	for i := 0; i < 3; i++ {
		if err = <-errors; err != nil {
			t.Fatal(err)
		}
	}

	var amount, count int64
	if err = store.DB.QueryRowContext(ctx, `SELECT coalesce(sum(amount_cents),0),count(*) FROM cloud_earnings WHERE user_id=$1`, creator.ID).Scan(&amount, &count); err != nil || amount != 800 || count != 1 {
		t.Fatalf("earned %d count%d %v", amount, count, err)
	}
	currentStatus = "canceled"
	if err = server.processMembershipWebhook(ctx, "cancel_"+id, "customer.subscription.deleted", json.RawMessage(`{"id":"sub_`+id+`"}`)); err != nil {
		t.Fatal(err)
	}
	var paidAccess bool
	if err = store.DB.QueryRowContext(ctx, `SELECT status='active' AND paid_through>now() FROM cloud_memberships WHERE user_id=$1 AND channel_id=$2`, viewer.ID, creator.ChannelID).Scan(&paidAccess); err != nil || !paidAccess {
		t.Fatalf("cancellation lost paid access: %v", err)
	}
	currentStatus = "active"
	refund := json.RawMessage(`{"id":"ch_` + id + `","payment_intent":"pi_` + id + `","amount":1000,"amount_refunded":500}`)
	for _, event := range []string{"refund1_" + id, "refund2_" + id} {
		if err = server.processMembershipWebhook(ctx, event, "charge.refunded", refund); err != nil {
			t.Fatal(err)
		}
	}
	if err = store.DB.QueryRowContext(ctx, `SELECT coalesce(sum(amount_cents),0),count(*) FROM cloud_earnings WHERE user_id=$1`, creator.ID).Scan(&amount, &count); err != nil || amount != 400 || count != 2 {
		t.Fatalf("refund %d count%d %v", amount, count, err)
	}
	// A replayed payment cannot resurrect access revoked for the paid period.
	if err = server.processMembershipWebhook(ctx, "late_paid_"+id, "invoice.paid", invoice); err != nil {
		t.Fatal(err)
	}
	var active bool
	if err = store.DB.QueryRowContext(ctx, `SELECT status='active' AND paid_through>now() FROM cloud_memberships WHERE user_id=$1 AND channel_id=$2`, viewer.ID, creator.ChannelID).Scan(&active); err != nil || active {
		t.Fatalf("refunded membership active=%v: %v", active, err)
	}
}

func TestMembershipCommerceRequiresExplicitActivation(t *testing.T) {
	server := NewServer(nil, Config{BillingEnabled: true, StripeSecret: "sk_test", MembershipsEnabled: true})
	if server.membershipSalesEnabled() {
		t.Fatal("unconfigured commission policy enabled sales")
	}
	server.Config.MembershipPolicyConfigured = true
	if !server.membershipSalesEnabled() {
		t.Fatal("explicit zero commission should be supported")
	}
	server.Config.MembershipCommissionBPS = 10001
	if server.membershipSalesEnabled() {
		t.Fatal("invalid commission policy enabled sales")
	}
}
func TestRewardPeriodIsClosedOnceAndCannotChangeInputs(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	var period string
	// Choose an unused historical period without modifying another test's facts.
	for year := 1900; year < 2000; year++ {
		candidate := strconv.Itoa(year) + "-01"
		var exists bool
		if err := store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_reward_periods WHERE period=$1)`, candidate).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			period = candidate
			break
		}
	}
	if period == "" {
		t.Fatal("no test period available")
	}
	t.Cleanup(func() {
		store.DB.ExecContext(ctx, `DELETE FROM cloud_earnings WHERE reference=$1`, "reward:"+period+":"+u.ID)
		store.DB.ExecContext(ctx, `DELETE FROM cloud_reward_periods WHERE period=$1`, period)
	})
	weights := map[string]int64{u.ID: 10}
	for i := 0; i < 2; i++ {
		if err := store.CloseRewardPeriod(ctx, period, 10000, 1000, 500, weights); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.CloseRewardPeriod(ctx, period, 10000, 1000, 600, weights); err == nil {
		t.Fatal("closed period inputs changed")
	}
	var amount, count int64
	if err := store.DB.QueryRowContext(ctx, `SELECT sum(amount_cents),count(*) FROM cloud_earnings WHERE user_id=$1`, u.ID).Scan(&amount, &count); err != nil || amount != 500 || count != 1 {
		t.Fatalf("rewards %d count%d %v", amount, count, err)
	}
}
