package cloud

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRewardAmbiguousTransferReconciliationNeverPostsMoney(t *testing.T) {
	s, u := testPlatform(t)
	ctx := context.Background()
	id := uuid.NewString()
	transfer := "tr_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	destination := "acct_fixture"
	_, e := s.DB.Exec(`INSERT INTO cloud_payout_batches(id,user_id,destination,country,amount_cents,currency,status,first_attempt_at)VALUES($1,$2,$3,'US',6000,'usd','submitting',now()-interval '2 days')`, id, u.ID, destination)
	if e != nil {
		t.Fatal(e)
	}
	app := NewServer(s, Config{StripeSecret: "test"})
	defer app.Close()
	calls := 0
	wrong := true
	partial := false
	app.HTTP = &http.Client{Transport: adminTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/v1/transfers/"+transfer {
			t.Errorf("unexpected provider request %s %s", r.Method, r.URL)
		}
		amount := 6000
		if wrong {
			amount = 5999
		}
		reversed := 0
		if partial {
			reversed = 1
		}
		body := fmt.Sprintf(`{"id":%q,"amount_reversed":%d,"amount":%d,"currency":"usd","destination":%q,"metadata":{"batch_id":%q}}`, transfer, reversed, amount, destination, id)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	if e = app.ReconcileRewardTransfer(ctx, id, transfer, "finance", "verify ambiguous transfer"); e == nil {
		t.Fatal("mismatched amount accepted")
	}
	wrong = false
	partial = true
	if e = app.ReconcileRewardTransfer(ctx, id, transfer, "finance", "partial reversal"); e == nil {
		t.Fatal("partially reversed transfer accepted")
	}
	partial = false
	if e = app.ReconcileRewardTransfer(ctx, id, transfer, "finance", "verify actual transfer"); e != nil {
		t.Fatal(e)
	}
	if e = app.ReconcileRewardTransfer(ctx, id, transfer, "finance", "retry actual transfer"); e != nil {
		t.Fatal(e)
	}
	if calls != 3 {
		t.Fatalf("calls %d", calls)
	}
	var status string
	if e = s.DB.QueryRow(`SELECT status FROM cloud_payout_batches WHERE id=$1`, id).Scan(&status); e != nil || status != "paid" {
		t.Fatalf("%s %v", status, e)
	}
}
