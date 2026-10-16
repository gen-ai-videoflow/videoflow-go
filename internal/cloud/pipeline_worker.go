package cloud

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (w *Worker) runPipeline(ctx context.Context, id string) error {
	var attempt int
	var workspace, quote string
	err := w.store.DB.QueryRowContext(ctx, `UPDATE cloud_jobs SET status='running',attempt=attempt+1,lease_until=now()+interval '10 minutes',updated_at=now() WHERE id=$1 AND status IN ('queued','running') AND (lease_until IS NULL OR lease_until<now())RETURNING attempt,workspace_id,quote_id`, id).Scan(&attempt, &workspace, &quote)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var kind, stage, storyModel, prompt, model, aspect, resolution, output string
	var count, duration, totalDuration int
	var maxCost, storyCost int64
	var keysRaw []byte
	if err = w.store.DB.QueryRowContext(ctx, `SELECT p.kind,p.stage,p.story_model,p.clip_count,p.clip_duration,p.source_keys,p.output_key,q.prompt,q.model,q.aspect_ratio,q.resolution,q.provider_max_cost_micros,q.duration,p.story_max_cost_micros FROM cloud_pipeline_plans p JOIN cloud_quotes q ON q.id=p.quote_id WHERE p.quote_id=$1`, quote).Scan(&kind, &stage, &storyModel, &count, &duration, &keysRaw, &output, &prompt, &model, &aspect, &resolution, &maxCost, &totalDuration, &storyCost); err != nil {
		return err
	}
	fail := func(message string) error { return w.store.FinishJobAttempt(ctx, id, attempt, "failed", "", message) }
	if output != "" {
		return w.store.FinishJobAttempt(ctx, id, attempt, "succeeded", output, "")
	}
	if kind == "storyboard" {
		if stage == "queued" {
			if w.cfg.OpenRouterKey == "" || !approvedStoryModel(w.cfg.StoryModel, storyModel) || storyCost < 1 || storyCost > w.cfg.StoryMaxCostMicros {
				return fail("Story generation policy is unavailable")
			}
			if maxCost > w.cfg.ProviderMaxCostMicros {
				return w.pipelineBudgetWait(ctx, id, attempt)
			}
			if err = w.store.TryProviderBudget(ctx, id, maxCost, w.cfg.ProviderDailyBudgetMicros); err != nil {
				if errors.Is(err, ErrProviderBudget) {
					return w.pipelineBudgetWait(ctx, id, attempt)
				}
				return err
			}
			if err = w.pipelineSubmission(ctx, id, attempt, quote, -1); err != nil {
				if errors.Is(err, ErrProviderBudget) {
					return w.pipelineBudgetWait(ctx, id, attempt)
				}
				return err
			}
			story, e := w.generateStory(ctx, storyModel, prompt, count, duration, storyCost)
			if e != nil {
				return w.pipelineAmbiguous(ctx, id, attempt, "Screenplay submission needs operator reconciliation")
			}
			if err = validatePipelineStory(story, count); err != nil {
				return fail("The screenplay provider returned an invalid storyboard")
			}
			tx, e := w.store.DB.BeginTx(ctx, nil)
			if e != nil {
				return e
			}
			defer tx.Rollback()
			if e = pipelineFence(ctx, tx, id, attempt); e != nil {
				return e
			}
			u, e := scanUser(tx.QueryRowContext(ctx, userSelect+" WHERE w.id=$1", workspace))
			if e != nil {
				return e
			}
			var project string
			if e = tx.QueryRowContext(ctx, `SELECT project_id FROM cloud_quotes WHERE id=$1`, quote).Scan(&project); e != nil {
				return e
			}
			if _, e = createStudioTx(ctx, tx, u, project, "scripts", StudioEntity{Title: "Generated screenplay", Content: story.Script}); e != nil {
				tx.Rollback()
				return fail("Studio capacity reached; screenplay could not be saved")
			}
			for index, scene := range story.Scenes {
				made, e := createStudioTx(ctx, tx, u, project, "scenes", StudioEntity{Title: scene.Title, Prompt: scene.Prompt, Position: index, Duration: float64(duration)})
				if e != nil {
					tx.Rollback()
					return fail("Studio capacity reached; storyboard could not be saved")
				}
				if _, e = createStudioTx(ctx, tx, u, project, "shots", StudioEntity{Title: scene.Title, Prompt: scene.Prompt, SceneID: made.ID, Position: 0, Duration: float64(duration)}); e != nil {
					tx.Rollback()
					return fail("Studio capacity reached; shots could not be saved")
				}
				story.Scenes[index].Duration = duration
				story.Scenes[index].Status = "queued"
				if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_pipeline_clips(quote_id,position,prompt,title)VALUES($1,$2,$3,$4)`, quote, index, scene.Prompt, scene.Title); e != nil {
					return e
				}
			}
			raw, _ := json.Marshal(story)
			if _, e = tx.ExecContext(ctx, `UPDATE cloud_pipeline_plans SET stage='clips',story=$2,updated_at=now() WHERE quote_id=$1`, quote, string(raw)); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE cloud_jobs SET status='running',lease_until=now()+interval '1 second' WHERE id=$1`, id); e != nil {
				return e
			}
			return tx.Commit()
		}
		if stage == "story_submitting" {
			return w.pipelineAmbiguous(ctx, id, attempt, "Interrupted screenplay submission requires reconciliation")
		}
		var position int
		var clipPrompt, status, providerID string
		e := w.store.DB.QueryRowContext(ctx, `SELECT position,prompt,status,provider_id FROM cloud_pipeline_clips WHERE quote_id=$1 AND status<>'succeeded' ORDER BY position LIMIT 1`, quote).Scan(&position, &clipPrompt, &status, &providerID)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if e == nil {
			if status == "submitting" {
				// The operator's explicit provider-ID resolution attaches only to the one ambiguous clip.
				var resolved string
				if err = w.store.DB.QueryRowContext(ctx, `SELECT provider_id FROM cloud_jobs WHERE id=$1`, id).Scan(&resolved); err != nil {
					return err
				}
				if resolved == "" {
					return w.pipelineAmbiguous(ctx, id, attempt, "Interrupted clip submission requires reconciliation")
				}
				if err = w.pipelineRecoverClip(ctx, id, attempt, quote, position, resolved); err != nil {
					return err
				}
				providerID = resolved
				status = "running"
			}
			if status == "queued" {
				payload := map[string]any{"model": model, "prompt": clipPrompt, "duration": duration, "aspect_ratio": aspect, "resolution": resolution}
				validationCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
				err = w.provider.ValidateVideo(validationCtx, payload)
				cancel()
				if err != nil {
					return fail("Storyboard video capabilities are unavailable")
				}
				if err = w.pipelineSubmission(ctx, id, attempt, quote, position); err != nil {
					if errors.Is(err, ErrProviderBudget) {
						return w.pipelineBudgetWait(ctx, id, attempt)
					}
					return err
				}
				submitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
				providerID, err = w.provider.CreateVideo(submitCtx, payload)
				cancel()
				if err != nil {
					return w.pipelineAmbiguous(ctx, id, attempt, "Clip submission outcome requires reconciliation")
				}
				tx, e := w.store.DB.BeginTx(ctx, nil)
				if e != nil {
					return e
				}
				defer tx.Rollback()
				if e = pipelineFence(ctx, tx, id, attempt); e != nil {
					return e
				}
				if _, e = tx.ExecContext(ctx, `UPDATE cloud_pipeline_clips SET provider_id=$3,status='running' WHERE quote_id=$1 AND position=$2`, quote, position, providerID); e != nil {
					return e
				}
				if _, e = tx.ExecContext(ctx, `UPDATE cloud_jobs SET status='running',lease_until=now()+interval '15 seconds',provider_id='' WHERE id=$1`, id); e != nil {
					return e
				}
				return tx.Commit()
			}
			pollCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			state, _, e := w.provider.GetVideo(pollCtx, providerID)
			cancel()
			if state == "failed" || state == "cancelled" {
				return fail("A storyboard clip failed generation")
			}
			if e != nil {
				return e
			}
			if state != "completed" && state != "succeeded" {
				return w.pipelineRelease(ctx, id, attempt)
			}
			downloadCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			data, e := w.provider.VideoContent(downloadCtx, providerID, 0)
			cancel()
			if e != nil {
				return e
			}
			key := fmt.Sprintf("workspaces/%s/jobs/%s/clips/%d.mp4", workspace, id, position)
			if _, e = w.media.s3.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(w.cfg.OriginalsBucket), Key: aws.String(key), Body: bytes.NewReader(data), ContentType: aws.String("video/mp4")}); e != nil {
				return e
			}
			tx, e := w.store.DB.BeginTx(ctx, nil)
			if e != nil {
				return e
			}
			defer tx.Rollback()
			if e = pipelineFence(ctx, tx, id, attempt); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE cloud_pipeline_clips SET status='succeeded',output_key=$3 WHERE quote_id=$1 AND position=$2`, quote, position, key); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE cloud_jobs SET lease_until=now()+interval '1 second',provider_id='' WHERE id=$1`, id); e != nil {
				return e
			}
			return tx.Commit()
		}
	}
	keys := []string{}
	if kind == "assembly" {
		if err = json.Unmarshal(keysRaw, &keys); err != nil {
			return err
		}
	} else {
		rows, e := w.store.DB.QueryContext(ctx, `SELECT output_key FROM cloud_pipeline_clips WHERE quote_id=$1 ORDER BY position`, quote)
		if e != nil {
			return e
		}
		for rows.Next() {
			var key string
			if e = rows.Scan(&key); e != nil {
				rows.Close()
				return e
			}
			keys = append(keys, key)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
	}
	if err = w.pipelineAssemblyStage(ctx, id, attempt, quote); err != nil {
		return err
	}
	key, err := w.assemblePipeline(ctx, workspace, id, attempt, aspect, totalDuration, keys)
	if err != nil {
		return fail("Video assembly failed; your reserved credits were released")
	}
	tx, err := w.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = pipelineFence(ctx, tx, id, attempt); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_pipeline_plans SET stage='output_ready',output_key=$2,updated_at=now() WHERE quote_id=$1`, quote, key); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return w.store.FinishJobAttempt(ctx, id, attempt, "succeeded", key, "")
}
func pipelineFence(ctx context.Context, tx *sql.Tx, id string, attempt int) error {
	var valid bool
	if err := tx.QueryRowContext(ctx, `SELECT attempt=$2 AND lease_until>now() AND status IN ('running','submitting') FROM cloud_jobs WHERE id=$1 FOR UPDATE`, id, attempt).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrConflict
	}
	return nil
}
func (w *Worker) pipelineSubmission(ctx context.Context, id string, attempt int, quote string, position int) error {
	tx, err := w.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = pipelineFence(ctx, tx, id, attempt); err != nil {
		return err
	}
	if err = admitPipelineDayTx(ctx, tx, id, w.cfg.ProviderDailyBudgetMicros, w.cfg.ProviderMaxCostMicros); err != nil {
		return err
	}
	var changed sql.Result
	if position < 0 {
		changed, err = tx.ExecContext(ctx, `UPDATE cloud_pipeline_plans SET stage='story_submitting' WHERE quote_id=$1 AND stage='queued'`, quote)
	} else {
		changed, err = tx.ExecContext(ctx, `UPDATE cloud_pipeline_clips SET status='submitting' WHERE quote_id=$1 AND position=$2 AND status='queued'`, quote, position)
	}
	if err != nil {
		return err
	}
	rows, err := changed.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_jobs SET status='submitting',error='',lease_until=now()+interval '10 minutes',updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (w *Worker) pipelineAmbiguous(ctx context.Context, id string, attempt int, message string) error {
	_, err := w.store.DB.ExecContext(ctx, `UPDATE cloud_jobs SET status='reconciliation_required',error=$3,lease_until=NULL,updated_at=now() WHERE id=$1 AND attempt=$2 AND status IN ('running','submitting')`, id, attempt, message)
	return err
}
func (w *Worker) pipelineRelease(ctx context.Context, id string, attempt int) error {
	_, err := w.store.DB.ExecContext(ctx, `UPDATE cloud_jobs SET lease_until=now()+interval '15 seconds' WHERE id=$1 AND attempt=$2 AND status='running'`, id, attempt)
	return err
}
func (w *Worker) generateStory(ctx context.Context, model, idea string, count, duration int, budget int64) (PipelineStory, error) {
	body, _ := json.Marshal(map[string]any{"model": model, "max_tokens": 2048, "provider": storyProviderLimits(budget), "temperature": 0.6, "messages": []map[string]string{{"role": "system", "content": fmt.Sprintf("Write an original screenplay and exactly %d visual scenes, each suitable for a %d-second independent AI video clip. Return JSON only: {\"script\":\"screenplay\",\"scenes\":[{\"title\":\"title\",\"prompt\":\"detailed self-contained visual direction\"}]}. Keep character appearance and setting consistent across scenes. No external URLs, no copyrighted characters requested by default.", count, duration)}, {"role": "user", "content": idea}}})
	requestCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return PipelineStory{}, err
	}
	req.Header.Set("Authorization", "Bearer "+w.cfg.OpenRouterKey)
	req.Header.Set("Content-Type", "application/json")
	client := w.storyHTTP
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrValidation }}
	}
	resp, err := client.Do(req)
	if err != nil {
		return PipelineStory{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PipelineStory{}, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (128<<10)+1))
	if err != nil || len(raw) > 128<<10 {
		return PipelineStory{}, ErrValidation
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err = json.Unmarshal(raw, &completion); err != nil || len(completion.Choices) != 1 {
		return PipelineStory{}, ErrValidation
	}
	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	var story PipelineStory
	err = json.Unmarshal([]byte(strings.TrimSpace(content)), &story)
	return story, err
}
func (w *Worker) assemblePipeline(ctx context.Context, workspace, id string, attempt int, aspect string, totalDuration int, keys []string) (string, error) {
	if len(keys) < 1 || len(keys) > 8 || totalDuration < 1 || totalDuration > 30 {
		return "", ErrValidation
	}
	dir, err := os.MkdirTemp("", "videoflow-assembly-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	assemblyCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	width, height := 1280, 720
	if aspect == "9:16" {
		width, height = 720, 1280
	} else if aspect == "1:1" {
		width, height = 720, 720
	}
	var list strings.Builder
	for index, key := range keys {
		if !validMediaKey(key) {
			return "", ErrValidation
		}
		source, e := w.media.s3.GetObject(assemblyCtx, &s3.GetObjectInput{Bucket: aws.String(w.cfg.OriginalsBucket), Key: aws.String(key)})
		if e != nil {
			return "", e
		}
		input := filepath.Join(dir, fmt.Sprintf("input-%d.mp4", index))
		file, e := os.Create(input)
		if e != nil {
			source.Body.Close()
			return "", e
		}
		n, e := io.Copy(file, io.LimitReader(source.Body, (256<<20)+1))
		source.Body.Close()
		file.Close()
		if e != nil || n > 256<<20 {
			return "", ErrValidation
		}
		data, e := os.ReadFile(input)
		if e != nil {
			return "", e
		}
		audio, e := mp4HasAudio(data, 0)
		if e != nil {
			return "", e
		}
		data = nil
		name := fmt.Sprintf("clip-%d.mp4", index)
		args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-filter_threads", "2", "-threads", "2", "-protocol_whitelist", "file,pipe", "-f", "mov", "-enable_drefs", "0", "-i", input}
		if !audio {
			args = append(args, "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo")
		}
		args = append(args, "-map", "0:v:0")
		if audio {
			args = append(args, "-map", "0:a:0")
		} else {
			args = append(args, "-map", "1:a:0")
		}
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1", width, height, width, height), "-r", "24", "-c:v", "libx264", "-threads", "2", "-preset", "fast", "-crf", "23", "-pix_fmt", "yuv420p", "-c:a", "aac", "-ar", "48000", "-ac", "2", "-af", "apad", "-shortest", "-t", "30", filepath.Join(dir, name))
		if e = runFFmpeg(assemblyCtx, w.cfg.FFmpegPath, dir, args); e != nil {
			return "", e
		}
		list.WriteString("file '" + name + "'\n")
	}
	manifest := filepath.Join(dir, "concat.txt")
	if err = os.WriteFile(manifest, []byte(list.String()), 0600); err != nil {
		return "", err
	}
	output := filepath.Join(dir, "output.mp4")
	if err = runFFmpeg(assemblyCtx, w.cfg.FFmpegPath, dir, []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-protocol_whitelist", "file,pipe", "-f", "concat", "-safe", "1", "-i", manifest, "-c:v", "libx264", "-threads", "2", "-preset", "fast", "-crf", "23", "-c:a", "aac", "-t", fmt.Sprint(totalDuration), "-movflags", "+faststart", output}); err != nil {
		return "", err
	}
	file, err := os.Open(output)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() < 1 || info.Size() > 512<<20 {
		return "", ErrValidation
	}
	key := fmt.Sprintf("workspaces/%s/jobs/%s/assembly/%d/output.mp4", workspace, id, attempt)
	_, err = w.media.s3.PutObject(assemblyCtx, &s3.PutObjectInput{Bucket: aws.String(w.cfg.OriginalsBucket), Key: aws.String(key), Body: file, ContentType: aws.String("video/mp4")})
	return key, err
}
func runFFmpeg(ctx context.Context, binary, dir string, args []string) error {
	if binary == "" {
		binary = "ffmpeg"
	}
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = dir
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}

// Provider prices use USD per million tokens, numerically micro-USD per token.
// 16,384 input tokens conservatively cover the 8KB idea plus fixed instructions.
func storyProviderLimits(budget int64) map[string]any {
	rate := math.Floor(float64(budget)/18432*1000000) / 1000000
	return map[string]any{"sort": "price", "require_parameters": true, "max_price": map[string]any{"prompt": rate, "completion": rate, "request": 0}}
}

func (w *Worker) pipelineRecoverClip(ctx context.Context, id string, attempt int, quote string, position int, provider string) error {
	tx, err := w.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = pipelineFence(ctx, tx, id, attempt); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE cloud_pipeline_clips SET provider_id=$3,status='running' WHERE quote_id=$1 AND position=$2 AND status='submitting'`, quote, position, provider)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_jobs SET provider_id='' WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (w *Worker) pipelineAssemblyStage(ctx context.Context, id string, attempt int, quote string) error {
	tx, err := w.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = pipelineFence(ctx, tx, id, attempt); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_pipeline_plans SET stage='assembly',updated_at=now() WHERE quote_id=$1`, quote); err != nil {
		return err
	}
	return tx.Commit()
}

func (w *Worker) pipelineBudgetWait(ctx context.Context, id string, attempt int) error {
	_, err := w.store.DB.ExecContext(ctx, `UPDATE cloud_jobs SET error='Waiting for funded provider budget',lease_until=now()+interval '5 minutes' WHERE id=$1 AND attempt=$2 AND status='running'`, id, attempt)
	return err
}

// Paid stages require a current UTC-day admission; status polling and downloads do not.
func admitPipelineDayTx(ctx context.Context, tx *sql.Tx, id string, dailyLimit, operationLimit int64) error {
	if dailyLimit < 1 || operationLimit < 1 {
		return ErrProviderBudget
	}
	var cost int64
	var originalToday bool
	var today time.Time
	if err := tx.QueryRowContext(ctx, `SELECT q.provider_max_cost_micros,a.day=(now() AT TIME ZONE 'UTC')::date,(now() AT TIME ZONE 'UTC')::date FROM cloud_jobs j JOIN cloud_quotes q ON q.id=j.quote_id JOIN cloud_provider_admissions a ON a.job_id=j.id WHERE j.id=$1`, id).Scan(&cost, &originalToday, &today); err != nil {
		return err
	}
	if cost < 1 || cost > operationLimit || cost > dailyLimit {
		return ErrProviderBudget
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_pipeline_budget_days WHERE job_id=$1 AND day=$2)`, id, today).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cloud_provider_budget_days(day)VALUES($1)ON CONFLICT DO NOTHING`, today); err != nil {
		return err
	}
	if !originalToday {
		var reserved int64
		if err := tx.QueryRowContext(ctx, `SELECT reserved_micros FROM cloud_provider_budget_days WHERE day=$1 FOR UPDATE`, today).Scan(&reserved); err != nil {
			return err
		}
		if reserved > dailyLimit-cost {
			return ErrProviderBudget
		}
		if _, err := tx.ExecContext(ctx, `UPDATE cloud_provider_budget_days SET reserved_micros=reserved_micros+$2 WHERE day=$1`, today, cost); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO cloud_pipeline_budget_days(job_id,day,max_cost_micros)VALUES($1,$2,$3)`, id, today, cost)
	return err
}

// Ordinary single-clip jobs use the same lease fence and current-day admission
// immediately before recording a provider submission intent.
func (w *Worker) claimVideoSubmission(ctx context.Context, id string, attempt int) error {
	tx, err := w.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = pipelineFence(ctx, tx, id, attempt); err != nil {
		return err
	}
	if err = admitPipelineDayTx(ctx, tx, id, w.cfg.ProviderDailyBudgetMicros, w.cfg.ProviderMaxCostMicros); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE cloud_jobs SET status='submitting',error='',lease_until=now()+interval '10 minutes',updated_at=now() WHERE id=$1 AND attempt=$2 AND status='running' AND lease_until>now()`, id, attempt)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	return tx.Commit()
}
