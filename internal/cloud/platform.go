package cloud

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"time"
)

var ErrNotFound = errors.New("not found")
var ErrInsufficientCredits = errors.New("insufficient credits")

type Project struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}
type CreditEntry struct {
	ID          string    `json:"id"`
	Amount      int64     `json:"amount"`
	Kind        string    `json:"kind"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}
type CreditBalance struct {
	Debt         int64         `json:"debt"`
	Restricted   bool          `json:"restricted"`
	Available    int64         `json:"available"`
	Reserved     int64         `json:"reserved"`
	Transactions []CreditEntry `json:"transactions"`
}
type QuoteInput struct {
	ProjectID   string `json:"project_id"`
	Prompt      string `json:"prompt"`
	Duration    int    `json:"duration"`
	AspectRatio string `json:"aspect_ratio"`
}
type Quote struct {
	ID        string    `json:"id"`
	Credits   int64     `json:"credits"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Job struct {
	CanCancel   bool      `json:"can_cancel"`
	ProjectID   string    `json:"project_id"`
	Kind        string    `json:"kind"`
	ID          string    `json:"id"`
	WorkspaceID string    `json:"-"`
	QuoteID     string    `json:"quote_id"`
	Status      string    `json:"status"`
	Credits     int64     `json:"credits"`
	ProviderID  string    `json:"-"`
	OutputKey   string    `json:"-"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const jobCols = "id,workspace_id,quote_id,status,credits,provider_id,output_key,error,created_at,updated_at,(SELECT project_id FROM cloud_quotes WHERE id=cloud_jobs.quote_id),COALESCE((SELECT CASE WHEN kind='storyboard' THEN 'pipeline' ELSE 'assembly' END FROM cloud_pipeline_plans WHERE quote_id=cloud_jobs.quote_id),'video'),(status IN ('queued','running') AND NOT EXISTS(SELECT 1 FROM cloud_provider_admissions WHERE job_id=cloud_jobs.id))"

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.WorkspaceID, &j.QuoteID, &j.Status, &j.Credits, &j.ProviderID, &j.OutputKey, &j.Error, &j.CreatedAt, &j.UpdatedAt, &j.ProjectID, &j.Kind, &j.CanCancel)
	return j, err
}
func (s *Store) CreateProject(ctx context.Context, u User, name, description string) (Project, error) {
	name = strings.TrimSpace(name)
	if len(name) < 1 || len(name) > 120 || len(description) > 4000 {
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
	p := Project{ID: uuid.NewString(), WorkspaceID: u.WorkspaceID, Name: name, Description: description}
	if err = tx.QueryRowContext(ctx, `INSERT INTO cloud_projects(id,workspace_id,name,description)VALUES($1,$2,$3,$4)RETURNING created_at`, p.ID, p.WorkspaceID, p.Name, p.Description).Scan(&p.CreatedAt); err != nil {
		return Project{}, err
	}
	return p, tx.Commit()
}
func (s *Store) Projects(ctx context.Context, u User) ([]Project, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,workspace_id,name,description,created_at FROM cloud_projects WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 100`, u.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Project{}
	for rows.Next() {
		var p Project
		if err = rows.Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
func (s *Store) GrantCredits(ctx context.Context, workspace, reference string, amount int64, description string) error {
	if amount <= 0 {
		return ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = grantCreditsTx(ctx, tx, workspace, reference, amount, description); err != nil {
		return err
	}
	return tx.Commit()
}
func grantCreditsTx(ctx context.Context, tx *sql.Tx, workspace, reference string, amount int64, description string) error {
	var locked string
	if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM cloud_credit_accounts WHERE workspace_id=$1 FOR UPDATE`, workspace).Scan(&locked); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO cloud_credit_entries(id,workspace_id,reference,amount,kind,description)VALUES($1,$2,$3,$4,'purchase',$5)ON CONFLICT(workspace_id,reference)DO NOTHING`, uuid.NewString(), workspace, reference, amount, description)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil
	}
	_, err = tx.ExecContext(ctx, `UPDATE cloud_credit_accounts SET balance=balance+greatest(0,$2-debt),debt=greatest(0,debt-$2) WHERE workspace_id=$1`, workspace, amount)
	return err
}
func (s *Store) Credits(ctx context.Context, workspace string) (CreditBalance, error) {
	b := CreditBalance{Transactions: []CreditEntry{}}
	err := s.DB.QueryRowContext(ctx, `SELECT CASE WHEN restricted OR dispute_hold OR debt>0 THEN 0 ELSE balance-reserved END,reserved,debt,(restricted OR dispute_hold) FROM cloud_credit_accounts WHERE workspace_id=$1`, workspace).Scan(&b.Available, &b.Reserved, &b.Debt, &b.Restricted)
	if err != nil {
		return b, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,amount,kind,description,created_at FROM cloud_credit_entries WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 100`, workspace)
	if err != nil {
		return b, err
	}
	defer rows.Close()
	for rows.Next() {
		var e CreditEntry
		if err = rows.Scan(&e.ID, &e.Amount, &e.Kind, &e.Description, &e.CreatedAt); err != nil {
			return b, err
		}
		b.Transactions = append(b.Transactions, e)
	}
	return b, rows.Err()
}
func (s *Store) CreateQuote(ctx context.Context, u User, in QuoteInput, creditsPerSecond int64) (Quote, error) {
	return s.createQuote(ctx, u, in, creditsPerSecond, "configured-video-model")
}
func (s *Store) createQuote(ctx context.Context, u User, in QuoteInput, rate int64, model string) (Quote, error) {
	if strings.TrimSpace(in.Prompt) == "" || len(in.Prompt) > 8000 || in.Duration < 5 || in.Duration > 30 || rate < 1 || rate > 10000 || (in.AspectRatio != "16:9" && in.AspectRatio != "9:16" && in.AspectRatio != "1:1") {
		return Quote{}, ErrValidation
	}
	var owned bool
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_projects WHERE id=$1 AND workspace_id=$2)`, in.ProjectID, u.WorkspaceID).Scan(&owned); err != nil {
		return Quote{}, err
	}
	if !owned {
		return Quote{}, ErrNotFound
	}
	q := Quote{ID: uuid.NewString(), Credits: rate * int64(in.Duration), ExpiresAt: time.Now().UTC().Add(15 * time.Minute)}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_quotes(id,workspace_id,project_id,prompt,duration,aspect_ratio,credits,model,expires_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, q.ID, u.WorkspaceID, in.ProjectID, in.Prompt, in.Duration, in.AspectRatio, q.Credits, model, q.ExpiresAt)
	return q, err
}
func (s *Store) SubmitJob(ctx context.Context, u User, quoteID, key string) (Job, error) {
	if len(key) < 8 || len(key) > 128 {
		return Job{}, ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	// Account lock serializes reservations and identical submissions, not Redis.
	var available int64
	if err = tx.QueryRowContext(ctx, `SELECT CASE WHEN restricted OR dispute_hold OR debt>0 THEN 0 ELSE balance-reserved END FROM cloud_credit_accounts WHERE workspace_id=$1 FOR UPDATE`, u.WorkspaceID).Scan(&available); err != nil {
		return Job{}, err
	}
	existing, e := scanJob(tx.QueryRowContext(ctx, `SELECT `+jobCols+` FROM cloud_jobs WHERE workspace_id=$1 AND idempotency_key=$2`, u.WorkspaceID, key))
	if e == nil {
		if existing.QuoteID != quoteID {
			return Job{}, ErrConflict
		}
		return existing, tx.Commit()
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return Job{}, e
	}
	var credits int64
	var consumed bool
	var expiry time.Time
	err = tx.QueryRowContext(ctx, `SELECT credits,consumed,expires_at FROM cloud_quotes WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, quoteID, u.WorkspaceID).Scan(&credits, &consumed, &expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	if consumed || !expiry.After(time.Now()) {
		return Job{}, ErrConflict
	}
	if credits > available {
		return Job{}, ErrInsufficientCredits
	}
	id := uuid.NewString()
	j, err := scanJob(tx.QueryRowContext(ctx, `INSERT INTO cloud_jobs(id,workspace_id,quote_id,idempotency_key,credits)VALUES($1,$2,$3,$4,$5)RETURNING `+jobCols, id, u.WorkspaceID, quoteID, key, credits))
	if err != nil {
		return Job{}, err
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{{`UPDATE cloud_credit_accounts SET reserved=reserved+$2 WHERE workspace_id=$1`, []any{u.WorkspaceID, credits}}, {`UPDATE cloud_quotes SET consumed=true WHERE id=$1`, []any{quoteID}}, {`INSERT INTO cloud_outbox(id,kind,aggregate_id)VALUES($1,'generation',$2)`, []any{uuid.NewString(), id}}, {`INSERT INTO cloud_job_events(job_id,status)VALUES($1,'queued')`, []any{id}}} {
		if _, err = tx.ExecContext(ctx, q.sql, q.args...); err != nil {
			return Job{}, err
		}
	}
	return j, tx.Commit()
}
func (s *Store) Jobs(ctx context.Context, u User) ([]Job, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+jobCols+` FROM cloud_jobs WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 100`, u.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, e := scanJob(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
func (s *Store) Job(ctx context.Context, u User, id string) (Job, error) {
	j, err := scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+` FROM cloud_jobs WHERE id=$1 AND workspace_id=$2`, id, u.WorkspaceID))
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return j, err
}
func (s *Store) FinishJob(ctx context.Context, id, status, key, message string) error {
	return s.finishJob(ctx, id, 0, status, key, message)
}

// FinishJobAttempt settles only the worker that still owns an unexpired lease.
func (s *Store) FinishJobAttempt(ctx context.Context, id string, attempt int, status, key, message string) error {
	if attempt < 1 {
		return ErrValidation
	}
	return s.finishJob(ctx, id, attempt, status, key, message)
}
func (s *Store) finishJob(ctx context.Context, id string, attempt int, status, key, message string, reconciliationActor ...string) error {
	if status != "succeeded" && status != "failed" && status != "cancelled" {
		return ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	j, err := scanJob(tx.QueryRowContext(ctx, `SELECT `+jobCols+` FROM cloud_jobs WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	if len(reconciliationActor) > 0 && (reconciliationActor[0] == "" || j.Status != "reconciliation_required" || j.ProviderID != "") {
		return ErrConflict
	}
	if status == "cancelled" {
		if j.Status != "queued" && j.Status != "running" {
			return ErrConflict
		}
		// Read again after obtaining the job row lock. The original SELECT's
		// snapshot may precede an admission that committed while it waited.
		var admitted bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_provider_admissions WHERE job_id=$1)`, id).Scan(&admitted); err != nil {
			return err
		}
		if admitted {
			return ErrConflict
		}
	}
	if attempt > 0 {
		var owns bool
		if err = tx.QueryRowContext(ctx, `SELECT attempt=$2 AND lease_until>now() FROM cloud_jobs WHERE id=$1`, id, attempt).Scan(&owns); err != nil {
			return err
		}
		if !owns {
			return ErrConflict
		}
	}
	if j.Status == "succeeded" || j.Status == "failed" || j.Status == "cancelled" {
		return tx.Commit()
	}
	amount := int64(0)
	if status == "succeeded" {
		amount = j.Credits
		if key == "" {
			return ErrValidation
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_credit_accounts SET reserved=reserved-$2,balance=balance-$3 WHERE workspace_id=$1`, j.WorkspaceID, j.Credits, amount); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_credit_accounts SET balance=balance-least(debt,balance-reserved),debt=debt-least(debt,balance-reserved) WHERE workspace_id=$1`, j.WorkspaceID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_credit_entries(id,workspace_id,reference,amount,kind,description)VALUES($1,$2,$3,$4,$5,$6)`, uuid.NewString(), j.WorkspaceID, "job:"+id, -amount, status, fmt.Sprintf("Video generation %s", status)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_jobs SET status=$2,output_key=$3,error=$4,updated_at=now(),lease_until=NULL WHERE id=$1`, id, status, key, message); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_job_events(job_id,status,message)VALUES($1,$2,$3)`, id, status, message); err != nil {
		return err
	}
	if len(reconciliationActor) > 0 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'resolve_job_fail',$2,'Explicit operator failure/refund')`, reconciliationActor[0], id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
