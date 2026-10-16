package cloud

import (
	"context"
	"fmt"
	"testing"
)

func TestBillingHistoryOwnedStablePaginationAndProviderFilters(t *testing.T) {
	store, u := testPlatform(t)
	_, other := testPlatform(t)
	for _, owner := range []User{u, other} {
		for i := 0; i < 3; i++ {
			id := fmt.Sprintf("history-%s-%d", owner.ID, i)
			_, err := store.DB.Exec(`INSERT INTO cloud_purchases(id,workspace_id,pack_id,credits,price_cents,currency,idempotency_key)VALUES($1,$2,'starter',1000,1000,'usd',$1)`, id, owner.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
		}
		_, err := store.DB.Exec(`INSERT INTO cloud_crypto_invoices(id,user_id,workspace_id,purpose,credits,price_cents,idempotency_key,reference,network,vm_type,token,token_address,treasury_address,amount,consolidate_amount,amount_net,fee_amount)VALUES($1,$2,$3,'credits',1000,1000,$1,$1,'test-network','EVM','USDC','0xtoken','0xtreasury','10','10','10','0')`, "crypto-history-"+owner.ID, owner.ID, owner.WorkspaceID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.DB.Exec(`INSERT INTO cloud_premium_checkouts(id,user_id,price_cents,idempotency_key)VALUES($1,$2,900,$1)`, "premium-history-"+owner.ID, owner.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer(store, Config{})
	defer server.Close()
	first, err := server.billingHistoryPage(context.Background(), u, "", "all", "all", 2)
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("firstpage %+v %v", first, err)
	}
	second, err := server.billingHistoryPage(context.Background(), u, first.NextCursor, "all", "all", 2)
	if err != nil || len(second.Items) != 2 || second.NextCursor == "" {
		t.Fatalf("secondpage %+v %v", second, err)
	}
	third, err := server.billingHistoryPage(context.Background(), u, second.NextCursor, "all", "all", 2)
	if err != nil || len(third.Items) != 1 || third.NextCursor != "" {
		t.Fatalf("thirdpage %+v %v", third, err)
	}
	seen := map[string]bool{}
	for _, page := range []BillingHistoryPage{first, second, third} {
		for _, entry := range page.Items {
			if seen[entry.ID] {
				t.Fatal("duplicate cursor entry")
			}
			seen[entry.ID] = true
		}
	}
	filtered, err := server.billingHistoryPage(context.Background(), u, "", "stripe", "premium", 20)
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].Product != "premium" {
		t.Fatalf("filter %+v %v", filtered, err)
	}
	if _, err = server.billingHistoryPage(context.Background(), u, "malformed", "all", "all", 20); err == nil {
		t.Fatal("bad cursor accepted")
	}
}

func TestBillingHistoryRejectsInvalidFiltersAndLimit(t *testing.T) {
	s := NewServer(nil, Config{})
	defer s.Close()
	for _, value := range []struct {
		provider, product string
		limit             int
	}{{"unknown", "all", 20}, {"all", "unknown", 20}, {"all", "all", 0}, {"all", "all", 51}} {
		if _, err := s.billingHistoryPage(context.Background(), User{ID: "user", WorkspaceID: "workspace"}, "", value.provider, value.product, value.limit); err == nil {
			t.Fatal("invalid history request accepted")
		}
	}
}
