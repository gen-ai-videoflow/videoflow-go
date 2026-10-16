package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type RuntimePolicy struct {
	StoryModels               []string `json:"story_models"`
	VideoModels               []string `json:"video_models"`
	CreditsPerSecond          int64    `json:"credits_per_second"`
	StoryCredits              int64    `json:"story_credits"`
	AssemblyCredits           int64    `json:"assembly_credits"`
	ProviderDailyBudgetMicros int64    `json:"provider_daily_budget_micros"`
	ProviderMaxCostMicros     int64    `json:"provider_max_cost_micros"`
	StoryMaxCostMicros        int64    `json:"story_max_cost_micros"`
	PremiumPriceCents         int64    `json:"premium_price_cents"`
	MaxActiveJobs             int      `json:"max_active_jobs"`
}
type RuntimePolicyRecord struct {
	Policy    RuntimePolicy `json:"policy"`
	Revision  int64         `json:"revision"`
	UpdatedAt *time.Time    `json:"updated_at"`
	Source    string        `json:"source"`
}

func modelList(value string) []string {
	out := []string{}
	for _, model := range strings.Split(value, ",") {
		if model = strings.TrimSpace(model); model != "" {
			out = append(out, model)
		}
	}
	return out
}
func runtimePolicyFromConfig(c Config) RuntimePolicy {
	return RuntimePolicy{StoryModels: modelList(c.StoryModel), VideoModels: modelList(c.VideoModel), CreditsPerSecond: c.CreditsPerSecond, StoryCredits: c.StoryCredits, AssemblyCredits: c.AssemblyCredits, ProviderDailyBudgetMicros: c.ProviderDailyBudgetMicros, ProviderMaxCostMicros: c.ProviderMaxCostMicros, StoryMaxCostMicros: c.StoryMaxCostMicros, PremiumPriceCents: c.PremiumPriceCents, MaxActiveJobs: 10}
}

var runtimeModelID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{1,159}$`)

func validateRuntimePolicy(p RuntimePolicy) error {
	if p.CreditsPerSecond < 1 || p.CreditsPerSecond > 10000 || p.StoryCredits < 1 || p.StoryCredits > 1000000 || p.AssemblyCredits < 1 || p.AssemblyCredits > 1000000 || p.ProviderDailyBudgetMicros < 0 || p.ProviderDailyBudgetMicros > 1000000000 || p.ProviderMaxCostMicros < 0 || p.ProviderMaxCostMicros > 100000000 || p.StoryMaxCostMicros < 0 || p.StoryMaxCostMicros > 10000000 || p.PremiumPriceCents < 0 || p.PremiumPriceCents > 100000 || (p.PremiumPriceCents > 0 && p.PremiumPriceCents < 100) || p.MaxActiveJobs < 1 || p.MaxActiveJobs > 20 {
		return ErrValidation
	}
	for _, models := range [][]string{p.StoryModels, p.VideoModels} {
		if len(models) > 20 {
			return ErrValidation
		}
		seen := map[string]bool{}
		for _, model := range models {
			if !runtimeModelID.MatchString(model) || strings.Contains(model, ":free") || seen[model] {
				return ErrValidation
			}
			seen[model] = true
		}
	}
	if len(p.VideoModels) > 0 && (p.ProviderMaxCostMicros < 1 || p.ProviderDailyBudgetMicros < p.ProviderMaxCostMicros) {
		return ErrValidation
	}
	if len(p.StoryModels) > 0 && (p.StoryMaxCostMicros < 1 || p.ProviderDailyBudgetMicros < p.StoryMaxCostMicros || p.ProviderMaxCostMicros <= p.StoryMaxCostMicros) {
		return ErrValidation
	}
	// Fixed credit packs define the lowest unit revenue. Require a minimum 20%
	// revenue/cost buffer against approved provider ceilings for newly sold work.
	unit := new(big.Rat)
	for i, pack := range Packs {
		value := big.NewRat(pack.PriceCents*10000, pack.Credits)
		if i == 0 || value.Cmp(unit) < 0 {
			unit = value
		}
	}
	enough := func(credits, cost int64) bool {
		return new(big.Rat).Mul(unit, big.NewRat(credits, 1)).Cmp(big.NewRat(cost*120, 100)) >= 0
	}
	if !enough(p.StoryCredits, p.StoryMaxCostMicros) || !enough(p.CreditsPerSecond*5, p.ProviderMaxCostMicros) {
		return fmt.Errorf("%w: credit prices need at least a 20%% provider-cost buffer", ErrValidation)
	}
	return nil
}
func (s *Store) runtimePolicy(ctx context.Context, base Config) (RuntimePolicyRecord, error) {
	record := RuntimePolicyRecord{Policy: runtimePolicyFromConfig(base), Source: "deployment bootstrap"}
	var raw []byte
	var updated time.Time
	err := s.DB.QueryRowContext(ctx, `SELECT policy,revision,updated_at FROM cloud_runtime_policy WHERE id=true`).Scan(&raw, &record.Revision, &updated)
	if err == sql.ErrNoRows {
		return record, nil
	}
	if err != nil {
		return record, err
	}
	if json.Unmarshal(raw, &record.Policy) != nil || validateRuntimePolicy(record.Policy) != nil {
		return record, ErrValidation
	}
	record.UpdatedAt = &updated
	record.Source = "versioned runtime policy"
	return record, nil
}
func (s *Store) EffectiveConfig(ctx context.Context, base Config) (Config, error) {
	record, err := s.runtimePolicy(ctx, base)
	if err != nil {
		return base, err
	}
	if record.Revision == 0 {
		return base, nil
	}
	p := record.Policy
	base.StoryModel = strings.Join(p.StoryModels, ",")
	base.VideoModel = strings.Join(p.VideoModels, ",")
	base.CreditsPerSecond = p.CreditsPerSecond
	base.StoryCredits = p.StoryCredits
	base.AssemblyCredits = p.AssemblyCredits
	base.ProviderDailyBudgetMicros = p.ProviderDailyBudgetMicros
	base.ProviderMaxCostMicros = p.ProviderMaxCostMicros
	base.StoryMaxCostMicros = p.StoryMaxCostMicros
	base.PremiumPriceCents = p.PremiumPriceCents
	return base, nil
}
func (s *Server) effectiveConfig(ctx context.Context) (Config, error) {
	if s.Store == nil {
		return s.Config, nil
	}
	return s.Store.EffectiveConfig(ctx, s.Config)
}
func (s *Store) saveRuntimePolicy(ctx context.Context, actor string, revision int64, policy RuntimePolicy, reason string) (RuntimePolicyRecord, error) {
	var out RuntimePolicyRecord
	if validateRuntimePolicy(policy) != nil || revision < 0 || len(strings.TrimSpace(reason)) < 3 || len(reason) > 1000 {
		return out, ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(814037238)`); err != nil {
		return out, err
	}
	var current int64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM cloud_runtime_policy WHERE id=true FOR UPDATE`).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return out, err
	}
	if current != revision {
		return out, ErrConflict
	}
	raw, _ := json.Marshal(policy)
	var updated time.Time
	err = tx.QueryRowContext(ctx, `INSERT INTO cloud_runtime_policy(id,policy,revision,updated_by)VALUES(true,$1,1,$2)ON CONFLICT(id)DO UPDATE SET policy=excluded.policy,revision=cloud_runtime_policy.revision+1,updated_by=excluded.updated_by,updated_at=now() RETURNING revision,updated_at`, raw, actor).Scan(&out.Revision, &updated)
	if err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_runtime_policy_versions(revision,policy,updated_by,reason)VALUES($1,$2,$3,$4)`, out.Revision, raw, actor, reason); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'runtime_policy',$2,$3)`, actor, fmt.Sprint(out.Revision), reason); err != nil {
		return out, err
	}
	out.Policy = policy
	out.UpdatedAt = &updated
	out.Source = "versioned runtime policy"
	return out, tx.Commit()
}
func (s *Store) validateRuntimeModels(ctx context.Context, p RuntimePolicy) error {
	if len(p.StoryModels) > 0 {
		var raw []byte
		if err := s.DB.QueryRowContext(ctx, `SELECT catalog FROM cloud_story_catalog WHERE refreshed_at>now()-interval '1 hour'`).Scan(&raw); err != nil {
			return fmt.Errorf("%w: refresh the reviewed story catalogue before approving models", ErrValidation)
		}
		var models []StoryCatalogModel
		if json.Unmarshal(raw, &models) != nil {
			return ErrValidation
		}
		for _, model := range p.StoryModels {
			if _, err := cheapestStoryModel(models, model, strings.Repeat("x", 8000), p.StoryMaxCostMicros); err != nil {
				return fmt.Errorf("%w: unknown or ineligible story model %s", ErrValidation, model)
			}
		}
	}
	if len(p.VideoModels) > 0 {
		var raw []byte
		if err := s.DB.QueryRowContext(ctx, `SELECT catalog FROM cloud_video_catalog WHERE refreshed_at>now()-interval '1 day'`).Scan(&raw); err != nil {
			return ErrValidation
		}
		var models []VideoCatalogModel
		if json.Unmarshal(raw, &models) != nil {
			return ErrValidation
		}
		for _, id := range p.VideoModels {
			valid := false
			for _, model := range models {
				if model.ID != id {
					continue
				}
				for _, duration := range model.Durations {
					for _, aspect := range model.AspectRatios {
						for _, resolution := range model.Resolutions {
							cost, err := videoPriceMicros(model, duration, aspect, resolution)
							if err == nil && cost <= p.ProviderMaxCostMicros {
								valid = true
							}
						}
					}
				}
			}
			if !valid {
				return fmt.Errorf("%w: unknown or ineligible video model %s", ErrValidation, id)
			}
		}
	}
	return nil
}
func (s *Server) adminRuntimePolicy(w http.ResponseWriter, r *http.Request) {
	result, err := s.Store.runtimePolicy(r.Context(), s.Config)
	s.result(w, result, err)
}
func (s *Server) adminUpdateRuntimePolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Policy   RuntimePolicy `json:"policy"`
		Revision int64         `json:"revision"`
		Reason   string        `json:"reason"`
	}
	if !s.decode(w, r, &body) {
		return
	}
	if err := validateRuntimePolicy(body.Policy); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.validateRuntimeModels(r.Context(), body.Policy); err != nil {
		s.fail(w, err)
		return
	}
	result, err := s.Store.saveRuntimePolicy(r.Context(), currentUser(r).ID, body.Revision, body.Policy, body.Reason)
	s.result(w, result, err)
}

// Refreshes public model metadata only. Never submits inference or enables a model.
func (s *Server) adminRefreshRuntimeCatalogues(w http.ResponseWriter, r *http.Request) {
	if s.Config.OpenRouterKey == "" {
		s.fail(w, ErrUnavailable)
		return
	}
	_, _ = s.selectStoryModelWithConfig(r.Context(), "catalogue readiness", s.Config)
	var storyFresh bool
	if err := s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_story_catalog WHERE refreshed_at>now()-interval '1 hour')`).Scan(&storyFresh); err != nil {
		s.fail(w, err)
		return
	}
	if !storyFresh {
		s.fail(w, ErrUnavailable)
		return
	}
	if err := s.Store.RefreshVideoCatalog(r.Context()); err != nil {
		s.fail(w, err)
		return
	}
	s.json(w, 200, map[string]bool{"catalogues_refreshed": true, "inference_submitted": false})
}

func (s *Server) adminRuntimePolicyVersions(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.DB.QueryContext(r.Context(), `SELECT revision,policy,updated_by,reason,created_at FROM cloud_runtime_policy_versions ORDER BY revision DESC LIMIT 20`)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer rows.Close()
	type entry struct {
		Revision  int64         `json:"revision"`
		Policy    RuntimePolicy `json:"policy"`
		UpdatedBy string        `json:"updated_by"`
		Reason    string        `json:"reason"`
		CreatedAt time.Time     `json:"created_at"`
	}
	entries := []entry{}
	for rows.Next() {
		var value entry
		var raw []byte
		if err = rows.Scan(&value.Revision, &raw, &value.UpdatedBy, &value.Reason, &value.CreatedAt); err != nil {
			s.fail(w, err)
			return
		}
		if json.Unmarshal(raw, &value.Policy) != nil {
			s.fail(w, ErrValidation)
			return
		}
		entries = append(entries, value)
	}
	s.result(w, map[string]any{"items": entries, "limit": 20}, rows.Err())
}
