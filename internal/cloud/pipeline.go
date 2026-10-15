package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"math"
	"net/http"
	"strings"
	"time"
)

type PipelineScene struct {
	Title    string `json:"title"`
	Prompt   string `json:"prompt"`
	Duration int    `json:"duration"`
	Status   string `json:"status"`
}
type PipelineStory struct {
	Script string          `json:"script"`
	Scenes []PipelineScene `json:"scenes"`
}
type PipelineProgress struct {
	Stage  string          `json:"stage"`
	Status string          `json:"status"`
	Script string          `json:"script"`
	Scenes []PipelineScene `json:"scenes"`
}
type PipelineQuoteInput struct {
	Idea         string `json:"idea"`
	Clips        int    `json:"clips"`
	ClipDuration int    `json:"clip_duration"`
	AspectRatio  string `json:"aspect_ratio"`
}

func validatePipelineStory(story PipelineStory, count int) error {
	if strings.TrimSpace(story.Script) == "" || len(story.Script) > 16000 || len(story.Scenes) != count {
		return ErrValidation
	}
	for _, scene := range story.Scenes {
		if strings.TrimSpace(scene.Title) == "" || len(scene.Title) > 180 || strings.TrimSpace(scene.Prompt) == "" || len(scene.Prompt) > 4000 {
			return ErrValidation
		}
	}
	return nil
}
func (s *Store) CreatePipelineQuote(ctx context.Context, u User, project string, in PipelineQuoteInput, policy VideoPolicy, cfg Config) (Quote, error) {
	if in.Clips < 1 || in.Clips > 4 || in.ClipDuration < 5 || in.ClipDuration > 10 || in.Clips*in.ClipDuration > 30 || strings.TrimSpace(in.Idea) == "" || len(in.Idea) > 8000 || cfg.StoryModel == "" || cfg.StoryCredits < 1 || cfg.StoryMaxCostMicros < 1 || policy.ProviderMaxCostMicros < 1 || policy.ProviderMaxCostMicros > (math.MaxInt64-cfg.StoryMaxCostMicros)/int64(in.Clips) || policy.CreditsPerSecond < 1 || policy.CreditsPerSecond > 10000 || cfg.StoryCredits > 1000000 || !policy.ExpiresAt.After(time.Now()) || !hasValue([]string{"16:9", "9:16", "1:1"}, in.AspectRatio) {
		return Quote{}, ErrValidation
	}
	if err := s.ownsStudioProject(ctx, u, project); err != nil {
		return Quote{}, err
	}
	// Split the reviewed per-operation ceiling across the screenplay and clips.
	if cfg.ProviderMaxCostMicros <= cfg.StoryMaxCostMicros {
		return Quote{}, ErrValidation
	}
	allowance := (cfg.ProviderMaxCostMicros - cfg.StoryMaxCostMicros) / int64(in.Clips)
	if allowance > policy.ProviderMaxCostMicros {
		allowance = policy.ProviderMaxCostMicros
	}
	var catalog []byte
	if err := s.DB.QueryRowContext(ctx, `SELECT catalog FROM cloud_video_catalog WHERE refreshed_at>now()-interval '1 day'`).Scan(&catalog); err != nil {
		return Quote{}, ErrUnavailable
	}
	var models []VideoCatalogModel
	if json.Unmarshal(catalog, &models) != nil {
		return Quote{}, ErrUnavailable
	}
	eligible := false
	for _, model := range models {
		if model.ID == policy.Model {
			cost, err := videoPriceMicros(model, in.ClipDuration, in.AspectRatio, policy.Resolution)
			if err == nil && cost <= allowance {
				eligible = true
			}
			break
		}
	}
	if !eligible {
		return Quote{}, ErrValidation
	}
	policy.ProviderMaxCostMicros = allowance
	q := Quote{ID: uuid.NewString(), Credits: cfg.StoryCredits + int64(in.Clips*in.ClipDuration)*policy.CreditsPerSecond, ExpiresAt: time.Now().Add(15 * time.Minute)}
	if policy.ExpiresAt.Before(q.ExpiresAt) {
		q.ExpiresAt = policy.ExpiresAt
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Quote{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO cloud_quotes(id,workspace_id,project_id,prompt,duration,aspect_ratio,credits,model,expires_at,policy_version,provider_max_cost_micros,resolution)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, q.ID, u.WorkspaceID, project, in.Idea, in.Clips*in.ClipDuration, in.AspectRatio, q.Credits, policy.Model, q.ExpiresAt, policy.Version, cfg.StoryMaxCostMicros+int64(in.Clips)*policy.ProviderMaxCostMicros, policy.Resolution)
	if err != nil {
		return Quote{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO cloud_pipeline_plans(quote_id,kind,story_model,clip_count,clip_duration,story_max_cost_micros)VALUES($1,'storyboard',$2,$3,$4,$5)`, q.ID, cfg.StoryModel, in.Clips, in.ClipDuration, cfg.StoryMaxCostMicros)
	if err != nil {
		return Quote{}, err
	}
	return q, tx.Commit()
}
func (s *Store) CreateAssemblyQuote(ctx context.Context, u User, project string, ids []string, credits int64) (Quote, error) {
	if len(ids) < 1 || len(ids) > 8 || credits < 1 || credits > 1000000 {
		return Quote{}, ErrValidation
	}
	if err := s.ownsStudioProject(ctx, u, project); err != nil {
		return Quote{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Quote{}, err
	}
	defer tx.Rollback()
	keys := []string{}
	duration := 0
	for _, id := range ids {
		var key string
		var d int
		err = tx.QueryRowContext(ctx, `SELECT j.output_key,q.duration FROM cloud_jobs j JOIN cloud_quotes q ON q.id=j.quote_id WHERE j.id=$1 AND j.workspace_id=$2 AND q.project_id=$3 AND j.status='succeeded'`, id, u.WorkspaceID, project).Scan(&key, &d)
		if errors.Is(err, sql.ErrNoRows) {
			return Quote{}, ErrNotFound
		}
		if err != nil {
			return Quote{}, err
		}
		if !validMediaKey(key) {
			return Quote{}, ErrValidation
		}
		keys = append(keys, key)
		duration += d
	}
	if duration > 30 {
		return Quote{}, ErrValidation
	}
	if duration < 5 {
		duration = 5
	}
	q := Quote{ID: uuid.NewString(), Credits: credits, ExpiresAt: time.Now().Add(15 * time.Minute)}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_quotes(id,workspace_id,project_id,prompt,duration,aspect_ratio,credits,model,expires_at)VALUES($1,$2,$3,'Assemble approved outputs',$4,'16:9',$5,'local/ffmpeg',$6)`, q.ID, u.WorkspaceID, project, duration, credits, q.ExpiresAt); err != nil {
		return Quote{}, err
	}
	raw, _ := json.Marshal(keys)
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_pipeline_plans(quote_id,kind,stage,source_keys)VALUES($1,'assembly','assembly',$2)`, q.ID, string(raw)); err != nil {
		return Quote{}, err
	}
	return q, tx.Commit()
}
func (s *Store) PipelineState(ctx context.Context, u User, id string) (PipelineProgress, error) {
	p := PipelineProgress{Scenes: []PipelineScene{}}
	var raw []byte
	var quote string
	err := s.DB.QueryRowContext(ctx, `SELECT p.stage,j.status,p.story,j.quote_id FROM cloud_jobs j JOIN cloud_pipeline_plans p ON p.quote_id=j.quote_id WHERE j.id=$1 AND j.workspace_id=$2`, id, u.WorkspaceID).Scan(&p.Stage, &p.Status, &raw, &quote)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	var story PipelineStory
	if err = json.Unmarshal(raw, &story); err != nil {
		return p, err
	}
	p.Script = story.Script
	rows, err := s.DB.QueryContext(ctx, `SELECT title,prompt,status,(SELECT clip_duration FROM cloud_pipeline_plans WHERE quote_id=$1) FROM cloud_pipeline_clips WHERE quote_id=$1 ORDER BY position`, quote)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var scene PipelineScene
		if err = rows.Scan(&scene.Title, &scene.Prompt, &scene.Status, &scene.Duration); err != nil {
			return p, err
		}
		p.Scenes = append(p.Scenes, scene)
	}
	return p, rows.Err()
}
func (s *Server) pipelineRoutes(r chi.Router) {
	r.Get("/jobs/{id}/pipeline", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Store.PipelineState(r.Context(), currentUser(r), chi.URLParam(r, "id"))
		s.result(w, v, e)
	})
	r.Get("/projects/{project}/jobs", func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		project := chi.URLParam(r, "project")
		if e := s.Store.ownsStudioProject(r.Context(), u, project); e != nil {
			s.fail(w, e)
			return
		}
		rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT `+jobCols+` FROM cloud_jobs WHERE workspace_id=$1 AND quote_id IN(SELECT id FROM cloud_quotes WHERE project_id=$2) ORDER BY created_at DESC LIMIT 100`, u.WorkspaceID, project)
		if e != nil {
			s.fail(w, e)
			return
		}
		defer rows.Close()
		jobs := []Job{}
		for rows.Next() {
			j, e := scanJob(rows)
			if e != nil {
				s.fail(w, e)
				return
			}
			jobs = append(jobs, j)
		}
		s.result(w, jobs, rows.Err())
	})
	r.Post("/projects/{project}/assembly/quotes", func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).Verified {
			s.fail(w, ErrUnauthorized)
			return
		}
		if !s.rate(w, r, "pipeline:"+currentUser(r).ID, 10, time.Minute, true) {
			return
		}
		if s.Config.AssemblyCredits < 1 || s.Config.QueueURL == "" || s.Config.OriginalsBucket == "" {
			s.fail(w, ErrUnavailable)
			return
		}
		var b struct {
			JobIDs []string `json:"job_ids"`
		}
		if !s.decode(w, r, &b) {
			return
		}
		v, e := s.Store.CreateAssemblyQuote(r.Context(), currentUser(r), chi.URLParam(r, "project"), b.JobIDs, s.Config.AssemblyCredits)
		s.result(w, v, e)
	})
	r.Post("/projects/{project}/pipelines/quotes", func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if !u.Verified {
			s.fail(w, ErrUnauthorized)
			return
		}
		if !s.rate(w, r, "pipeline:"+u.ID, 10, time.Minute, true) {
			return
		}
		if s.Config.StoryModel == "" || s.Config.StoryCredits < 1 || s.Config.StoryMaxCostMicros < 1 || s.Config.OpenRouterKey == "" || s.Config.ProviderDailyBudgetMicros < 1 || s.Config.QueueURL == "" || s.Config.OriginalsBucket == "" {
			s.fail(w, ErrUnavailable)
			return
		}
		var in PipelineQuoteInput
		if !s.decode(w, r, &in) {
			return
		}
		for _, model := range strings.Split(s.Config.VideoModel, ",") {
			if err := s.Store.ConfigureVideoPolicy(r.Context(), strings.TrimSpace(model), s.Config.CreditsPerSecond, s.Config.ProviderMaxCostMicros); err != nil {
				s.fail(w, err)
				return
			}
		}
		if err := s.Store.RefreshVideoCatalog(r.Context()); err != nil {
			s.fail(w, ErrUnavailable)
			return
		}
		policy, e := s.Store.ActiveVideoPolicy(r.Context(), in.ClipDuration, in.AspectRatio, strings.Split(s.Config.VideoModel, ",")...)
		if e != nil {
			s.fail(w, ErrValidation)
			return
		}
		v, e := s.Store.CreatePipelineQuote(r.Context(), u, chi.URLParam(r, "project"), in, policy, s.Config)
		s.result(w, v, e)
	})
}
