package cloud

import (
	"context"
	"net/url"
	"time"
)

// RewardStripeStatement reads the existing Stripe API through the configured
// client. It never grants credits, recognizes revenue, or approves its own
// evidence. Only available USD processor-balance transactions are accepted.
// CryptoRails' settlement receipt exposes token amounts; it intentionally has
// no automatic fiat adapter because that is not proof of USD conversion.
func (s *Server) RewardStripeStatement(ctx context.Context, purchaseID string) (RewardRevenueEvidence, error) {
	var session string
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT stripe_session FROM cloud_purchases WHERE id=$1 AND status='paid' AND currency='usd'`, purchaseID).Scan(&session); err != nil {
		return RewardRevenueEvidence{}, err
	}
	if s.Config.StripeSecret == "" {
		return RewardRevenueEvidence{}, ErrUnavailable
	}
	var v struct {
		Currency      string `json:"currency"`
		AmountTotal   int64  `json:"amount_total"`
		PaymentStatus string `json:"payment_status"`
		TotalDetails  struct {
			Tax *int64 `json:"amount_tax"`
		} `json:"total_details"`
		PaymentIntent struct {
			LatestCharge struct {
				Paid     bool   `json:"paid"`
				Refunded *int64 `json:"amount_refunded"`
				Balance  struct {
					Amount    int64  `json:"amount"`
					Fee       *int64 `json:"fee"`
					Net       int64  `json:"net"`
					Currency  string `json:"currency"`
					Status    string `json:"status"`
					Available int64  `json:"available_on"`
					ID        string `json:"id"`
				} `json:"balance_transaction"`
			} `json:"latest_charge"`
		} `json:"payment_intent"`
	}
	err := s.stripeForm(ctx, "checkout/sessions/"+url.PathEscape(session)+"?expand[]=payment_intent.latest_charge.balance_transaction", "", nil, &v)
	if err != nil {
		return RewardRevenueEvidence{}, err
	}
	b := v.PaymentIntent.LatestCharge.Balance
	if v.Currency != "usd" || v.PaymentStatus != "paid" || !v.PaymentIntent.LatestCharge.Paid || v.TotalDetails.Tax == nil || v.PaymentIntent.LatestCharge.Refunded == nil || b.Currency != "usd" || b.Status != "available" || b.Fee == nil || b.Amount != v.AmountTotal || b.Net != b.Amount-*b.Fee || b.ID == "" || b.Available <= 0 {
		return RewardRevenueEvidence{}, ErrUnavailable
	}
	e := RewardRevenueEvidence{Kind: "receipt", ResourceID: purchaseID, Provider: "stripe", GrossCents: &v.AmountTotal, FeeCents: b.Fee, TaxCents: v.TotalDetails.Tax, RefundCents: v.PaymentIntent.LatestCharge.Refunded, SettledAt: time.Unix(b.Available, 0).UTC().Format(time.RFC3339), EvidenceReference: "Stripe balance transaction " + b.ID, Reason: "Independent review required: Stripe available USD balance transaction"}
	if !validRevenueEvidence(e) {
		return RewardRevenueEvidence{}, ErrValidation
	}
	return e, nil
}
