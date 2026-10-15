package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"math"
	"net/http"
	"strings"
)

type CanvasNode struct {
	ID    string         `json:"id"`
	Type  string         `json:"type"`
	Label string         `json:"label"`
	Data  map[string]any `json:"data,omitempty"`
}
type CanvasEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label"`
}
type CanvasGraph struct {
	Nodes []CanvasNode `json:"nodes"`
	Edges []CanvasEdge `json:"edges"`
}

func canvasData(e StudioEntity) map[string]any {
	meta := map[string]any{}
	_ = json.Unmarshal(e.Metadata, &meta)
	text := func(key string) string { v, _ := meta[key].(string); return v }
	d := map[string]any{"id": e.ID, "project_id": e.ProjectID, "title": e.Title, "name": e.Title, "description": e.Description, "content": e.Content, "duration": e.Duration, "prompt": e.Prompt, "position": e.Position}
	switch e.Kind {
	case "scenes":
		d["summary"] = e.Description
		d["aspect_ratio"] = text("aspect_ratio")
	case "shots":
		d["scene_id"] = e.SceneID
		d["shot_order"] = e.Position
		d["camera"] = text("camera")
		d["movement"] = text("movement")
	case "assets":
		d["status"] = "quarantined"
		d["file_path"] = ""
		d["asset_type"] = text("asset_type")
		if d["asset_type"] == "" {
			d["asset_type"] = text("type")
		}
	}
	return d
}
func (s *Store) CloudCanvasGraph(ctx context.Context, u User, project string) (CanvasGraph, error) {
	g := CanvasGraph{Nodes: []CanvasNode{}, Edges: []CanvasEdge{}}
	if err := s.ownsStudioProject(ctx, u, project); err != nil {
		return g, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+studioColumns+` FROM cloud_studio_entities WHERE project_id=$1 AND kind<>'scripts' ORDER BY position,created_at`, project)
	if err != nil {
		return g, err
	}
	known := map[string]bool{}
	for rows.Next() {
		e, err := scanStudio(rows)
		if err != nil {
			rows.Close()
			return g, err
		}
		kind := strings.TrimSuffix(e.Kind, "s")
		label := e.Title
		if kind == "shot" {
			label = fmt.Sprintf("#%d %s", e.Position+1, e.Title)
		}
		g.Nodes = append(g.Nodes, CanvasNode{ID: e.ID, Type: kind, Label: label, Data: canvasData(e)})
		known[e.ID] = true
		if kind == "shot" {
			g.Edges = append(g.Edges, CanvasEdge{Source: e.SceneID, Target: e.ID, Label: "shot"})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return g, err
	}
	edges, err := s.DB.QueryContext(ctx, `SELECT source_id,target_id,label FROM cloud_canvas_edges WHERE project_id=$1 ORDER BY source_id,target_id,label`, project)
	if err != nil {
		return g, err
	}
	for edges.Next() {
		var e CanvasEdge
		if err = edges.Scan(&e.Source, &e.Target, &e.Label); err != nil {
			edges.Close()
			return g, err
		}
		if known[e.Source] && known[e.Target] {
			g.Edges = append(g.Edges, e)
		}
	}
	err = edges.Err()
	edges.Close()
	if err != nil {
		return g, err
	}
	jobs, err := s.DB.QueryContext(ctx, `SELECT j.id,j.status FROM cloud_jobs j JOIN cloud_quotes q ON q.id=j.quote_id WHERE j.workspace_id=$1 AND q.project_id=$2 ORDER BY j.created_at LIMIT 100`, u.WorkspaceID, project)
	if err != nil {
		return g, err
	}
	defer jobs.Close()
	for jobs.Next() {
		var id, status string
		if err = jobs.Scan(&id, &status); err != nil {
			return g, err
		}
		g.Nodes = append(g.Nodes, CanvasNode{ID: id, Type: "render_job", Label: status, Data: map[string]any{"status": status, "job_id": id}})
		if status == "succeeded" {
			output := "output:" + id
			g.Nodes = append(g.Nodes, CanvasNode{ID: output, Type: "output", Label: "Generated video", Data: map[string]any{"job_id": id, "render_job_id": id, "video_path": "", "thumbnail_path": "", "download_endpoint": "/v1/jobs/" + id + "/download"}})
			g.Edges = append(g.Edges, CanvasEdge{Source: id, Target: output, Label: "output"})
		}
	}
	return g, jobs.Err()
}
func validCanvasEdge(source, target, label string) bool {
	return (label == "casts" && source == "scenes" && target == "characters") || (label == "uses" && (source == "shots" || source == "scenes") && target == "assets") || (label == "reference" && source == "characters" && target == "assets")
}
func (s *Store) SetCanvasEdge(ctx context.Context, u User, project string, e CanvasEdge, remove bool) error {
	if e.Source == e.Target || e.Source == "" || e.Target == "" {
		return ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked string
	if err = tx.QueryRowContext(ctx, `SELECT workspace_id FROM cloud_credit_accounts WHERE workspace_id=$1 FOR UPDATE`, u.WorkspaceID).Scan(&locked); err != nil {
		return err
	}
	var source, target string
	err = tx.QueryRowContext(ctx, `SELECT a.kind,b.kind FROM cloud_studio_entities a JOIN cloud_studio_entities b ON b.project_id=a.project_id JOIN cloud_projects p ON p.id=a.project_id WHERE a.id=$1 AND b.id=$2 AND p.id=$3 AND p.workspace_id=$4`, e.Source, e.Target, project, u.WorkspaceID).Scan(&source, &target)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !validCanvasEdge(source, target, e.Label) {
		return ErrValidation
	}
	if remove {
		_, err = tx.ExecContext(ctx, `DELETE FROM cloud_canvas_edges WHERE project_id=$1 AND source_id=$2 AND target_id=$3 AND label=$4`, project, e.Source, e.Target, e.Label)
	} else {
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_canvas_edges WHERE project_id=$1 AND source_id=$2 AND target_id=$3 AND label=$4)`, project, e.Source, e.Target, e.Label).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return tx.Commit()
		}
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM cloud_canvas_edges e JOIN cloud_projects p ON p.id=e.project_id WHERE p.workspace_id=$1`, u.WorkspaceID).Scan(&count); err != nil {
			return err
		}
		if count >= 2000 {
			return ErrConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_canvas_edges(project_id,source_id,target_id,label)VALUES($1,$2,$3,$4)ON CONFLICT DO NOTHING`, project, e.Source, e.Target, e.Label)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
func canvasApply(e *StudioEntity, kind string, patch map[string]any) error {
	metadata := map[string]any{}
	_ = json.Unmarshal(e.Metadata, &metadata)
	allowed := []string{"description"}
	switch kind {
	case "scenes":
		allowed = append(allowed, "title", "summary", "prompt", "duration", "aspect_ratio")
	case "shots":
		allowed = append(allowed, "title", "scene_id", "prompt", "duration", "camera", "movement", "shot_order")
	case "characters":
		allowed = append(allowed, "name", "content", "prompt")
	case "assets":
		allowed = append(allowed, "name", "object_key", "asset_type", "prompt")
	default:
		return ErrValidation
	}
	for key, value := range patch {
		if !hasValue(allowed, key) || value == nil {
			return ErrValidation
		}
		switch key {
		case "duration":
			number, ok := value.(float64)
			if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number > 3600 {
				return ErrValidation
			}
			e.Duration = number
		case "shot_order":
			number, ok := value.(float64)
			if !ok || number < 0 || number > 10000 || math.Trunc(number) != number {
				return ErrValidation
			}
			e.Position = int(number)
		default:
			text, ok := value.(string)
			if !ok {
				return ErrValidation
			}
			switch key {
			case "title", "name":
				e.Title = text
			case "description", "summary":
				e.Description = text
			case "prompt":
				e.Prompt = text
			case "content":
				e.Content = text
			case "scene_id":
				if e.SceneID != "" && e.SceneID != text {
					return ErrValidation
				}
				e.SceneID = text
			case "object_key":
				if e.ObjectKey != "" && e.ObjectKey != text {
					return ErrValidation
				}
				e.ObjectKey = text
			case "aspect_ratio":
				if text != "" && !hasValue([]string{"16:9", "9:16", "1:1", "4:3", "3:4", "21:9"}, text) {
					return ErrValidation
				}
				metadata[key] = text
			case "camera", "movement", "asset_type":
				if len(text) > 500 {
					return ErrValidation
				}
				metadata[key] = text
			}
		}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	e.Metadata = raw
	return nil
}
func (s *Store) CreateCanvasEntity(ctx context.Context, u User, project, kind string, body map[string]any) (map[string]any, error) {
	e := StudioEntity{Kind: kind}
	if kind == "shots" {
		e.Title = "Shot"
	}
	if err := canvasApply(&e, kind, body); err != nil {
		return nil, err
	}
	e, err := s.CreateStudioEntity(ctx, u, project, kind, e)
	if err != nil {
		return nil, err
	}
	return canvasData(e), nil
}
func (s *Store) PatchCanvasEntity(ctx context.Context, u User, project, kind, id string, body map[string]any) error {
	if len(body) == 0 {
		return ErrValidation
	}
	if _, exists := body["scene_id"]; exists {
		return ErrValidation
	}
	if _, exists := body["object_key"]; exists {
		return ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	e, err := scanStudio(tx.QueryRowContext(ctx, `SELECT `+studioColumns+` FROM cloud_studio_entities WHERE id=$1 AND project_id=$2 AND kind=$3 AND EXISTS(SELECT 1 FROM cloud_projects WHERE id=$2 AND workspace_id=$4)FOR UPDATE`, id, project, kind, u.WorkspaceID))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err = canvasApply(&e, kind, body); err != nil {
		return err
	}
	if err = validateStudio(e); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE cloud_studio_entities SET title=$2,description=$3,prompt=$4,duration=$5,position=$6,metadata=$7,content=$8,updated_at=now() WHERE id=$1`, id, e.Title, e.Description, e.Prompt, e.Duration, e.Position, string(e.Metadata), e.Content)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Server) canvasRoutes(r chi.Router) {
	r.Get("/projects/{project}/graph", func(w http.ResponseWriter, r *http.Request) {
		g, e := s.Store.CloudCanvasGraph(r.Context(), currentUser(r), chi.URLParam(r, "project"))
		s.result(w, g, e)
	})
	for _, kind := range []string{"scenes", "shots", "characters", "assets"} {
		kind := kind
		base := "/projects/{project}/canvas/" + kind
		r.Get(base, func(w http.ResponseWriter, r *http.Request) {
			u := currentUser(r)
			project := chi.URLParam(r, "project")
			if e := s.Store.ownsStudioProject(r.Context(), u, project); e != nil {
				s.fail(w, e)
				return
			}
			rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT `+studioColumns+` FROM cloud_studio_entities WHERE project_id=$1 AND kind=$2 ORDER BY position,created_at`, project, kind)
			if e != nil {
				s.fail(w, e)
				return
			}
			defer rows.Close()
			items := []map[string]any{}
			for rows.Next() {
				item, e := scanStudio(rows)
				if e != nil {
					s.fail(w, e)
					return
				}
				items = append(items, canvasData(item))
			}
			s.result(w, items, rows.Err())
		})
		r.Post(base, func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if !s.decode(w, r, &body) {
				return
			}
			v, e := s.Store.CreateCanvasEntity(r.Context(), currentUser(r), chi.URLParam(r, "project"), kind, body)
			s.result(w, v, e)
		})
		r.Get(base+"/{entity}", func(w http.ResponseWriter, r *http.Request) {
			v, e := scanStudio(s.Store.DB.QueryRowContext(r.Context(), `SELECT `+studioColumns+` FROM cloud_studio_entities WHERE id=$1 AND project_id=$2 AND kind=$3 AND EXISTS(SELECT 1 FROM cloud_projects WHERE id=$2 AND workspace_id=$4)`, chi.URLParam(r, "entity"), chi.URLParam(r, "project"), kind, currentUser(r).WorkspaceID))
			if errors.Is(e, sql.ErrNoRows) {
				e = ErrNotFound
			}
			s.result(w, canvasData(v), e)
		})
		r.Patch(base+"/{entity}", func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if !s.decode(w, r, &body) {
				return
			}
			e := s.Store.PatchCanvasEntity(r.Context(), currentUser(r), chi.URLParam(r, "project"), kind, chi.URLParam(r, "entity"), body)
			s.result(w, map[string]bool{"ok": true}, e)
		})
		r.Delete(base+"/{entity}", func(w http.ResponseWriter, r *http.Request) {
			detached, e := s.Store.DeleteCanvasEntity(r.Context(), currentUser(r), chi.URLParam(r, "project"), kind, chi.URLParam(r, "entity"))
			s.result(w, map[string]any{"ok": true, "detached_from": detached}, e)
		})
	}
	for _, method := range []string{"POST", "DELETE"} {
		method := method
		handler := func(w http.ResponseWriter, r *http.Request) {
			var edge CanvasEdge
			if !s.decode(w, r, &edge) {
				return
			}
			e := s.Store.SetCanvasEdge(r.Context(), currentUser(r), chi.URLParam(r, "project"), edge, method == "DELETE")
			s.result(w, map[string]bool{"ok": true}, e)
		}
		r.MethodFunc(method, "/projects/{project}/edges", handler)
		r.MethodFunc(method, "/projects/{project}/scenes/{scene}/cast/{character}", func(w http.ResponseWriter, r *http.Request) {
			e := s.Store.SetCanvasEdge(r.Context(), currentUser(r), chi.URLParam(r, "project"), CanvasEdge{Source: chi.URLParam(r, "scene"), Target: chi.URLParam(r, "character"), Label: "casts"}, method == "DELETE")
			s.result(w, map[string]bool{"ok": true}, e)
		})
		r.MethodFunc(method, "/projects/{project}/shots/{shot}/assets/{asset}", func(w http.ResponseWriter, r *http.Request) {
			e := s.Store.SetCanvasEdge(r.Context(), currentUser(r), chi.URLParam(r, "project"), CanvasEdge{Source: chi.URLParam(r, "shot"), Target: chi.URLParam(r, "asset"), Label: "uses"}, method == "DELETE")
			s.result(w, map[string]bool{"ok": true}, e)
		})
	}
}

func (s *Store) DeleteCanvasEntity(ctx context.Context, u User, project, kind, id string) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var locked string
	err = tx.QueryRowContext(ctx, `SELECT id FROM cloud_studio_entities WHERE id=$1 AND project_id=$2 AND kind=$3 AND EXISTS(SELECT 1 FROM cloud_projects WHERE id=$2 AND workspace_id=$4)FOR UPDATE`, id, project, kind, u.WorkspaceID).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	var detached int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM cloud_canvas_edges WHERE project_id=$1 AND (source_id=$2 OR target_id=$2)`, project, id).Scan(&detached); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM cloud_studio_entities WHERE id=$1`, id); err != nil {
		return 0, err
	}
	return detached, tx.Commit()
}
