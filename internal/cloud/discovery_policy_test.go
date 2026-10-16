package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestDiscoveryPolicyValidation(t *testing.T) {
	p := defaultDiscoveryPolicy()
	if p.validate() != nil {
		t.Fatal("default invalid")
	}
	for _, change := range []func(*DiscoveryPolicy){func(p *DiscoveryPolicy) { p.CandidateLimit = 99999 }, func(p *DiscoveryPolicy) { p.WatchCap = -1 }, func(p *DiscoveryPolicy) { p.TitleWeight = 0 }, func(p *DiscoveryPolicy) { p.DescriptionWeight = 10001 }} {
		q := p
		change(&q)
		if q.validate() == nil {
			t.Fatal("unsafe policy accepted")
		}
	}
	for _, q := range []string{"中文", "汉字 weather", "かな", "한국"} {
		if !discoveryCJK(q) {
			t.Fatal(q)
		}
	}
	if discoveryCJK("ordinary words") {
		t.Fatal("Latin misclassified")
	}
}
func TestDiscoveryPolicyPostgresFeedPrivacyAndCJK(t *testing.T) {
	s, u, token := securityTestServer(t)
	ctx := context.Background()
	_, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_videos(id,channel_id,title,visibility,audience,status,format,published_at) VALUES($1,$2,'中文测试影片','public','free','published','standard',now()),($3,$2,'中文测试私密','private','free','published','standard',now()),($4,$2,'中文测试付费','public','members','published','standard',now())`, u.ID+"-cjk", u.ChannelID, u.ID+"-private", u.ID+"-premium")
	if e != nil {
		t.Fatal(e)
	}
	w := securityRequest(s, "GET", "/feed?format=standard&q=%E4%B8%AD%E6%96%87%E6%B5%8B%E8%AF%95", "", nil)
	var result struct {
		Items []Video `json:"items"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	found := false
	for _, v := range result.Items {
		if v.Visibility != "public" || v.Audience != "free" {
			t.Fatalf("leaked inaccessible result %+v", v)
		}
		if v.ID == u.ID+"-cjk" {
			found = true
		}
	}
	if !found {
		t.Fatal("CJK result missing")
	}

	_ = token
	p, e := s.Store.discoveryPolicy(ctx)
	if e != nil || p.CandidateLimit != 2000 {
		t.Fatal(p, e)
	}
}

func TestDiscoveryPolicyExclusionPreservesEvidenceAndLegacyViews(t *testing.T) {
	s, u, _ := securityTestServer(t)
	ctx := context.Background()
	video := u.ID + "-correct"
	session := u.ID + "-session"
	event := u.ID + "-event"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO cloud_videos(id,channel_id,title,visibility,audience,status,views,watch_seconds)VALUES($1,$2,'correction','public','free','published',8,15)`, []any{video, u.ChannelID}},
		{`INSERT INTO cloud_watch_sessions(id,video_id,user_id,viewer_hash)VALUES($1,$2,$3,'hash')`, []any{session, video, u.ID}},
		{`INSERT INTO cloud_watch_events(id,user_id,video_id,seconds,session_id)VALUES($1,$2,$3,15,$4)`, []any{event, u.ID, video, session}},
		{`INSERT INTO cloud_view_qualifications(video_id,viewer_hash,day,session_id,threshold_seconds)VALUES($1,'hash',current_date,$2,10)`, []any{video, session}},
	} {
		if _, e := s.Store.DB.ExecContext(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	for i := 0; i < 2; i++ {
		body, _ := json.Marshal(map[string]string{"event_id": event, "reason": "confirmed invalid replay"})
		r := httptest.NewRequest("POST", "/", bytes.NewReader(body)).WithContext(context.WithValue(ctx, userContext{}, u))
		w := httptest.NewRecorder()
		s.excludeWatchEvent(w, r)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	var views, seconds, original int64
	if e := s.Store.DB.QueryRowContext(ctx, `SELECT views,watch_seconds,(SELECT seconds FROM cloud_watch_events WHERE id=$2) FROM cloud_videos WHERE id=$1`, video, event).Scan(&views, &seconds, &original); e != nil {
		t.Fatal(e)
	}
	if views != 7 || seconds != 0 || original != 15 {
		t.Fatal(views, seconds, original)
	}
}

func TestDiscoveryPolicySearchCurationAndExpiry(t *testing.T) {
	s, u, token := securityTestServer(t)
	ctx := context.Background()
	video := u.ID + "-pin"
	query := "search-" + u.ID
	p := defaultDiscoveryPolicy()
	p.Synonyms = map[string]string{"alias-" + u.ID: query}
	data, _ := json.Marshal(p)
	_, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_discovery_policies(revision,policy,actor,reason)SELECT coalesce(max(revision),0)+1,$1,$2,'search evaluation fixture' FROM cloud_discovery_policies`, data, u.ID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_videos(id,channel_id,title,visibility,audience,status,published_at)VALUES($1,$2,'No lexical match','public','free','published',now())`, video, u.ChannelID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_discovery_overrides(id,video_id,weight,expires_at,reason,actor,search_query)VALUES($1,$2,1000000,now()+interval '1 hour','evaluated pin',$3,$4)`, video, video, u.ID, query)
	if e != nil {
		t.Fatal(e)
	}
	w := securityRequest(s, "GET", "/feed?format=standard&q=alias-"+u.ID, "", nil)
	var out struct {
		Items []Video `json:"items"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Items) != 1 || out.Items[0].ID != video {
		t.Fatalf("pin/synonym %d %s", w.Code, w.Body.String())
	}
	if _, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_viewer_preferences(user_id,channel_id,hidden)VALUES($1,$2,true)`, u.ID, u.ChannelID); e != nil {
		t.Fatal(e)
	}
	hidden := securityRequest(s, "GET", "/feed?format=standard&q=alias-"+u.ID, token, nil)
	if hidden.Code != 200 || json.Unmarshal(hidden.Body.Bytes(), &out) != nil || len(out.Items) != 0 {
		t.Fatal("pin bypassed hidden preferences", hidden.Body.String())
	}
	if _, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_user_restrictions(user_id,suspended,reason,updated_by)VALUES($1,true,'moderation fixture','fixture')`, u.ID); e != nil {
		t.Fatal(e)
	}
	blocked := securityRequest(s, "GET", "/feed?format=standard&q=alias-"+u.ID, "", nil)
	if blocked.Code != 200 || json.Unmarshal(blocked.Body.Bytes(), &out) != nil || len(out.Items) != 0 {
		t.Fatal("pin exposed suspended creator", blocked.Body.String())
	}
	_, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_discovery_overrides(id,video_id,weight,expires_at,reason,actor,search_query)VALUES($1||'-expired',$1,1000000,now()-interval '1 second','expired evaluation fixture',$2,$3)`, video, u.ID, query+"-expired")
	if e != nil {
		t.Fatal(e)
	}
	w = securityRequest(s, "GET", "/feed?format=standard&q="+query+"-expired", "", nil)
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Items) != 0 {
		t.Fatal("expired pin returned", w.Body.String())
	}
}

func TestDiscoveryPolicyProtectedRegistrationAndLegacyCompatibility(t *testing.T) {
	s, u, token := securityTestServer(t)
	ctx := context.Background()
	for _, path := range []string{"/admin/discovery/policy", "/admin/campaigns", "/admin/rewards/policy", "/admin/rewards/revenue"} {
		if w := securityRequest(s, "GET", path, "", nil); w.Code != 401 {
			t.Fatal(path, w.Code)
		}
	}
	if _, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_admin_roles(user_id,role,granted_by)VALUES($1,'operations','fixture')`, u.ID); e != nil {
		t.Fatal(e)
	}
	if w := securityRequest(s, "GET", "/admin/discovery/policy", token, nil); w.Code != 403 {
		t.Fatal("MFA not required", w.Code)
	}
	if _, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_totp(user_id,secret_ciphertext,enabled)VALUES($1,'fixture',true)`, u.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Store.DB.ExecContext(ctx, `UPDATE cloud_sessions SET mfa_verified_at=now() WHERE token_hash=$1`, tokenHash(token)); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/admin/discovery/policy", "/admin/discovery/history", "/admin/discovery/index", "/admin/discovery/overrides", "/admin/discovery/preview", "/admin/discovery/audience?channel_id=" + u.ChannelID} {
		w := securityRequest(s, "GET", path, token, nil)
		if w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	if w := securityRequest(s, "GET", "/admin/campaigns", token, nil); w.Code != 403 {
		t.Fatal("operations finance access", w.Code)
	}
	if _, e := s.Store.DB.ExecContext(ctx, `UPDATE cloud_admin_roles SET role='finance' WHERE user_id=$1`, u.ID); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/admin/campaigns", "/admin/rewards/policy", "/admin/rewards/policy/proposals", "/admin/rewards/health", "/admin/rewards/calculations", "/admin/rewards/reviews", "/admin/rewards/quality", "/admin/rewards/revenue"} {
		w := securityRequest(s, "GET", path, token, nil)
		if w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	w := securityRequest(s, "GET", "/feed?q=unmatched-query-"+u.ID, "", nil)
	var legacy []Video
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &legacy) != nil {
		t.Fatal("legacy array compatibility", w.Code, w.Body.String())
	}
}
func TestDiscoveryPolicyPreciseAdminPermissions(t *testing.T) {
	for _, test := range []struct {
		role, method, path string
		allowed            bool
	}{{"operations", "GET", "/admin/discovery/index", true}, {"operations", "POST", "/admin/discovery/index", false}, {"finance", "POST", "/admin/campaigns/a/awards", true}, {"finance", "PUT", "/admin/campaigns/a/awards", false}, {"moderation", "POST", "/admin/campaign-awards/a/approve", false}, {"finance", "GET", "/admin/rewards/quality", true}, {"finance", "PUT", "/admin/rewards/quality/a", true}, {"finance", "POST", "/admin/rewards/revenue/evidence/a/approve", true}, {"finance", "DELETE", "/admin/rewards/revenue/evidence/a/approve", false}, {"finance", "POST", "/admin/rewards/revenue/arbitrary/approve", false}, {"support", "GET", "/admin/campaigns", false}} {
		if got := adminAllowed(test.role, test.method, test.path); got != test.allowed {
			t.Fatal(test, got)
		}
	}
}
