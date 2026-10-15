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

type StudioEntity struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	SceneID     string          `json:"scene_id,omitempty"`
	Kind        string          `json:"kind"`
	Status      string          `json:"status,omitempty"`
	Title       string          `json:"title"`
	Content     string          `json:"content"`
	Description string          `json:"description"`
	Prompt      string          `json:"prompt"`
	ObjectKey   string          `json:"object_key,omitempty"`
	Duration    float64         `json:"duration"`
	Position    int             `json:"position"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

const studioColumns = `id,project_id,COALESCE(scene_id,''),kind,title,content,description,prompt,object_key,duration,position,metadata,created_at,updated_at`

func scanStudio(row interface{ Scan(...any) error }) (StudioEntity, error) {
	var e StudioEntity
	err := row.Scan(&e.ID, &e.ProjectID, &e.SceneID, &e.Kind, &e.Title, &e.Content, &e.Description, &e.Prompt, &e.ObjectKey, &e.Duration, &e.Position, &e.Metadata, &e.CreatedAt, &e.UpdatedAt)
	if e.Kind == "assets" {
		e.Status = "quarantined"
	}
	return e, err
}
func studioKind(kind string) bool {
	return hasValue([]string{"scripts", "characters", "scenes", "shots", "assets"}, kind)
}
func validateStudio(e StudioEntity) error {
	if !studioKind(e.Kind) || strings.TrimSpace(e.Title) == "" || len(e.Title) > 180 || len(e.Content) > 100000 || len(e.Description) > 8000 || len(e.Prompt) > 8000 || math.IsNaN(e.Duration) || math.IsInf(e.Duration, 0) || e.Duration < 0 || e.Duration > 3600 || e.Position < 0 || e.Position > 10000 {
		return ErrValidation
	}
	if len(e.Metadata) > 32768 {
		return ErrValidation
	}
	if len(e.Metadata) > 0 {
		var m map[string]any
		if json.Unmarshal(e.Metadata, &m) != nil || m == nil {
			return ErrValidation
		}
	}
	return nil
}
func (s *Store) ownsStudioProject(ctx context.Context, u User, project string) error {
	var owned bool
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_projects WHERE id=$1 AND workspace_id=$2)`, project, u.WorkspaceID).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return ErrNotFound
	}
	return nil
}
func (s *Store) StudioEntities(ctx context.Context, u User, project, kind, scene string) ([]StudioEntity, error) {
	if !studioKind(kind) {
		return nil, ErrValidation
	}
	if err := s.ownsStudioProject(ctx, u, project); err != nil {
		return nil, err
	}
	if kind == "shots" {
		var valid bool
		if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_studio_entities WHERE id=$1 AND project_id=$2 AND kind='scenes')`, scene, project).Scan(&valid); err != nil {
			return nil, err
		}
		if !valid {
			return nil, ErrNotFound
		}
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+studioColumns+` FROM cloud_studio_entities WHERE project_id=$1 AND kind=$2 AND ($3='' OR scene_id=$3) ORDER BY position,created_at LIMIT 1000`, project, kind, scene)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StudioEntity{}
	for rows.Next() {
		e, err := scanStudio(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func createStudioTx(ctx context.Context, tx *sql.Tx, u User, project, kind string, e StudioEntity) (StudioEntity, error) {
	e.ID = uuid.NewString()
	e.ProjectID = project
	e.Kind = kind
	e.Title = strings.TrimSpace(e.Title)
	if len(e.Metadata) == 0 {
		e.Metadata = json.RawMessage(`{}`)
	}
	if err := validateStudio(e); err != nil {
		return StudioEntity{}, err
	}
	var owned bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_projects WHERE id=$1 AND workspace_id=$2)`, project, u.WorkspaceID).Scan(&owned); err != nil {
		return StudioEntity{}, err
	}
	if !owned {
		return StudioEntity{}, ErrNotFound
	}

	// The credit account row is the workspace's durable quota serialization point.
	var locked string
	if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM cloud_credit_accounts WHERE workspace_id=$1 FOR UPDATE`, u.WorkspaceID).Scan(&locked); err != nil {
		return StudioEntity{}, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM cloud_studio_entities e JOIN cloud_projects p ON p.id=e.project_id WHERE p.workspace_id=$1`, u.WorkspaceID).Scan(&count); err != nil {
		return StudioEntity{}, err
	}
	if count >= 500 {
		return StudioEntity{}, ErrConflict
	}
	var scene any
	if kind == "shots" {
		var valid bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_studio_entities WHERE id=$1 AND project_id=$2 AND kind='scenes')`, e.SceneID, project).Scan(&valid); err != nil {
			return StudioEntity{}, err
		}
		if !valid {
			return StudioEntity{}, ErrNotFound
		}
		scene = e.SceneID
	} else if e.SceneID != "" {
		return StudioEntity{}, ErrValidation
	}
	if e.ObjectKey != "" {
		if kind != "assets" || !validMediaKey(e.ObjectKey) {
			return StudioEntity{}, ErrValidation
		}
		var valid bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_upload_reservations WHERE object_key=$1 AND workspace_id=$2 AND deleted_at IS NULL)`, e.ObjectKey, u.WorkspaceID).Scan(&valid); err != nil {
			return StudioEntity{}, err
		}
		if !valid {
			return StudioEntity{}, ErrNotFound
		}
	}
	return scanStudio(tx.QueryRowContext(ctx, `INSERT INTO cloud_studio_entities(id,project_id,scene_id,kind,title,content,description,prompt,object_key,duration,position,metadata)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)RETURNING `+studioColumns, e.ID, project, scene, kind, e.Title, e.Content, e.Description, e.Prompt, e.ObjectKey, e.Duration, e.Position, string(e.Metadata)))
}
func (s *Store) CreateStudioEntity(ctx context.Context, u User, project, kind string, e StudioEntity) (StudioEntity, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return StudioEntity{}, err
	}
	defer tx.Rollback()
	e, err = createStudioTx(ctx, tx, u, project, kind, e)
	if err != nil {
		return StudioEntity{}, err
	}
	return e, tx.Commit()
}
func (s *Store) UpdateStudioEntity(ctx context.Context, u User, project, kind, id string, patch map[string]any) error {
	if !studioKind(kind) || len(patch) == 0 {
		return ErrValidation
	}
	for key, value := range patch {
		if value == nil {
			return ErrValidation
		}
		if !hasValue([]string{"title", "content", "description", "prompt", "duration", "position", "metadata"}, key) {
			return ErrValidation
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	e, err := scanStudio(tx.QueryRowContext(ctx, `SELECT `+studioColumns+` FROM cloud_studio_entities WHERE id=$1 AND project_id=$2 AND kind=$3 AND EXISTS(SELECT 1 FROM cloud_projects WHERE id=$2 AND workspace_id=$4) FOR UPDATE`, id, project, kind, u.WorkspaceID))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return ErrValidation
	}
	if err = json.Unmarshal(data, &e); err != nil {
		return ErrValidation
	}
	if err = validateStudio(e); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE cloud_studio_entities SET title=$2,content=$3,description=$4,prompt=$5,duration=$6,position=$7,metadata=$8,updated_at=now() WHERE id=$1`, id, e.Title, e.Content, e.Description, e.Prompt, e.Duration, e.Position, string(e.Metadata))
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) DeleteStudioEntity(ctx context.Context, u User, project, kind, id string) error {
	if !studioKind(kind) {
		return ErrValidation
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM cloud_studio_entities WHERE id=$1 AND project_id=$2 AND kind=$3 AND EXISTS(SELECT 1 FROM cloud_projects WHERE id=$2 AND workspace_id=$4)`, id, project, kind, u.WorkspaceID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type CaptionSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}
type JobCaptions struct {
	Segments []CaptionSegment `json:"segments"`
	Duration int              `json:"duration"`
}

func (s *Store) Captions(ctx context.Context, u User, job string) (JobCaptions, error) {
	c := JobCaptions{Segments: []CaptionSegment{}}
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT q.duration,COALESCE(c.segments,'[]'::jsonb) FROM cloud_jobs j JOIN cloud_quotes q ON q.id=j.quote_id LEFT JOIN cloud_job_captions c ON c.job_id=j.id WHERE j.id=$1 AND j.workspace_id=$2 AND j.status='succeeded'`, job, u.WorkspaceID).Scan(&c.Duration, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(raw, &c.Segments)
	return c, err
}
func (s *Store) SaveCaptions(ctx context.Context, u User, job string, segments []CaptionSegment) error {
	c, err := s.Captions(ctx, u, job)
	if err != nil {
		return err
	}
	if len(segments) > 1000 {
		return ErrValidation
	}
	previous := float64(0)
	for _, segment := range segments {
		if math.IsNaN(segment.Start) || math.IsNaN(segment.End) || math.IsInf(segment.Start, 0) || math.IsInf(segment.End, 0) || segment.Start < previous || segment.End <= segment.Start || segment.End > float64(c.Duration) || strings.TrimSpace(segment.Text) == "" || len(segment.Text) > 2000 {
			return ErrValidation
		}
		previous = segment.End
	}
	if segments == nil {
		segments = []CaptionSegment{}
	}
	raw, err := json.Marshal(segments)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_job_captions(job_id,segments)VALUES($1,$2)ON CONFLICT(job_id)DO UPDATE SET segments=excluded.segments,version=cloud_job_captions.version+1,updated_at=now()`, job, string(raw))
	return err
}

type ProjectArchive struct {
	Version int `json:"version"`
	Project struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"project"`
	Entities []StudioEntity `json:"entities"`
}

func (s *Store) StreamProjectExport(ctx context.Context, u User, project string) (map[string]any, error) {
	if err := s.ownsStudioProject(ctx, u, project); err != nil {
		return nil, err
	}
	var name, description string
	if err := s.DB.QueryRowContext(ctx, `SELECT name,description FROM cloud_projects WHERE id=$1`, project).Scan(&name, &description); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+studioColumns+` FROM cloud_studio_entities WHERE project_id=$1 ORDER BY CASE WHEN kind='shots' THEN 1 ELSE 0 END,position,id LIMIT 5001`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entities := []StudioEntity{}
	for rows.Next() {
		e, err := scanStudio(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(entities) > 5000 {
		return nil, ErrValidation
	}
	return map[string]any{"version": 1, "project": map[string]string{"name": name, "description": description}, "entities": entities}, nil
}
func (s *Store) ImportStudioProject(ctx context.Context, u User, a ProjectArchive) (Project, error) {
	a.Project.Name = strings.TrimSpace(a.Project.Name)
	if a.Version != 1 || a.Project.Name == "" || len(a.Project.Name) > 120 || len(a.Project.Description) > 4000 || len(a.Entities) > 5000 {
		return Project{}, ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()
	if err = checkProjectQuotaTx(ctx, tx, u.WorkspaceID); err != nil {
		return Project{}, err
	}
	p := Project{ID: uuid.NewString(), WorkspaceID: u.WorkspaceID, Name: a.Project.Name, Description: a.Project.Description}
	if err = tx.QueryRowContext(ctx, `INSERT INTO cloud_projects(id,workspace_id,name,description)VALUES($1,$2,$3,$4)RETURNING created_at`, p.ID, p.WorkspaceID, p.Name, p.Description).Scan(&p.CreatedAt); err != nil {
		return Project{}, err
	}
	mapping := map[string]string{}
	for _, e := range a.Entities {
		if e.ID == "" {
			return Project{}, ErrValidation
		}
		if _, exists := mapping[e.ID]; exists {
			return Project{}, ErrValidation
		}
		mapping[e.ID] = ""
	}
	for _, shots := range []bool{false, true} {
		for _, e := range a.Entities {
			if (e.Kind == "shots") != shots {
				continue
			}
			oldID := e.ID
			if shots {
				parent, ok := mapping[e.SceneID]
				if !ok || parent == "" {
					return Project{}, ErrValidation
				}
				e.SceneID = parent
			}
			made, err := createStudioTx(ctx, tx, u, p.ID, e.Kind, e)
			if err != nil {
				return Project{}, err
			}
			mapping[oldID] = made.ID
		}
	}
	return p, tx.Commit()
}
func (s *Server) studioRoutes(r chi.Router) {
	r.Post("/projects/import", func(w http.ResponseWriter, r *http.Request) {
		var a ProjectArchive
		if !s.decode(w, r, &a) {
			return
		}
		p, e := s.Store.ImportStudioProject(r.Context(), currentUser(r), a)
		s.result(w, p, e)
	})
	r.Get("/projects/{project}/export", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Store.StreamProjectExport(r.Context(), currentUser(r), chi.URLParam(r, "project"))
		s.result(w, v, e)
	})
	for _, kind := range []string{"scripts", "characters", "scenes", "assets"} {
		kind := kind
		base := "/projects/{project}/" + kind
		r.Get(base, s.listStudio(kind))
		r.Post(base, s.createStudio(kind))
		r.Patch(base+"/{entity}", s.patchStudio(kind))
		r.Delete(base+"/{entity}", s.deleteStudio(kind))
	}
	base := "/projects/{project}/scenes/{scene}/shots"
	r.Get(base, s.listStudio("shots"))
	r.Post(base, s.createStudio("shots"))
	r.Patch(base+"/{entity}", s.patchStudio("shots"))
	r.Delete(base+"/{entity}", s.deleteStudio("shots"))
	r.Get("/jobs/{id}/captions", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Store.Captions(r.Context(), currentUser(r), chi.URLParam(r, "id"))
		s.result(w, v, e)
	})
	r.Put("/jobs/{id}/captions", func(w http.ResponseWriter, r *http.Request) {
		var c struct {
			Segments []CaptionSegment `json:"segments"`
		}
		if !s.decode(w, r, &c) {
			return
		}
		e := s.Store.SaveCaptions(r.Context(), currentUser(r), chi.URLParam(r, "id"), c.Segments)
		s.result(w, map[string]bool{"ok": true}, e)
	})
}
func (s *Server) listStudio(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Store.StudioEntities(r.Context(), currentUser(r), chi.URLParam(r, "project"), kind, chi.URLParam(r, "scene"))
		s.result(w, v, e)
	}
}
func (s *Server) createStudio(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var v StudioEntity
		if !s.decode(w, r, &v) {
			return
		}
		if kind == "shots" {
			if v.SceneID != "" && v.SceneID != chi.URLParam(r, "scene") {
				s.fail(w, ErrValidation)
				return
			}
			v.SceneID = chi.URLParam(r, "scene")
		}
		v, e := s.Store.CreateStudioEntity(r.Context(), currentUser(r), chi.URLParam(r, "project"), kind, v)
		s.result(w, v, e)
	}
}
func (s *Server) studioPathEntity(r *http.Request, kind string) error {
	if kind != "shots" {
		return nil
	}
	var exists bool
	err := s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_studio_entities WHERE id=$1 AND project_id=$2 AND scene_id=$3 AND kind='shots')`, chi.URLParam(r, "entity"), chi.URLParam(r, "project"), chi.URLParam(r, "scene")).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}
func (s *Server) patchStudio(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if e := s.studioPathEntity(r, kind); e != nil {
			s.fail(w, e)
			return
		}
		var patch map[string]any
		if !s.decode(w, r, &patch) {
			return
		}
		e := s.Store.UpdateStudioEntity(r.Context(), currentUser(r), chi.URLParam(r, "project"), kind, chi.URLParam(r, "entity"), patch)
		s.result(w, map[string]bool{"ok": true}, e)
	}
}
func (s *Server) deleteStudio(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if e := s.studioPathEntity(r, kind); e != nil {
			s.fail(w, e)
			return
		}
		e := s.Store.DeleteStudioEntity(r.Context(), currentUser(r), chi.URLParam(r, "project"), kind, chi.URLParam(r, "entity"))
		s.result(w, map[string]bool{"ok": true}, e)
	}
}

// Launch defaults: at most 100 projects and 500 studio entities per workspace.
// Both project creation and archive import use the same locked admission check.
func checkProjectQuotaTx(ctx context.Context, tx *sql.Tx, workspace string) error {
	var locked string
	if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM cloud_credit_accounts WHERE workspace_id=$1 FOR UPDATE`, workspace).Scan(&locked); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM cloud_projects WHERE workspace_id=$1`, workspace).Scan(&count); err != nil {
		return err
	}
	if count >= 100 {
		return ErrConflict
	}
	return nil
}
