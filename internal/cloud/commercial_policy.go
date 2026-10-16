package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"math/big"
	"regexp"
	"strings"
	"time"
)

type CommercialPolicy struct {
	Packs                []CreditPack `json:"packs"`
	TargetGrossMarginBPS int64        `json:"target_gross_margin_bps"`
	FeeReserveBPS        int64        `json:"fee_reserve_bps"`
	MediaReserveBPS      int64        `json:"media_reserve_bps"`
}
type CommercialPolicyRecord struct {
	Policy    CommercialPolicy `json:"policy"`
	Revision  int64            `json:"revision"`
	UpdatedAt *time.Time       `json:"updated_at"`
	Source    string           `json:"source"`
}

func DefaultCommercialPolicy() CommercialPolicy {
	return CommercialPolicy{Packs: append([]CreditPack(nil), Packs...), TargetGrossMarginBPS: 2000}
}

var commercialPackID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func validateCommercialPolicy(p CommercialPolicy) error {
	if len(p.Packs) < 1 || len(p.Packs) > 20 || p.TargetGrossMarginBPS < 1000 || p.TargetGrossMarginBPS > 9000 || p.FeeReserveBPS < 0 || p.FeeReserveBPS > 5000 || p.MediaReserveBPS < 0 || p.MediaReserveBPS > 5000 || p.TargetGrossMarginBPS+p.FeeReserveBPS+p.MediaReserveBPS >= 10000 {
		return ErrValidation
	}
	seen := map[string]bool{}
	for _, pack := range p.Packs {
		if !commercialPackID.MatchString(pack.ID) || seen[pack.ID] || strings.TrimSpace(pack.Name) == "" || len(pack.Name) > 100 || pack.Credits < 1 || pack.Credits > 100000000 || pack.PriceCents < 100 || pack.PriceCents > 10000000 || pack.Currency != "usd" {
			return ErrValidation
		}
		seen[pack.ID] = true
	}
	return nil
}
func minimumUnitRevenue(p CommercialPolicy) *big.Rat {
	var minimum *big.Rat
	for _, pack := range p.Packs {
		unit := big.NewRat(pack.PriceCents*10000, pack.Credits)
		if minimum == nil || unit.Cmp(minimum) < 0 {
			minimum = unit
		}
	}
	return minimum
}
func validateCommercialMargin(runtime RuntimePolicy, p CommercialPolicy) error {
	if validateCommercialPolicy(p) != nil {
		return ErrValidation
	}
	unit := minimumUnitRevenue(p)
	available := 10000 - p.TargetGrossMarginBPS - p.FeeReserveBPS - p.MediaReserveBPS
	enough := func(credits, cost int64) bool {
		revenue := new(big.Rat).Mul(unit, big.NewRat(credits, 1))
		budget := new(big.Rat).Mul(revenue, big.NewRat(available, 10000))
		return budget.Cmp(big.NewRat(cost, 1)) >= 0
	}
	if !enough(runtime.StoryCredits, runtime.StoryMaxCostMicros) || !enough(runtime.CreditsPerSecond*5, runtime.ProviderMaxCostMicros) {
		return fmt.Errorf("%w: cheapest pack does not cover provider ceilings, reserves and target gross margin", ErrValidation)
	}
	return nil
}

type commercialQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func commercialPolicyRead(ctx context.Context, q commercialQuery) (CommercialPolicyRecord, error) {
	out := CommercialPolicyRecord{Policy: DefaultCommercialPolicy(), Source: "deployment bootstrap"}
	var raw []byte
	var at time.Time
	e := q.QueryRowContext(ctx, `SELECT policy,revision,updated_at FROM cloud_commercial_policy WHERE id=true`).Scan(&raw, &out.Revision, &at)
	if e == sql.ErrNoRows {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	if json.Unmarshal(raw, &out.Policy) != nil || validateCommercialPolicy(out.Policy) != nil {
		return out, ErrValidation
	}
	out.UpdatedAt = &at
	out.Source = "independently approved commercial policy"
	return out, nil
}
func (s *Store) CommercialPolicy(ctx context.Context) (CommercialPolicyRecord, error) {
	return commercialPolicyRead(ctx, s.DB)
}
func (s *Store) CreditPacks(ctx context.Context) ([]CreditPack, error) {
	r, e := s.CommercialPolicy(ctx)
	return r.Policy.Packs, e
}
func findCommercialPack(p CommercialPolicy, id string) (CreditPack, error) {
	for _, pack := range p.Packs {
		if pack.ID == id {
			return pack, nil
		}
	}
	return CreditPack{}, ErrValidation
}
func (s *Store) ProposeCommercialPolicy(ctx context.Context, p CommercialPolicy, revision int64, reason, actor string) (string, error) {
	if validateCommercialPolicy(p) != nil || revision < 0 || actor == "" || len(strings.TrimSpace(reason)) < 3 || len(reason) > 1000 {
		return "", ErrValidation
	}
	raw, _ := json.Marshal(p)
	id := uuid.NewString()
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	r, e := commercialPolicyRead(ctx, tx)
	if e != nil {
		return "", e
	}
	if r.Revision != revision {
		return "", ErrConflict
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_commercial_proposals(id,base_revision,policy,reason,proposed_by)VALUES($1,$2,$3,$4,$5)`, id, revision, raw, reason, actor); e != nil {
		return "", e
	}
	if e = supportAudit(ctx, tx, actor, "propose_commercial_policy", id, reason); e != nil {
		return "", e
	}
	return id, tx.Commit()
}
func (s *Store) ApproveCommercialPolicy(ctx context.Context, id, actor, reason string, base Config) error {
	if actor == "" || len(strings.TrimSpace(reason)) < 3 || len(reason) > 1000 {
		return ErrValidation
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	// Same lock as runtime edits: independently safe edits cannot combine into an unsafe price/cost pair.
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(814037238)`); e != nil {
		return e
	}
	var raw []byte
	var revision int64
	var proposer string
	var approved sql.NullString
	if e = tx.QueryRowContext(ctx, `SELECT policy,base_revision,proposed_by,approved_by FROM cloud_commercial_proposals WHERE id=$1 FOR UPDATE`, id).Scan(&raw, &revision, &proposer, &approved); e != nil {
		return e
	}
	if proposer == actor {
		return ErrUnauthorized
	}
	if approved.Valid {
		if approved.String != actor {
			return ErrConflict
		}
		return tx.Commit()
	}
	current, e := commercialPolicyRead(ctx, tx)
	if e != nil {
		return e
	}
	if current.Revision != revision {
		return ErrConflict
	}
	var p CommercialPolicy
	if json.Unmarshal(raw, &p) != nil {
		return ErrValidation
	}
	runtime := runtimePolicyFromConfig(base)
	var runtimeRaw []byte
	e = tx.QueryRowContext(ctx, `SELECT policy FROM cloud_runtime_policy WHERE id=true FOR SHARE`).Scan(&runtimeRaw)
	if e != nil && e != sql.ErrNoRows {
		return e
	}
	if e == nil && json.Unmarshal(runtimeRaw, &runtime) != nil {
		return ErrValidation
	}
	if validateRuntimePolicyStructure(runtime) != nil {
		return ErrValidation
	}
	if e = validateCommercialMargin(runtime, p); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_commercial_policy(id,revision,policy,updated_by)VALUES(true,$1,$2,$3) ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,policy=excluded.policy,updated_by=excluded.updated_by,updated_at=now()`, revision+1, raw, actor); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE cloud_commercial_proposals SET approved_by=$2,approved_at=now(),approval_reason=$3 WHERE id=$1`, id, actor, reason); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_commercial_history(revision,proposal_id,policy,proposed_by,approved_by,reason,approval_reason)SELECT $2,id,policy,proposed_by,$3,reason,$4 FROM cloud_commercial_proposals WHERE id=$1`, id, revision+1, actor, reason); e != nil {
		return e
	}
	if e = supportAudit(ctx, tx, actor, "approve_commercial_policy", id, reason); e != nil {
		return e
	}
	return tx.Commit()
}

type CommercialPreview struct {
	MinimumUnitRevenueMicros float64 `json:"minimum_unit_revenue_micros"`
	WorstPackGrossMarginBPS  int64   `json:"worst_pack_gross_margin_bps"`
	RequiredGrossMarginBPS   int64   `json:"required_gross_margin_bps"`
	MarginOK                 bool    `json:"margin_ok"`
	Basis                    string  `json:"basis"`
}

func commercialRuntime(ctx context.Context, q commercialQuery, base Config) (RuntimePolicy, error) {
	p := runtimePolicyFromConfig(base)
	var raw []byte
	e := q.QueryRowContext(ctx, `SELECT policy FROM cloud_runtime_policy WHERE id=true`).Scan(&raw)
	if e != nil && e != sql.ErrNoRows {
		return p, e
	}
	if e == nil && json.Unmarshal(raw, &p) != nil {
		return p, ErrValidation
	}
	return p, validateRuntimePolicyStructure(p)
}
func commercialPreview(p CommercialPolicy, runtime RuntimePolicy) (CommercialPreview, error) {
	out := CommercialPreview{RequiredGrossMarginBPS: p.TargetGrossMarginBPS, Basis: "Current lowest USD pack unit revenue; five-second video and story provider ceilings, fee and media reserves. Existing purchased credits retain their original sale price."}
	if validateCommercialPolicy(p) != nil {
		return out, ErrValidation
	}
	unit := minimumUnitRevenue(p)
	out.MinimumUnitRevenueMicros, _ = unit.Float64()
	worst := int64(10000)
	for _, service := range [][2]int64{{runtime.StoryCredits, runtime.StoryMaxCostMicros}, {runtime.CreditsPerSecond * 5, runtime.ProviderMaxCostMicros}} {
		if service[0] < 1 {
			return out, ErrValidation
		}
		revenue := new(big.Rat).Mul(unit, big.NewRat(service[0], 1))
		costShare := new(big.Rat).Quo(big.NewRat(service[1]*10000, 1), revenue)
		costCeil := new(big.Int).Quo(new(big.Int).Add(costShare.Num(), new(big.Int).Sub(costShare.Denom(), big.NewInt(1))), costShare.Denom()).Int64()
		margin := 10000 - costCeil - p.FeeReserveBPS - p.MediaReserveBPS
		if margin < worst {
			worst = margin
		}
	}
	out.WorstPackGrossMarginBPS = worst
	out.MarginOK = validateCommercialMargin(runtime, p) == nil
	return out, nil
}
