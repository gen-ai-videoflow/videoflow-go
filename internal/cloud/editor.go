package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
)

type EditorDocument struct {
	Revision  int64           `json:"revision"`
	Document  json.RawMessage `json:"document"`
	UpdatedAt *time.Time      `json:"updated_at,omitempty"`
}

func (s *Store) EditorDocument(ctx context.Context, u User, project string) (EditorDocument, error) {
	d := EditorDocument{Document: json.RawMessage(`null`)}
	if e := s.ownsStudioProject(ctx, u, project); e != nil {
		return d, e
	}
	var updated time.Time
	e := s.DB.QueryRowContext(ctx, `SELECT revision,document,updated_at FROM cloud_editor_documents WHERE project_id=$1`, project).Scan(&d.Revision, &d.Document, &updated)
	if errors.Is(e, sql.ErrNoRows) {
		return d, nil
	}
	if e == nil {
		d.UpdatedAt = &updated
	}
	return d, e
}
func (s *Store) SaveEditorDocument(ctx context.Context, u User, project string, revision int64, body json.RawMessage) (EditorDocument, error) {
	var d EditorDocument
	if e := s.ownsStudioProject(ctx, u, project); e != nil {
		return d, e
	}
	var obj map[string]json.RawMessage
	if revision < 0 || len(body) > 750000 || json.Unmarshal(body, &obj) != nil || obj == nil || len(obj["scenes"]) == 0 || len(obj["metadata"]) == 0 || len(obj["version"]) == 0 {
		return d, ErrValidation
	}
	var updated time.Time
	var e error
	if revision == 0 {
		e = s.DB.QueryRowContext(ctx, `INSERT INTO cloud_editor_documents(project_id,revision,document) VALUES($1,1,$2::jsonb) ON CONFLICT(project_id) DO NOTHING RETURNING revision,document,updated_at`, project, []byte(body)).Scan(&d.Revision, &d.Document, &updated)
	} else {
		e = s.DB.QueryRowContext(ctx, `UPDATE cloud_editor_documents SET revision=revision+1,document=$2::jsonb,updated_at=now() WHERE project_id=$1 AND revision=$3 RETURNING revision,document,updated_at`, project, []byte(body), revision).Scan(&d.Revision, &d.Document, &updated)
	}
	if errors.Is(e, sql.ErrNoRows) {
		return d, ErrConflict
	}
	if e == nil {
		d.UpdatedAt = &updated
	}
	return d, e
}
func (s *Server) getEditorDocument(w http.ResponseWriter, r *http.Request) {
	v, e := s.Store.EditorDocument(r.Context(), currentUser(r), chi.URLParam(r, "project"))
	s.result(w, v, e)
}
func (s *Server) saveEditorDocument(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Revision int64           `json:"revision"`
		Document json.RawMessage `json:"document"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	v, e := s.Store.SaveEditorDocument(r.Context(), currentUser(r), chi.URLParam(r, "project"), b.Revision, b.Document)
	s.result(w, v, e)
}
