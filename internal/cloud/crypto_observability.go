package cloud

import (
	"context"
	"fmt"
	"strings"
)

type CryptoPaymentIntegrity struct {
	SettledWithoutReceipt        int64 `json:"settled_without_receipt"`
	ReceiptWithoutSettledInvoice int64 `json:"receipt_without_settled_invoice"`
	MissingCreditGrant           int64 `json:"missing_or_incorrect_credit_grant"`
}
type CryptoPaymentHealth struct {
	StatusCounts          map[string]int64       `json:"status_counts"`
	StaleCounts           map[string]int64       `json:"stale_over_30_minutes"`
	OldestStaleAgeSeconds map[string]int64       `json:"oldest_stale_age_seconds"`
	Integrity             CryptoPaymentIntegrity `json:"integrity"`
}

// CryptoPaymentHealth reads one database snapshot. It contains only aggregate
// operational counters: never user IDs, addresses, provider payloads or secrets.
// Reporting does not contact the gateway or repair/grant any financial state.
func (s *Store) CryptoPaymentHealth(ctx context.Context) (CryptoPaymentHealth, error) {
	result := CryptoPaymentHealth{StatusCounts: map[string]int64{"pending": 0, "processing": 0, "review": 0, "settled": 0}, StaleCounts: map[string]int64{"pending": 0, "processing": 0, "review": 0}, OldestStaleAgeSeconds: map[string]int64{"pending": 0, "processing": 0, "review": 0}}
	rows, err := s.DB.QueryContext(ctx, `WITH counts AS (
 SELECT status,count(*) AS total,count(*) FILTER(WHERE status<>'settled' AND created_at<now()-interval '30 minutes') AS stale,
 coalesce(floor(extract(epoch FROM (now()-min(created_at) FILTER(WHERE status<>'settled' AND created_at<now()-interval '30 minutes')))),0)::bigint AS oldest
 FROM cloud_crypto_invoices GROUP BY status
 ), integrity AS (
 SELECT (SELECT count(*) FROM cloud_crypto_invoices i WHERE i.status='settled' AND NOT EXISTS(SELECT 1 FROM cloud_crypto_settlements r WHERE r.reference=i.reference)) AS no_receipt,
 (SELECT count(*) FROM cloud_crypto_settlements r JOIN cloud_crypto_invoices i ON i.reference=r.reference WHERE i.status<>'settled') AS no_settled,
 (SELECT count(*) FROM cloud_crypto_invoices i WHERE i.status='settled' AND i.purpose='credits' AND NOT EXISTS(SELECT 1 FROM cloud_credit_entries e WHERE e.workspace_id=i.workspace_id AND e.reference='cryptorails:'||i.reference AND e.amount=i.credits AND e.kind='purchase')) AS no_grant
 ) SELECT c.status,c.total,c.stale,c.oldest,i.no_receipt,i.no_settled,i.no_grant FROM integrity i LEFT JOIN counts c ON true`)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var status *string
		var total, stale, oldest *int64
		if err = rows.Scan(&status, &total, &stale, &oldest, &result.Integrity.SettledWithoutReceipt, &result.Integrity.ReceiptWithoutSettledInvoice, &result.Integrity.MissingCreditGrant); err != nil {
			return result, err
		}
		if status != nil {
			result.StatusCounts[*status] = *total
			if *status != "settled" {
				result.StaleCounts[*status] = *stale
				result.OldestStaleAgeSeconds[*status] = *oldest
			}
		}
	}
	return result, rows.Err()
}

// AuditedCryptoPaymentHealth is the private operator CLI entry point. Audit
// insertion must succeed before an otherwise read-only summary is returned.
func (s *Store) AuditedCryptoPaymentHealth(ctx context.Context, actor string) (CryptoPaymentHealth, error) {
	if strings.TrimSpace(actor) == "" {
		return CryptoPaymentHealth{}, ErrValidation
	}
	report, err := s.CryptoPaymentHealth(ctx)
	if err != nil {
		return report, err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'crypto_report','aggregate','Read-only aggregate payment health; no financial state changed')`, actor)
	if err != nil {
		return CryptoPaymentHealth{}, fmt.Errorf("payment report audit failed: %w", err)
	}
	return report, nil
}
