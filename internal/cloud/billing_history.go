package cloud

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type BillingHistoryEntry struct {
	ID               string     `json:"id"`
	Provider         string     `json:"provider"`
	Product          string     `json:"product"`
	AmountCents      int64      `json:"amount_cents"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	PaidAt           *time.Time `json:"paid_at"`
	ManagementURL    string     `json:"management_url"`
	ReceiptAvailable bool       `json:"receipt_available"`
}
type BillingHistoryPage struct {
	Items      []BillingHistoryEntry `json:"items"`
	NextCursor string                `json:"next_cursor"`
}
type billingCursor struct {
	Time time.Time `json:"time"`
	ID   string    `json:"id"`
}

func (s *Server) billingHistory(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			s.fail(w, ErrValidation)
			return
		}
		limit = n
	}
	provider, product := r.URL.Query().Get("provider"), r.URL.Query().Get("product")
	if provider == "" {
		provider = "all"
	}
	if product == "" {
		product = "all"
	}
	result, err := s.billingHistoryPage(r.Context(), currentUser(r), r.URL.Query().Get("cursor"), provider, product, limit)
	s.result(w, result, err)
}
func (s *Server) billingHistoryPage(ctx context.Context, u User, cursor, provider, product string, limit int) (BillingHistoryPage, error) {
	out := BillingHistoryPage{Items: []BillingHistoryEntry{}}
	if u.ID == "" || u.WorkspaceID == "" || limit < 1 || limit > 50 || !hasValue([]string{"all", "stripe", "cryptorails"}, provider) || !hasValue([]string{"all", "credits", "premium"}, product) {
		return out, ErrValidation
	}
	cut := billingCursor{Time: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), ID: "~"}
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(raw) > 512 || json.Unmarshal(raw, &cut) != nil || cut.Time.IsZero() || cut.ID == "" || len(cut.ID) > 200 {
			return out, ErrValidation
		}
	}
	rows, err := s.Store.DB.QueryContext(ctx, `WITH history AS (
 SELECT 'stripe-credit:'||p.id AS id,'stripe'::text AS provider,'credits'::text AS product,p.price_cents AS amount_cents,p.currency,p.status,p.created_at,(SELECT created_at FROM cloud_credit_entries e WHERE e.workspace_id=p.workspace_id AND e.reference='purchase:'||p.id) AS paid_at,'/credits'::text AS management_url FROM cloud_purchases p WHERE p.workspace_id=$1
 UNION ALL
 SELECT 'crypto:'||i.id,'cryptorails',i.purpose,i.price_cents,'usd',i.status,i.created_at,(SELECT created_at FROM cloud_crypto_settlements st WHERE st.reference=i.reference),'/payments?invoice='||i.id FROM cloud_crypto_invoices i WHERE i.user_id=$2
 UNION ALL
 SELECT 'stripe-premium-invoice:'||i.id,'stripe','premium',i.gross_cents,i.currency,CASE WHEN i.refunded_cents>=i.gross_cents THEN 'refunded' WHEN i.refunded_cents>0 THEN 'partially_refunded' ELSE 'paid' END,i.created_at,i.created_at,'/premium' FROM cloud_premium_invoices i JOIN cloud_premium_checkouts c ON c.id=i.checkout_id WHERE c.user_id=$2
 UNION ALL
 SELECT 'stripe-premium-checkout:'||c.id,'stripe','premium',c.price_cents,c.currency,'pending',c.created_at,NULL::timestamptz,'/premium' FROM cloud_premium_checkouts c WHERE c.user_id=$2 AND NOT EXISTS(SELECT 1 FROM cloud_premium_invoices i WHERE i.checkout_id=c.id)
 ) SELECT id,provider,product,amount_cents,currency,status,created_at,paid_at,management_url FROM history WHERE ($3='all' OR provider=$3) AND ($4='all' OR product=$4) AND (created_at,id)<($5,$6) ORDER BY created_at DESC,id DESC LIMIT $7`, u.WorkspaceID, u.ID, provider, product, cut.Time, cut.ID, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry BillingHistoryEntry
		var paid sql.NullTime
		if err = rows.Scan(&entry.ID, &entry.Provider, &entry.Product, &entry.AmountCents, &entry.Currency, &entry.Status, &entry.CreatedAt, &paid, &entry.ManagementURL); err != nil {
			return out, err
		}
		if paid.Valid {
			entry.PaidAt = &paid.Time
		}
		if entry.Provider == "cryptorails" {
			entry.ManagementURL = "/payments?invoice=" + url.QueryEscape(entry.ID[len("crypto:"):])
		}
		out.Items = append(out.Items, entry)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[len(out.Items)-1]
		raw, _ := json.Marshal(billingCursor{Time: last.CreatedAt, ID: last.ID})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return out, nil
}
