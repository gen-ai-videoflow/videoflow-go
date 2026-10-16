package cloud

import "strings"

func normalizedAdminRole(role string) string {
	if role == "moderator" {
		return "moderation"
	}
	return role
}
func adminCanRead(role, kind string) bool {
	role = normalizedAdminRole(role)
	if role == "owner" {
		return true
	}
	groups := map[string]string{
		"viewer":     "users content jobs reports usage audience",
		"support":    "users content jobs reports",
		"moderation": "users content reports audience",
		"finance":    "billing crypto premium premium_payments revenue",
		"operations": "jobs usage budgets readiness capacity ai-readiness settings policies audit runtime-policy",
	}
	return strings.Contains(" "+groups[role]+" ", " "+kind+" ")
}
func adminCanExport(role, kind string) bool {
	role = normalizedAdminRole(role)
	if !adminCanRead(role, kind) {
		return false
	}
	if role == "owner" {
		return true
	}
	if role == "finance" {
		return kind == "billing" || kind == "crypto" || kind == "premium" || kind == "premium_payments"
	}
	if role == "operations" {
		return kind == "jobs" || kind == "audit"
	}
	if role == "moderation" {
		return kind == "content" || kind == "reports"
	}
	return false
}
func adminAllowed(role, method, path string) bool {
	if role == "owner" {
		return true
	}
	role = normalizedAdminRole(role)
	if !strings.Contains(" viewer support moderation finance operations owner ", " "+role+" ") {
		return false
	}
	index := strings.Index(path, "/admin")
	if index < 0 {
		return false
	}
	path = path[index+len("/admin"):]
	if path == "/me" && method == "GET" {
		return true
	}
	if strings.Contains(path, "/export/") {
		kind := path[strings.LastIndex(path, "/")+1:]
		return method == "POST" && adminCanExport(role, kind)
	}
	if method == "GET" {
		if path == "/runtime-policy/versions" {
			return role == "operations"
		}
		if strings.HasPrefix(path, "/records/") {
			return adminCanRead(role, strings.TrimPrefix(path, "/records/"))
		}
		if strings.HasPrefix(path, "/users/") && strings.Contains(path, "/records/") {
			kind := path[strings.LastIndex(path, "/")+1:]
			return (role == "support" || role == "moderation" || role == "viewer") && (kind != "jobs" || role != "moderation")
		}
		return adminCanRead(role, strings.TrimPrefix(path, "/"))
	}
	if role == "moderation" {
		return method == "POST" && (strings.HasSuffix(path, "/block") || strings.HasSuffix(path, "/restore") || strings.HasSuffix(path, "/resolve"))
	}
	if role == "operations" {
		return (method == "PUT" && strings.HasPrefix(path, "/settings/")) || (strings.HasPrefix(path, "/policies/") && (method == "PUT" || method == "POST"))
	}
	return false
}
func adminSections(role string) []string {
	out := []string{"overview"}
	for _, section := range []string{"users", "content", "reports", "jobs", "billing", "analytics", "readiness", "settings", "audit"} {
		kind := section
		if kind == "analytics" {
			if adminCanRead(role, "usage") || adminCanRead(role, "revenue") || adminCanRead(role, "audience") {
				out = append(out, section)
			}
			continue
		}
		if adminCanRead(role, kind) {
			out = append(out, section)
		}
	}
	return out
}

func adminReadableKinds(role string) []string {
	out := []string{}
	for _, kind := range []string{"users", "content", "jobs", "reports", "audit", "billing", "crypto", "premium", "premium_payments", "usage", "budgets", "revenue", "audience", "policies"} {
		if adminCanRead(role, kind) {
			out = append(out, kind)
		}
	}
	return out
}
func adminExportKinds(role string) []string {
	out := []string{}
	for _, kind := range []string{"users", "content", "jobs", "reports", "audit", "billing", "crypto", "premium", "premium_payments"} {
		if adminCanExport(role, kind) {
			out = append(out, kind)
		}
	}
	return out
}
