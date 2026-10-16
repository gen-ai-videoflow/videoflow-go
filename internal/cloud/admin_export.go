package cloud

import (
	"context"
	"encoding/csv"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
)

func adminCSVCell(value any) string {
	if value == nil {
		return ""
	}
	if t, ok := value.(time.Time); ok {
		return t.UTC().Format(time.RFC3339Nano)
	}
	text := fmt.Sprint(value)
	if _, ok := value.(string); ok {
		trimmed := strings.TrimLeftFunc(text, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) })
		if strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "@") || strings.HasPrefix(text, "\t") || strings.HasPrefix(text, "\r") || strings.HasPrefix(text, "\n") {
			return "'" + text
		}
	}
	return text
}
func (s *Server) adminExport(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	role, _ := r.Context().Value(adminRoleContext{}).(string)
	if !adminCanExport(role, kind) {
		s.json(w, 403, map[string]string{"message": "Your role cannot export these records."})
		return
	}
	if _, ok := adminListSpecs[kind]; !ok {
		s.fail(w, ErrValidation)
		return
	}
	if !s.rate(w, r, "admin-export:"+currentUser(r).ID, 3, time.Minute, true) {
		return
	}
	var b struct {
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if strings.TrimSpace(b.Reason) == "" || len(b.Reason) > 1000 {
		s.fail(w, ErrValidation)
		return
	}
	p, e := adminPageInput(r)
	if e != nil {
		s.fail(w, e)
		return
	}
	p.Cursor = ""
	p.Limit = 100
	_, e = s.Store.DB.ExecContext(r.Context(), `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'admin_export_started',$2,$3)`, currentUser(r).ID, kind, b.Reason)
	if e != nil {
		s.fail(w, e)
		return
	}
	// Fetch before sending response headers so initial failures remain a normal JSON error.
	page, e := s.adminListPage(r.Context(), role, kind, p)
	if e != nil {
		s.fail(w, e)
		return
	}
	columns := []string{}
	if len(page.Items) > 0 {
		for key := range page.Items[0] {
			columns = append(columns, key)
		}
		sort.Strings(columns)
	} else {
		columns = []string{"no_records"}
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="videoflow-`+kind+`.csv"`)
	w.Header().Set("X-Export-Limit", "1000")
	w.Header().Set("Cache-Control", "no-store")
	writer := csv.NewWriter(w)
	e = writer.Write(columns)
	count := 0
	complete := false
	for e == nil {
		for _, record := range page.Items {
			if count >= 1000 {
				break
			}
			line := make([]string, len(columns))
			for i, key := range columns {
				line[i] = adminCSVCell(record[key])
			}
			if e = writer.Write(line); e != nil {
				break
			}
			count++
		}
		writer.Flush()
		if writer.Error() != nil {
			e = writer.Error()
			break
		}
		if flush, ok := w.(http.Flusher); ok {
			flush.Flush()
		}
		if page.NextCursor == "" {
			complete = true
			break
		}
		if count >= 1000 {
			break
		}
		p.Cursor = page.NextCursor
		page, e = s.adminListPage(r.Context(), role, kind, p)
	}
	writer.Flush()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
	defer cancel()
	action := "admin_export_completed"
	if e != nil {
		action = "admin_export_failed"
	}
	_, _ = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,$2,$3,$4)`, currentUser(r).ID, action, kind, fmt.Sprintf("rows=%d; complete=%t; hard_limit=1000", count, complete))
}
