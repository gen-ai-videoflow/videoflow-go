package cloud

import (
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

type StoryCatalogModel struct {
	ID            string `json:"id"`
	ContextLength int    `json:"context_length"`
	Architecture  struct {
		Input  []string `json:"input_modalities"`
		Output []string `json:"output_modalities"`
	} `json:"architecture"`
	TopProvider struct {
		ContextLength       int  `json:"context_length"`
		MaxCompletionTokens *int `json:"max_completion_tokens"`
	} `json:"top_provider"`
	SupportedParameters []string          `json:"supported_parameters"`
	Pricing             map[string]string `json:"pricing"`
}

const storyOutputTokens = 2048

var storyDecimal = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,18})?$`)

func approvedStoryModel(allowlist, model string) bool {
	for _, candidate := range strings.Split(allowlist, ",") {
		if strings.TrimSpace(candidate) == model {
			return true
		}
	}
	return false
}
func cheapestStoryModel(models []StoryCatalogModel, approved, idea string, budget int64) (string, error) {
	if budget < 1 || len(idea) > 8000 {
		return "", ErrValidation
	}
	inputTokens := len(idea) + 2048 // Conservative byte-per-token bound plus system/message overhead.
	best := ""
	var bestCost *big.Rat
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	for _, model := range models {
		if (model.TopProvider.MaxCompletionTokens != nil && *model.TopProvider.MaxCompletionTokens < storyOutputTokens) || (model.TopProvider.ContextLength > 0 && model.TopProvider.ContextLength < inputTokens+storyOutputTokens) {
			continue
		}
		if !approvedStoryModel(approved, model.ID) || strings.Contains(model.ID, ":free") || model.ContextLength < inputTokens+storyOutputTokens || !hasValue(model.Architecture.Input, "text") || !hasValue(model.Architecture.Output, "text") || !hasValue(model.SupportedParameters, "max_tokens") || !hasValue(model.SupportedParameters, "temperature") {
			continue
		}
		valid := true
		for key, price := range model.Pricing {
			if key != "prompt" && key != "completion" {
				if len(price) > 64 {
					valid = false
					continue
				}
				value, ok := new(big.Rat).SetString(price)
				if !ok || value.Sign() != 0 {
					valid = false
				}
			}
		}
		prompt, completion := model.Pricing["prompt"], model.Pricing["completion"]
		if !valid || len(prompt) > 64 || len(completion) > 64 || !storyDecimal.MatchString(prompt) || !storyDecimal.MatchString(completion) {
			continue
		}
		a, _ := new(big.Rat).SetString(prompt)
		b, _ := new(big.Rat).SetString(completion)
		if a.Sign() <= 0 || b.Sign() <= 0 {
			continue
		}
		// Match the worker's conservative per-token provider ceiling, so a quoted
		// model is not selected outside the enforced execution price limit.
		rateUnits := new(big.Int).Quo(new(big.Int).Mul(big.NewInt(budget), big.NewInt(1000000)), big.NewInt(18432))
		perTokenLimit := new(big.Rat).SetFrac(rateUnits, big.NewInt(1000000000000))
		if a.Cmp(perTokenLimit) > 0 || b.Cmp(perTokenLimit) > 0 {
			continue
		}
		cost := new(big.Rat).Add(new(big.Rat).Mul(a, big.NewRat(int64(inputTokens), 1)), new(big.Rat).Mul(b, big.NewRat(storyOutputTokens, 1)))
		micros := new(big.Rat).Mul(cost, big.NewRat(1000000, 1))
		if micros.Cmp(big.NewRat(budget, 1)) > 0 {
			continue
		}
		if bestCost == nil || cost.Cmp(bestCost) < 0 {
			best = model.ID
			bestCost = cost
		}
	}
	if best == "" {
		return "", ErrUnavailable
	}
	return best, nil
}
func (s *Server) selectStoryModel(ctx context.Context, idea string) (string, error) {
	cfg, err := s.effectiveConfig(ctx)
	if err != nil {
		return "", err
	}
	return s.selectStoryModelWithConfig(ctx, idea, cfg)
}
func (s *Server) selectStoryModelWithConfig(ctx context.Context, idea string, cfg Config) (string, error) {
	ctx, stop := context.WithTimeout(ctx, 25*time.Second)
	defer stop()
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(814037236)`); err != nil {
		return "", err
	}
	var fresh bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_story_catalog WHERE refreshed_at>now()-interval '1 hour')`).Scan(&fresh); err != nil {
		return "", err
	}
	if !fresh {
		requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		req, e := http.NewRequestWithContext(requestCtx, "GET", "https://openrouter.ai/api/v1/models", nil)
		if e != nil {
			return "", e
		}
		req.Header.Set("Authorization", "Bearer "+cfg.OpenRouterKey)
		client := *s.HTTP
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return ErrValidation }
		res, e := client.Do(req)
		if e != nil {
			return "", ErrUnavailable
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return "", ErrUnavailable
		}
		raw, e := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
		if e != nil || len(raw) > 8<<20 {
			return "", ErrUnavailable
		}
		var envelope struct {
			Data []StoryCatalogModel `json:"data"`
		}
		if json.Unmarshal(raw, &envelope) != nil || len(envelope.Data) == 0 {
			return "", ErrUnavailable
		}
		catalog, _ := json.Marshal(envelope.Data)
		if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_story_catalog(id,catalog)VALUES(true,$1)ON CONFLICT(id)DO UPDATE SET catalog=excluded.catalog,refreshed_at=now()`, catalog); err != nil {
			return "", err
		}
	}
	var raw []byte
	if err = tx.QueryRowContext(ctx, `SELECT catalog FROM cloud_story_catalog WHERE id=true`).Scan(&raw); err != nil {
		return "", err
	}
	var models []StoryCatalogModel
	if json.Unmarshal(raw, &models) != nil {
		return "", ErrUnavailable
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return cheapestStoryModel(models, cfg.StoryModel, idea, cfg.StoryMaxCostMicros)
}
