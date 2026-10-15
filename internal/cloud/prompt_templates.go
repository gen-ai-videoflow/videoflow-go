package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type PromptTemplate struct {
	Defaults  map[string]string `json:"defaults,omitempty"`
	ID        string            `json:"id"`
	Revision  int64             `json:"revision"`
	Category  string            `json:"category"`
	TitleEn   string            `json:"title_en"`
	TitleZh   string            `json:"title_zh"`
	BodyEn    string            `json:"body_en"`
	BodyZh    string            `json:"body_zh"`
	UpdatedAt time.Time         `json:"updated_at"`
}

var promptVariableName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

func validatePromptTemplate(p PromptTemplate) error {
	if len(p.Defaults) > 40 {
		return ErrValidation
	}
	rawDefaults, _ := json.Marshal(p.Defaults)
	if len(rawDefaults) > 16000 {
		return ErrValidation
	}
	for key, value := range p.Defaults {
		if !promptVariableName.MatchString(key) || len(value) > 8000 {
			return ErrValidation
		}
	}
	if p.Revision < 0 || !hasValue([]string{"story", "character", "scene", "shot", "asset", "video"}, p.Category) || len(p.TitleEn) > 180 || len(p.TitleZh) > 180 || len(p.BodyEn) > 16000 || len(p.BodyZh) > 16000 || (strings.TrimSpace(p.TitleEn) == "" && strings.TrimSpace(p.TitleZh) == "") || (strings.TrimSpace(p.BodyEn) == "" && strings.TrimSpace(p.BodyZh) == "") {
		return ErrValidation
	}
	if p.ID != "" {
		if _, err := uuid.Parse(p.ID); err != nil {
			return ErrValidation
		}
	}
	return nil
}
func scanPromptTemplate(row interface{ Scan(...any) error }) (PromptTemplate, error) {
	var p PromptTemplate
	var raw []byte
	err := row.Scan(&p.ID, &p.Revision, &raw, &p.UpdatedAt)
	if err == nil {
		var body PromptTemplate
		if err = json.Unmarshal(raw, &body); err == nil {
			body.ID = p.ID
			body.Revision = p.Revision
			body.UpdatedAt = p.UpdatedAt
			p = body
		}
	}
	return p, err
}
func (s *Store) PromptTemplates(ctx context.Context, u User, project string) ([]PromptTemplate, error) {
	result := []PromptTemplate{}
	if err := s.ownsStudioProject(ctx, u, project); err != nil {
		return result, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,revision,template,updated_at FROM cloud_prompt_templates WHERE project_id=$1 ORDER BY updated_at DESC,id LIMIT 100`, project)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		p, e := scanPromptTemplate(rows)
		if e != nil {
			return result, e
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
func (s *Store) SavePromptTemplate(ctx context.Context, u User, project string, p PromptTemplate) (PromptTemplate, error) {
	var empty PromptTemplate
	if err := s.ownsStudioProject(ctx, u, project); err != nil {
		return empty, err
	}
	if err := validatePromptTemplate(p); err != nil {
		return empty, err
	}
	if p.ID == "" {
		if p.Revision != 0 {
			return empty, ErrValidation
		}
		p.ID = uuid.NewString()
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	var locked string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM cloud_projects WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, project, u.WorkspaceID).Scan(&locked); errors.Is(err, sql.ErrNoRows) {
		return empty, ErrNotFound
	} else if err != nil {
		return empty, err
	}
	raw, _ := json.Marshal(p)
	var saved PromptTemplate
	if p.Revision == 0 {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM cloud_prompt_templates WHERE project_id=$1`, project).Scan(&count); err != nil {
			return empty, err
		}
		if count >= 100 {
			return empty, ErrConflict
		}
		saved, err = scanPromptTemplate(tx.QueryRowContext(ctx, `INSERT INTO cloud_prompt_templates(id,project_id,revision,template)VALUES($1,$2,1,$3::jsonb)ON CONFLICT(id)DO NOTHING RETURNING id,revision,template,updated_at`, p.ID, project, raw))
	} else {
		saved, err = scanPromptTemplate(tx.QueryRowContext(ctx, `UPDATE cloud_prompt_templates SET revision=revision+1,template=$3::jsonb,updated_at=now()WHERE id=$1 AND project_id=$2 AND revision=$4 RETURNING id,revision,template,updated_at`, p.ID, project, raw, p.Revision))
	}
	if errors.Is(err, sql.ErrNoRows) {
		return empty, ErrConflict
	}
	if err != nil {
		return empty, err
	}
	return saved, tx.Commit()
}
func (s *Store) DeletePromptTemplate(ctx context.Context, u User, project, id string, revision int64) error {
	if err := s.ownsStudioProject(ctx, u, project); err != nil {
		return err
	}
	if revision < 1 {
		return ErrValidation
	}
	r, err := s.DB.ExecContext(ctx, `DELETE FROM cloud_prompt_templates WHERE id=$1 AND project_id=$2 AND revision=$3`, id, project, revision)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
func (s *Server) listPromptTemplates(w http.ResponseWriter, r *http.Request) {
	v, e := s.Store.PromptTemplates(r.Context(), currentUser(r), chi.URLParam(r, "project"))
	s.result(w, v, e)
}
func (s *Server) savePromptTemplate(w http.ResponseWriter, r *http.Request) {
	var p PromptTemplate
	if !s.decode(w, r, &p) {
		return
	}
	v, e := s.Store.SavePromptTemplate(r.Context(), currentUser(r), chi.URLParam(r, "project"), p)
	s.result(w, v, e)
}
func (s *Server) deletePromptTemplate(w http.ResponseWriter, r *http.Request) {
	revision, _ := strconv.ParseInt(r.URL.Query().Get("revision"), 10, 64)
	e := s.Store.DeletePromptTemplate(r.Context(), currentUser(r), chi.URLParam(r, "project"), chi.URLParam(r, "template"), revision)
	s.result(w, map[string]bool{"deleted": e == nil}, e)
}
