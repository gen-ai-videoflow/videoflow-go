package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
)

var ErrProviderBudget = errors.New("provider spending budget unavailable")

type VideoPolicy struct {
	Model                 string    `json:"model"`
	CreditsPerSecond      int64     `json:"credits_per_second"`
	ProviderMaxCostMicros int64     `json:"provider_max_cost_micros"`
	Version               string    `json:"version"`
	ExpiresAt             time.Time `json:"expires_at"`
	Resolution            string    `json:"resolution"`
}
type VideoCatalogModel struct {
	ID           string            `json:"id"`
	Durations    []int             `json:"supported_durations"`
	Resolutions  []string          `json:"supported_resolutions"`
	AspectRatios []string          `json:"supported_aspect_ratios"`
	PricingSKUs  map[string]string `json:"pricing_skus"`
}

func hasValue[T comparable](values []T, v T) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

// Only known per-second SKUs are understood. Additional audio/input pricing
// needs a reviewed implementation before it can be selected automatically.
func videoPriceMicros(m VideoCatalogModel, duration int, aspect, resolution string) (int64, error) {
	if duration < 1 || !hasValue(m.Durations, duration) || !hasValue(m.AspectRatios, aspect) || !hasValue(m.Resolutions, resolution) {
		return 0, ErrValidation
	}

	// Catalog SKUs have changed naming over time. Normalize explicitly supported
	// per-second families and reject all other output/audio billing units.
	families := []struct {
		base, separator string
		divisor         int64
	}{
		{"per-video-second", "-", 1},
		{"duration_seconds", "_", 1},
		{"cents_per_video_output_second", "_", 100},
		{"cents_per_second_output", "_", 100},
	}
	for sku := range m.PricingSKUs {
		if sku == "cents_per_image_input" || sku == "reference_images" || sku == "reference_input_images" {
			continue
		} // No images are sent by text-to-video.
		known := false
		for _, family := range families {
			if sku == family.base {
				known = true
				break
			}
			prefix := family.base + family.separator
			if strings.HasPrefix(sku, prefix) {
				suffix := strings.TrimPrefix(sku, prefix)
				for _, r := range m.Resolutions {
					if strings.EqualFold(r, suffix) {
						known = true
						break
					}
				}
				if known {
					break
				}
			}
		}
		if !known {
			return 0, ErrValidation
		}
	}
	var rate *big.Rat
	for _, family := range families {
		price := m.PricingSKUs[family.base]
		for sku, value := range m.PricingSKUs {
			if strings.EqualFold(sku, family.base+family.separator+resolution) {
				price = value
				break
			}
		}
		if price == "" {
			continue
		}
		// Prices are decimal money amounts, never fractions, exponents, or signs.
		valid := true
		dots := 0
		digits := 0
		for _, ch := range price {
			if ch == '.' {
				dots++
				if dots > 1 {
					valid = false
				}
			} else if ch < '0' || ch > '9' {
				valid = false
			} else {
				digits++
			}
		}
		if !valid || digits == 0 {
			return 0, ErrValidation
		}
		candidate, ok := new(big.Rat).SetString(price)
		if !ok || candidate.Sign() <= 0 {
			return 0, ErrValidation
		}
		candidate.Quo(candidate, big.NewRat(family.divisor, 1))
		if rate != nil && rate.Cmp(candidate) != 0 {
			return 0, ErrValidation
		}
		rate = candidate
	}
	if rate == nil {
		return 0, ErrValidation
	}
	cost := new(big.Rat).Mul(rate, big.NewRat(int64(duration)*1000000, 1))
	n := new(big.Int).Quo(cost.Num(), cost.Denom())
	if new(big.Int).Mod(cost.Num(), cost.Denom()).Sign() > 0 {
		n.Add(n, big.NewInt(1))
	}
	if !n.IsInt64() || n.Sign() <= 0 {
		return 0, ErrValidation
	}
	return n.Int64(), nil
}

// RefreshVideoCatalog performs at most one successful refresh per day across instances.
func (s *Store) RefreshVideoCatalog(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(814037220)`); err != nil {
		return err
	}
	var fresh bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_video_catalog WHERE refreshed_at>now()-interval '1 day')`).Scan(&fresh); err != nil {
		return err
	}
	if fresh {
		return tx.Commit()
	}
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, "GET", "https://openrouter.ai/api/v1/videos/models", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("catalog redirect rejected") }}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errors.New("video catalog unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil {
		return err
	}
	if len(body) > 4<<20 {
		return errors.New("video catalog too large")
	}
	var envelope struct {
		Data []VideoCatalogModel `json:"data"`
	}
	if err = json.Unmarshal(body, &envelope); err != nil {
		return err
	}
	if len(envelope.Data) == 0 {
		return errors.New("video catalog empty")
	}
	data, err := json.Marshal(envelope.Data)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO cloud_video_catalog(id,catalog,refreshed_at)VALUES(true,$1,now())ON CONFLICT(id)DO UPDATE SET catalog=excluded.catalog,refreshed_at=excluded.refreshed_at`, string(data))
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ConfigureVideoPolicy records explicit operator approval of a commercially reviewed model.
func (s *Store) ConfigureVideoPolicy(ctx context.Context, model string, rate, maxCostMicros int64) error {
	if model == "" || strings.Contains(model, ":free") || rate < 1 || rate > 10000 || maxCostMicros < 1 {
		return ErrValidation
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_video_policies(model,credits_per_second,max_cost_micros,version)VALUES($1,$2,$3,$4)ON CONFLICT(model)DO UPDATE SET credits_per_second=excluded.credits_per_second,max_cost_micros=excluded.max_cost_micros,version=CASE WHEN cloud_video_policies.credits_per_second=excluded.credits_per_second AND cloud_video_policies.max_cost_micros=excluded.max_cost_micros THEN cloud_video_policies.version ELSE excluded.version END,enabled=true`, model, rate, maxCostMicros, uuid.NewString())
	return err
}
func (s *Store) ActiveVideoPolicy(ctx context.Context, duration int, aspect string, approved ...string) (VideoPolicy, error) {
	var catalog []byte
	var refreshed time.Time
	err := s.DB.QueryRowContext(ctx, `SELECT catalog,refreshed_at FROM cloud_video_catalog WHERE refreshed_at>now()-interval '1 day'`).Scan(&catalog, &refreshed)
	if errors.Is(err, sql.ErrNoRows) {
		return VideoPolicy{}, ErrNotFound
	}
	if err != nil {
		return VideoPolicy{}, err
	}
	var models []VideoCatalogModel
	if err = json.Unmarshal(catalog, &models); err != nil {
		return VideoPolicy{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT model,credits_per_second,max_cost_micros,version FROM cloud_video_policies WHERE enabled ORDER BY model`)
	if err != nil {
		return VideoPolicy{}, err
	}
	defer rows.Close()
	var best VideoPolicy
	var bestCost int64
	for rows.Next() {
		var candidate VideoPolicy
		var cap int64
		if err = rows.Scan(&candidate.Model, &candidate.CreditsPerSecond, &cap, &candidate.Version); err != nil {
			return VideoPolicy{}, err
		}
		if len(approved) > 0 && !hasValue(approved, candidate.Model) {
			continue
		}
		if strings.Contains(candidate.Model, ":free") {
			continue
		}
		for _, model := range models {
			if model.ID != candidate.Model {
				continue
			}
			cost, e := videoPriceMicros(model, duration, aspect, "720p")
			if e != nil || cost > cap {
				continue
			}
			if bestCost == 0 || cost < bestCost {
				candidate.ProviderMaxCostMicros = cap
				candidate.Resolution = "720p"
				candidate.ExpiresAt = refreshed.Add(24 * time.Hour)
				best = candidate
				bestCost = cost
			}
		}
	}
	if err = rows.Err(); err != nil {
		return VideoPolicy{}, err
	}
	if bestCost == 0 {
		return VideoPolicy{}, ErrNotFound
	}
	return best, nil
}

// TryProviderBudget reserves worst-case funded spend once per job. Ambiguous or
// failed attempts retain this admission; refunds never reopen provider spend.
func (s *Store) TryProviderBudget(ctx context.Context, jobID string, maxCostMicros, dailyLimitMicros int64) error {
	if maxCostMicros <= 0 || dailyLimitMicros <= 0 || maxCostMicros > dailyLimitMicros {
		return ErrProviderBudget
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lockedJob string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM cloud_jobs WHERE id=$1 FOR UPDATE`, jobID).Scan(&lockedJob); err != nil {
		return err
	}
	var day time.Time
	if err = tx.QueryRowContext(ctx, `SELECT (now() AT TIME ZONE 'UTC')::date`).Scan(&day); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_provider_budget_days(day)VALUES($1)ON CONFLICT DO NOTHING`, day); err != nil {
		return err
	}
	var reserved int64
	if err = tx.QueryRowContext(ctx, `SELECT reserved_micros FROM cloud_provider_budget_days WHERE day=$1 FOR UPDATE`, day).Scan(&reserved); err != nil {
		return err
	}
	var previous int64
	err = tx.QueryRowContext(ctx, `SELECT max_cost_micros FROM cloud_provider_admissions WHERE job_id=$1`, jobID).Scan(&previous)
	if err == nil {
		if previous != maxCostMicros {
			return ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if reserved > dailyLimitMicros-maxCostMicros {
		return ErrProviderBudget
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_provider_admissions(job_id,day,max_cost_micros)VALUES($1,$2,$3)`, jobID, day, maxCostMicros); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_provider_budget_days SET reserved_micros=reserved_micros+$2 WHERE day=$1`, day, maxCostMicros); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreatePolicyQuote(ctx context.Context, u User, in QuoteInput, p VideoPolicy) (Quote, error) {
	if p.Model == "" || p.Version == "" || p.ProviderMaxCostMicros < 1 || p.CreditsPerSecond < 1 || p.CreditsPerSecond > 10000 || p.Resolution != "720p" || !p.ExpiresAt.After(time.Now()) || strings.TrimSpace(in.Prompt) == "" || len(in.Prompt) > 8000 || in.Duration < 5 || in.Duration > 30 || !hasValue([]string{"16:9", "9:16", "1:1"}, in.AspectRatio) {
		return Quote{}, ErrValidation
	}
	q := Quote{ID: uuid.NewString(), Credits: p.CreditsPerSecond * int64(in.Duration), ExpiresAt: time.Now().UTC().Add(15 * time.Minute)}
	if p.ExpiresAt.Before(q.ExpiresAt) {
		q.ExpiresAt = p.ExpiresAt
	}
	result, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_quotes(id,workspace_id,project_id,prompt,duration,aspect_ratio,credits,model,expires_at,policy_version,provider_max_cost_micros,resolution)SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12 WHERE EXISTS(SELECT 1 FROM cloud_projects WHERE id=$3 AND workspace_id=$2)`, q.ID, u.WorkspaceID, in.ProjectID, in.Prompt, in.Duration, in.AspectRatio, q.Credits, p.Model, q.ExpiresAt, p.Version, p.ProviderMaxCostMicros, p.Resolution)
	if err != nil {
		return Quote{}, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return Quote{}, ErrNotFound
	}
	return q, nil
}
