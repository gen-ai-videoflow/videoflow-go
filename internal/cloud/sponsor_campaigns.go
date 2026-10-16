package cloud

import (
	"context"
	"database/sql"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"strings"
	"time"
)

type SponsorCampaign struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Brief        string    `json:"brief"`
	TermsVersion string    `json:"terms_version"`
	VideoID      string    `json:"video_id"`
	SourceID     string    `json:"source_id"`
	BudgetCents  int64     `json:"budget_cents"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
}

func (s *Server) campaignAdminRoutes(r chi.Router) {
	r.Get("/campaigns", s.listSponsorCampaigns)
	r.Post("/campaigns", s.createSponsorCampaign)
	r.Get("/campaigns/{id}/report", s.sponsorReport)
	r.Post("/campaigns/{id}/awards", s.proposeSponsorAward)
	r.Get("/campaigns/{id}/awards", s.listSponsorAwards)
	r.Post("/campaign-awards/{id}/approve", s.approveSponsorAward)
}
func (s *Server) campaignCreatorRoutes(r chi.Router) {
	r.Get("/creator/campaigns", s.creatorSponsorCampaigns)
	r.Post("/creator/campaigns/{id}/accept", s.acceptSponsorCampaign)
	r.Post("/campaigns/{id}/watch", s.beginSponsoredWatch)
}
func (s *Store) createSponsorCampaign(ctx context.Context, c SponsorCampaign, actor, reason string) error {
	if c.ID == "" || strings.TrimSpace(c.Title) == "" || len(c.Title) > 200 || len(c.Brief) < 3 || len(c.Brief) > 10000 || c.TermsVersion == "" || c.BudgetCents < 1 || c.BudgetCents > 100000000 || !c.EndsAt.After(c.StartsAt) || c.EndsAt.Sub(c.StartsAt) > 90*24*time.Hour || !c.EndsAt.After(time.Now()) || len(strings.TrimSpace(reason)) < 3 {
		return ErrValidation
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var available int64
	e = tx.QueryRowContext(ctx, `SELECT cents-reserved_cents FROM cloud_reward_sources WHERE id=$1 AND kind='advertising' AND proposed_by<>reconciled_by FOR UPDATE`, c.SourceID).Scan(&available)
	if e == sql.ErrNoRows {
		return ErrValidation
	}
	if e != nil {
		return e
	}
	if available < c.BudgetCents {
		return ErrInsufficientCredits
	}
	var valid bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_videos v JOIN cloud_channels ch ON ch.id=v.channel_id WHERE v.id=$1 AND v.status='published' AND v.visibility='public' AND NOT EXISTS(SELECT 1 FROM cloud_user_restrictions ur WHERE ur.user_id=ch.user_id AND ur.suspended))`, c.VideoID).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return ErrValidation
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_sponsor_campaigns(id,title,brief,terms_version,video_id,source_id,budget_cents,starts_at,ends_at,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, c.ID, c.Title, c.Brief, c.TermsVersion, c.VideoID, c.SourceID, c.BudgetCents, c.StartsAt, c.EndsAt, actor)
	if e == nil {
		_, e = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'create_sponsor_campaign',$2,$3)`, actor, c.ID, reason)
	}
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Server) createSponsorCampaign(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Campaign SponsorCampaign `json:"campaign"`
		Reason   string          `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	b.Campaign.ID = uuid.NewString()
	s.result(w, b.Campaign, s.Store.createSponsorCampaign(r.Context(), b.Campaign, currentUser(r).ID, b.Reason))
}
func (s *Server) listSponsorCampaigns(w http.ResponseWriter, r *http.Request) {
	s.sponsorCampaignList(w, r, false)
}
func (s *Server) creatorSponsorCampaigns(w http.ResponseWriter, r *http.Request) {
	s.sponsorCampaignList(w, r, true)
}
func (s *Server) sponsorCampaignList(w http.ResponseWriter, r *http.Request, owned bool) {
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT sc.id,sc.title,sc.brief,sc.terms_version,sc.video_id,sc.source_id,sc.budget_cents,sc.starts_at,sc.ends_at,EXISTS(SELECT 1 FROM cloud_sponsor_acceptances a WHERE a.campaign_id=sc.id) FROM cloud_sponsor_campaigns sc JOIN cloud_videos v ON v.id=sc.video_id JOIN cloud_channels c ON c.id=v.channel_id WHERE NOT $1 OR c.user_id=$2 ORDER BY sc.created_at DESC LIMIT 100`, owned, currentUser(r).ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var c SponsorCampaign
		var accepted bool
		if e = rows.Scan(&c.ID, &c.Title, &c.Brief, &c.TermsVersion, &c.VideoID, &c.SourceID, &c.BudgetCents, &c.StartsAt, &c.EndsAt, &accepted); e != nil {
			s.fail(w, e)
			return
		}
		out = append(out, map[string]any{"campaign": c, "accepted": accepted})
	}
	s.result(w, map[string]any{"items": out}, rows.Err())
}
func (s *Server) acceptSponsorCampaign(w http.ResponseWriter, r *http.Request) {
	var b struct {
		TermsVersion string `json:"terms_version"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	res, e := s.Store.DB.ExecContext(r.Context(), `INSERT INTO cloud_sponsor_acceptances(campaign_id,user_id,terms_version)SELECT sc.id,$2,sc.terms_version FROM cloud_sponsor_campaigns sc JOIN cloud_videos v ON v.id=sc.video_id JOIN cloud_channels c ON c.id=v.channel_id WHERE sc.id=$1 AND c.user_id=$2 AND sc.terms_version=$3 AND sc.ends_at>now() ON CONFLICT DO NOTHING`, chi.URLParam(r, "id"), currentUser(r).ID, b.TermsVersion)
	if e == nil {
		n, _ := res.RowsAffected()
		if n == 0 {
			e = ErrConflict
		}
	}
	s.result(w, map[string]bool{"accepted": e == nil}, e)
}
func (s *Server) beginSponsoredWatch(w http.ResponseWriter, r *http.Request) {
	if !s.rate(w, r, "watch-session:"+currentUser(r).ID, 30, time.Minute, false) {
		return
	}
	campaign := chi.URLParam(r, "id")
	var video string
	e := s.Store.DB.QueryRowContext(r.Context(), `SELECT sc.video_id FROM cloud_sponsor_campaigns sc JOIN cloud_sponsor_acceptances a ON a.campaign_id=sc.id WHERE sc.id=$1 AND now() BETWEEN sc.starts_at AND sc.ends_at`, campaign).Scan(&video)
	if e != nil {
		s.fail(w, ErrNotFound)
		return
	}
	v, e := s.Store.Video(r.Context(), video, currentUser(r).ID)
	if e != nil || !s.Store.CanWatch(r.Context(), currentUser(r), v) {
		s.fail(w, ErrUnauthorized)
		return
	}
	id, e := s.Store.BeginWatch(r.Context(), currentUser(r), video, currentUser(r).ID)
	if e == nil {
		_, e = s.Store.DB.ExecContext(r.Context(), `UPDATE cloud_watch_sessions SET traffic_source='paid',campaign_id=$2 WHERE id=$1`, id, campaign)
	}
	s.result(w, map[string]any{"id": id, "sponsored": true, "disclosure": "Sponsored", "campaign_id": campaign}, e)
}
func (s *Server) proposeSponsorAward(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Cents             int64  `json:"cents"`
		EvidenceReference string `json:"evidence_reference"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if b.Cents < 1 || len(strings.TrimSpace(b.EvidenceReference)) < 3 || len(b.EvidenceReference) > 1000 {
		s.fail(w, ErrValidation)
		return
	}
	id := uuid.NewString()
	res, e := s.Store.DB.ExecContext(r.Context(), `WITH added AS(INSERT INTO cloud_sponsor_awards(id,campaign_id,cents,evidence_reference,proposed_by)SELECT $1,sc.id,$3,$4,$5 FROM cloud_sponsor_campaigns sc JOIN cloud_sponsor_acceptances a ON a.campaign_id=sc.id WHERE sc.id=$2 AND sc.budget_cents>=$3 RETURNING id)INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)SELECT $5,'propose_sponsor_award',id,$4 FROM added`, id, chi.URLParam(r, "id"), b.Cents, b.EvidenceReference, currentUser(r).ID)
	if e == nil {
		n, _ := res.RowsAffected()
		if n == 0 {
			e = ErrValidation
		}
	}
	s.result(w, map[string]string{"id": id}, e)
}
func (s *Server) approveSponsorAward(w http.ResponseWriter, r *http.Request) {
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	id := chi.URLParam(r, "id")
	var campaign, proposer string
	var cents int64
	e = tx.QueryRowContext(r.Context(), `SELECT campaign_id,proposed_by,cents FROM cloud_sponsor_awards WHERE id=$1 AND approved_by IS NULL FOR UPDATE`, id).Scan(&campaign, &proposer, &cents)
	if e == nil && proposer == currentUser(r).ID {
		e = ErrUnauthorized
	}
	var budget, committed int64
	if e == nil {
		e = tx.QueryRowContext(r.Context(), `SELECT budget_cents FROM cloud_sponsor_campaigns WHERE id=$1 FOR UPDATE`, campaign).Scan(&budget)
	}
	if e == nil {
		e = tx.QueryRowContext(r.Context(), `SELECT coalesce(sum(cents),0) FROM cloud_sponsor_awards WHERE campaign_id=$1 AND approved_by IS NOT NULL`, campaign).Scan(&committed)
	}
	if e == nil && committed+cents > budget {
		e = ErrInsufficientCredits
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `UPDATE cloud_sponsor_awards SET approved_by=$2,approved_at=now() WHERE id=$1`, id, currentUser(r).ID)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'approve_sponsor_award',$2,'Reviewed campaign delivery evidence; pending reward-batch confirmation')`, currentUser(r).ID, id)
	}
	if e == nil {
		e = tx.Commit()
	}
	s.result(w, map[string]bool{"approved": e == nil}, e)
}

// ValidateCampaignRewardDraftTx isolates campaign funding and binds a reviewed award to exactly one draft.
// Call inside DraftRewardBatch after inserting batch and allocations, before commit.
func (s *Store) ValidateCampaignRewardDraftTx(ctx context.Context, tx *sql.Tx, batchID, source, campaignID, awardID string, cents int64) error {
	var campaign string
	e := tx.QueryRowContext(ctx, `SELECT id FROM cloud_sponsor_campaigns WHERE source_id=$1 FOR UPDATE`, source).Scan(&campaign)
	if e == sql.ErrNoRows {
		if campaignID != "" || awardID != "" {
			return ErrValidation
		}
		return nil
	}
	if e != nil {
		return e
	}
	if campaignID != campaign || awardID == "" {
		return ErrValidation
	}
	var valid bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_sponsor_awards a JOIN cloud_sponsor_campaigns sc ON sc.id=a.campaign_id JOIN cloud_sponsor_acceptances ac ON ac.campaign_id=sc.id WHERE a.id=$1 AND a.campaign_id=$2 AND a.approved_by IS NOT NULL AND a.cents=$3 AND (a.batch_id IS NULL OR a.batch_id=$4) AND NOT EXISTS(SELECT 1 FROM cloud_reward_allocations ra WHERE ra.batch_id=$4 AND ra.video_id<>sc.video_id))`, awardID, campaign, cents, batchID).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return ErrValidation
	}
	res, e := tx.ExecContext(ctx, `UPDATE cloud_sponsor_awards SET batch_id=$2 WHERE id=$1 AND (batch_id IS NULL OR batch_id=$2)`, awardID, batchID)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	return nil
}

// ReserveCampaignAwardTx runs before reserveRewardFunding in the same confirmation transaction.
func (s *Store) ReserveCampaignAwardTx(ctx context.Context, tx *sql.Tx, batchID string) error {
	var campaign string
	var cents int64
	e := tx.QueryRowContext(ctx, `SELECT sc.id,b.total_cents FROM cloud_reward_batches b JOIN cloud_sponsor_campaigns sc ON sc.source_id=b.source_id WHERE b.id=$1 FOR UPDATE OF sc`, batchID).Scan(&campaign, &cents)
	if e == sql.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	res, e := tx.ExecContext(ctx, `UPDATE cloud_sponsor_awards SET confirmed_at=now() WHERE batch_id=$1 AND campaign_id=$2 AND cents=$3 AND approved_by IS NOT NULL AND confirmed_at IS NULL`, batchID, campaign, cents)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	return nil
}
func (s *Server) sponsorReport(w http.ResponseWriter, r *http.Request) {
	var raw, accepted, sessions, awarded int64
	e := s.Store.DB.QueryRowContext(r.Context(), `SELECT coalesce(sum(e.seconds),0),coalesce(sum(e.seconds) FILTER(WHERE x.event_id IS NULL),0),count(DISTINCT ws.id),(SELECT coalesce(sum(cents),0) FROM cloud_sponsor_awards WHERE campaign_id=$1 AND approved_by IS NOT NULL) FROM cloud_watch_sessions ws LEFT JOIN cloud_watch_events e ON e.session_id=ws.id LEFT JOIN cloud_watch_exclusions x ON x.event_id=e.id WHERE ws.campaign_id=$1 AND ws.traffic_source='paid'`, chi.URLParam(r, "id")).Scan(&raw, &accepted, &sessions, &awarded)
	s.result(w, map[string]any{"measured_paid_seconds": raw, "accepted_paid_seconds": accepted, "sessions": sessions, "approved_obligations_cents": awarded, "settlement": "Existing independently reconciled advertising source only; no provider settlement adapter activated."}, e)
}

func (s *Server) listSponsorAwards(w http.ResponseWriter, r *http.Request) {
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT id,cents,evidence_reference,proposed_by,coalesce(approved_by,''),coalesce(batch_id,''),confirmed_at IS NOT NULL FROM cloud_sponsor_awards WHERE campaign_id=$1 ORDER BY created_at DESC LIMIT 100`, chi.URLParam(r, "id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, evidence, proposed, approved, batch string
		var cents int64
		var confirmed bool
		if e = rows.Scan(&id, &cents, &evidence, &proposed, &approved, &batch, &confirmed); e != nil {
			s.fail(w, e)
			return
		}
		out = append(out, map[string]any{"id": id, "cents": cents, "evidence_reference": evidence, "proposed_by": proposed, "approved_by": approved, "batch_id": batch, "confirmed": confirmed})
	}
	s.result(w, map[string]any{"items": out}, rows.Err())
}
