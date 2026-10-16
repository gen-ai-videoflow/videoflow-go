package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSponsorCampaignFundingAcceptanceAndPaidMeasurement(t *testing.T) {
	s, creator, _ := securityTestServer(t)
	_, viewer, _ := securityTestServer(t)
	ctx := context.Background()
	source := creator.ID + "-advertising"
	video := creator.ID + "-sponsor"
	campaign := SponsorCampaign{ID: creator.ID + "-campaign", Title: "Reviewed sponsor", Brief: "Display sponsored disclosure", TermsVersion: "sponsor-v1", VideoID: video, SourceID: source, BudgetCents: 100, StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(time.Hour)}
	if e := s.Store.RecordRewardFunding(ctx, RewardFunding{ID: source, Reference: source, Kind: "advertising", Cents: 100, EvidenceReference: "settled-invoice-fixture", ProposedBy: creator.ID, ReconciledBy: viewer.ID}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_videos(id,channel_id,title,visibility,audience,status,published_at)VALUES($1,$2,'Sponsored fixture','public','free','published',now())`, video, creator.ChannelID); e != nil {
		t.Fatal(e)
	}
	if e := s.Store.createSponsorCampaign(ctx, campaign, viewer.ID, "approved invoice and brief"); e != nil {
		t.Fatal(e)
	}
	over := campaign
	over.ID += "-over"
	over.BudgetCents = 101
	if e := s.Store.createSponsorCampaign(ctx, over, viewer.ID, "overspend attempt"); e == nil {
		t.Fatal("overspend accepted")
	}
	request := func(user User, body any, handler func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/", bytes.NewReader(b))
		route := chi.NewRouteContext()
		route.URLParams.Add("id", campaign.ID)
		r = r.WithContext(context.WithValue(context.WithValue(ctx, userContext{}, user), chi.RouteCtxKey, route))
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	if w := request(viewer, map[string]string{"terms_version": "sponsor-v1"}, s.acceptSponsorCampaign); w.Code == 200 {
		t.Fatal("nonowner accepted terms")
	}
	if w := request(viewer, nil, s.beginSponsoredWatch); w.Code == 200 {
		t.Fatal("unaccepted campaign delivered")
	}
	if w := request(creator, map[string]string{"terms_version": "sponsor-v1"}, s.acceptSponsorCampaign); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := request(viewer, nil, s.beginSponsoredWatch)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct {
		ID        string `json:"id"`
		Sponsored bool   `json:"sponsored"`
	}
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || !out.Sponsored {
		t.Fatal(w.Body.String())
	}
	var provenance string
	if e := s.Store.DB.QueryRowContext(ctx, `SELECT traffic_source FROM cloud_watch_sessions WHERE id=$1`, out.ID).Scan(&provenance); e != nil || provenance != "paid" {
		t.Fatal(provenance, e)
	}
	if _, e := s.Store.DB.ExecContext(ctx, `UPDATE cloud_watch_sessions SET last_event_at=now()-interval '15 seconds',started_at=now()-interval '15 seconds' WHERE id=$1`, out.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Store.DB.ExecContext(ctx, `UPDATE cloud_watch_clocks SET last_at=now()-interval '15 seconds' WHERE user_id=$1`, viewer.ID); e != nil {
		t.Fatal(e)
	}
	seconds, e := s.Store.RecordWatch(ctx, viewer, out.ID, out.ID+"-event", 10)
	if e != nil || seconds != 10 {
		t.Fatal(seconds, e)
	}
	var organic, paid int64
	if e = s.Store.DB.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE ws.traffic_source='organic'),count(*) FILTER(WHERE ws.traffic_source='paid') FROM cloud_watch_events e JOIN cloud_watch_sessions ws ON ws.id=e.session_id WHERE ws.id=$1`, out.ID).Scan(&organic, &paid); e != nil || organic != 0 || paid != 1 {
		t.Fatal(organic, paid, e)
	}
}

func TestSponsorCampaignBudgetApprovalAndRewardIsolation(t *testing.T) {
	s, u, _ := securityTestServer(t)
	_, approver, _ := securityTestServer(t)
	ctx := context.Background()
	source := u.ID + "-ad-source"
	video := u.ID + "-ad-video"
	campaign := u.ID + "-ad-campaign"
	award := u.ID + "-award"
	batch := u.ID + "-batch"
	if e := s.Store.RecordRewardFunding(ctx, RewardFunding{ID: source, Reference: source, Kind: "advertising", Cents: 100, EvidenceReference: "invoice", ProposedBy: u.ID, ReconciledBy: approver.ID}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_videos(id,channel_id,title,visibility,audience,status)VALUES($1,$2,'reviewed delivery','public','free','published')`, video, u.ChannelID); e != nil {
		t.Fatal(e)
	}
	c := SponsorCampaign{ID: campaign, Title: "Funded", Brief: "Approved terms", TermsVersion: "v1", VideoID: video, SourceID: source, BudgetCents: 100, StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour)}
	if e := s.Store.createSponsorCampaign(ctx, c, approver.ID, "reviewed invoice"); e != nil {
		t.Fatal(e)
	}
	for _, q := range []struct {
		s string
		a []any
	}{
		{`INSERT INTO cloud_sponsor_acceptances(campaign_id,user_id,terms_version)VALUES($1,$2,'v1')`, []any{campaign, u.ID}},
		{`INSERT INTO cloud_sponsor_awards(id,campaign_id,cents,evidence_reference,proposed_by,approved_by,approved_at)VALUES($1,$2,100,'actual delivered campaign evidence',$3,$4,now())`, []any{award, campaign, u.ID, approver.ID}},
		{`INSERT INTO cloud_reward_batches(id,day,source_id,policy_version,input_hash,evidence_reference,total_cents,proposed_by)SELECT $1,current_date-1,$2,version,'hash','campaign evidence',100,$3 FROM cloud_reward_policies WHERE version='pilot-v1' LIMIT 1`, []any{batch, source, u.ID}},
		{`INSERT INTO cloud_reward_allocations(id,batch_id,video_id,user_id,cents)VALUES($1,$2,$3,$4,100)`, []any{batch + "-allocation", batch, video, u.ID}},
	} {
		if _, e := s.Store.DB.ExecContext(ctx, q.s, q.a...); e != nil {
			t.Fatal(e)
		}
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if e = s.Store.ValidateCampaignRewardDraftTx(ctx, tx, batch, source, "", "", 100); e == nil {
		t.Fatal("campaign source entered organic draft")
	}
	if e = s.Store.ValidateCampaignRewardDraftTx(ctx, tx, batch, source, campaign, award, 101); e == nil {
		t.Fatal("award amount changed")
	}
	if e = s.Store.ValidateCampaignRewardDraftTx(ctx, tx, batch, source, campaign, award, 100); e != nil {
		t.Fatal(e)
	}
	if e = s.Store.ReserveCampaignAwardTx(ctx, tx, batch); e != nil {
		t.Fatal(e)
	}
	if e = reserveRewardFunding(ctx, tx, source, 100); e != nil {
		t.Fatal(e)
	}
	if e = s.Store.ReserveCampaignAwardTx(ctx, tx, batch); e == nil {
		t.Fatal("award reserved twice")
	}
	if e = reserveRewardFunding(ctx, tx, source, 1); e == nil {
		t.Fatal("source overdrawn")
	}
}
