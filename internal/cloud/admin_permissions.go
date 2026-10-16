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
		"finance":    "billing crypto premium premium_payments revenue rewards api-docs openapi.json campaigns campaign-awards",
		"operations": "jobs usage budgets readiness capacity ai-readiness settings policies audit runtime-policy api-docs openapi.json discovery",
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
	if path == "/commercial-policy" || strings.HasPrefix(path, "/commercial-policy/") {
		return role == "finance" && ((method == "GET" && (path == "/commercial-policy" || path == "/commercial-policy/history" || path == "/commercial-policy/proposals")) || (method == "PUT" && path == "/commercial-policy") || (method == "POST" && (path == "/commercial-policy/preview" || adminPathMatches(path, "/commercial-policy/proposals/:id/approve"))))
	}
	if path == "/rewards/legacy-backing" || strings.HasPrefix(path, "/rewards/legacy-backing/") {
		return role == "finance" && ((method == "GET" && path == "/rewards/legacy-backing") || (method == "POST" && (path == "/rewards/legacy-backing" || adminPathMatches(path, "/rewards/legacy-backing/:id/approve"))))
	}
	if path == "/discovery" || strings.HasPrefix(path, "/discovery/") {
		return role == "operations" && ((method == "GET" && (path == "/discovery/policy" || path == "/discovery/history" || path == "/discovery/overrides" || path == "/discovery/preview" || path == "/discovery/index" || path == "/discovery/audience")) || (method == "PUT" && path == "/discovery/policy") || (method == "POST" && (path == "/discovery/overrides" || path == "/discovery/exclusions")))
	}
	if path == "/campaigns" || strings.HasPrefix(path, "/campaigns/") || strings.HasPrefix(path, "/campaign-awards/") {
		return role == "finance" && ((method == "GET" && (path == "/campaigns" || adminPathMatches(path, "/campaigns/:id/report") || adminPathMatches(path, "/campaigns/:id/awards"))) || (method == "POST" && (path == "/campaigns" || adminPathMatches(path, "/campaigns/:id/awards") || adminPathMatches(path, "/campaign-awards/:id/approve"))))
	}
	if strings.HasPrefix(path, "/rewards/revenue") {
		return role == "finance" && ((method == "GET" && (path == "/rewards/revenue" || adminPathMatches(path, "/rewards/revenue/stripe/:id/statement"))) || (method == "POST" && (path == "/rewards/revenue/evidence" || path == "/rewards/revenue/reconcile" || adminPathMatches(path, "/rewards/revenue/evidence/:id/approve"))))
	}
	if path == "/rewards/policy" || strings.HasPrefix(path, "/rewards/policy/") || path == "/rewards/health" || path == "/rewards/calculations" || strings.HasPrefix(path, "/rewards/calculations/") || path == "/rewards/reviews" || path == "/rewards/quality" || strings.HasPrefix(path, "/rewards/quality/") || path == "/rewards/calculate" || path == "/rewards/payout-runs" {
		return role == "finance" && ((method == "GET" && (path == "/rewards/policy" || path == "/rewards/policy/proposals" || path == "/rewards/policy/history" || adminPathMatches(path, "/rewards/calculations/:id") || path == "/rewards/health" || path == "/rewards/calculations" || path == "/rewards/reviews" || path == "/rewards/quality" || adminPathMatches(path, "/rewards/quality/:id") || path == "/rewards/payout-runs")) || (method == "PUT" && (path == "/rewards/policy" || adminPathMatches(path, "/rewards/quality/:id"))) || (method == "POST" && (path == "/rewards/calculate" || path == "/rewards/payout-runs" || adminPathMatches(path, "/rewards/policy/proposals/:id/approve"))))
	}
	if path == "/rewards/corrections" || strings.HasPrefix(path, "/rewards/corrections/") {
		return role == "finance" && ((method == "GET" && path == "/rewards/corrections") || method == "POST" && (path == "/rewards/corrections" || adminPathMatches(path, "/rewards/corrections/:id/approve")))
	}
	if adminPathMatches(path, "/rewards/users/:id") || adminPathMatches(path, "/rewards/payouts/:id/reconcile") {
		return role == "finance" && ((method == "GET" && adminPathMatches(path, "/rewards/users/:id")) || (method == "POST" && adminPathMatches(path, "/rewards/payouts/:id/reconcile")))
	}
	if path == "/rewards/corrections" || strings.HasPrefix(path, "/rewards/corrections/") {
		return role == "finance" && ((method == "GET" && path == "/rewards/corrections") || (method == "POST" && (path == "/rewards/corrections" || adminPathMatches(path, "/rewards/corrections/:id/approve"))))
	}
	if path == "/rewards/support" || strings.HasPrefix(path, "/rewards/support/") || path == "/rewards/appeals" || strings.HasPrefix(path, "/rewards/appeals/") {
		if role != "finance" {
			return false
		}
		if method == "GET" && (path == "/rewards/support" || path == "/rewards/appeals") {
			return true
		}
		if method == "POST" && (path == "/rewards/support" || strings.HasPrefix(path, "/rewards/support/") && strings.HasSuffix(path, "/approve") || strings.HasPrefix(path, "/rewards/support/claims/") && strings.HasSuffix(path, "/review") || strings.HasPrefix(path, "/rewards/appeals/") && strings.HasSuffix(path, "/resolve")) {
			return true
		}
		return false
	}
	if role == "finance" && strings.HasPrefix(path, "/rewards/") {
		return (method == "GET" && strings.HasPrefix(path, "/rewards/batches/")) || method == "POST" && (path == "/rewards/funding" || path == "/rewards/batches" || strings.HasSuffix(path, "/reconcile") || strings.HasSuffix(path, "/confirm") || strings.HasSuffix(path, "/hold"))
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
	for _, section := range []string{"users", "content", "reports", "jobs", "billing", "rewards", "discovery", "campaigns", "api-docs", "analytics", "readiness", "settings", "audit"} {
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

func adminPathMatches(path, pattern string) bool {
	a, b := strings.Split(path, "/"), strings.Split(pattern, "/")
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if b[i] == ":id" {
			if a[i] == "" {
				return false
			}
			continue
		}
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
