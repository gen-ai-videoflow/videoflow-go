package cloud

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/mediaconvert"
	mt "github.com/aws/aws-sdk-go-v2/service/mediaconvert/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/redis/go-redis/v9"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"videoflow-go/internal/providers"
)

type Worker struct {
	storyHTTP *http.Client
	redis     *redis.Client
	store     *Store
	cfg       Config
	media     *Media
	queue     *sqs.Client
	encoder   *mediaconvert.Client
	provider  *providers.OpenRouterClient
}
type workMessage struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type encodingEvent struct {
	Source string `json:"source"`
	Detail struct {
		JobID        string            `json:"jobId"`
		Status       string            `json:"status"`
		UserMetadata map[string]string `json:"userMetadata"`
	} `json:"detail"`
}

func parseEncodingEvent(body []byte) (encodingEvent, error) {
	var envelope struct {
		Message string `json:"Message"`
	}
	_ = json.Unmarshal(body, &envelope)
	if envelope.Message != "" {
		body = []byte(envelope.Message)
	}
	var e encodingEvent
	err := json.Unmarshal(body, &e)
	if err == nil && (e.Source != "aws.mediaconvert" || e.Detail.JobID == "" || e.Detail.UserMetadata["video_id"] == "") {
		err = ErrValidation
	}
	return e, err
}
func NewWorker(ctx context.Context, store *Store, cfg Config) (*Worker, error) {
	if cfg.QueueURL == "" || cfg.Region == "" || cfg.OriginalsBucket == "" {
		return nil, errors.New("worker requires queue URL, region and originals bucket")
	}
	ac, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, err
	}
	media, err := NewMedia(ctx, cfg)
	if err != nil {
		return nil, err
	}
	encoder := mediaconvert.NewFromConfig(ac, func(o *mediaconvert.Options) {
		if cfg.MediaConvertEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.MediaConvertEndpoint)
		}
	})
	var redisClient *redis.Client
	if cfg.RedisURL != "" {
		opts, e := redis.ParseURL(cfg.RedisURL)
		if e != nil {
			return nil, e
		}
		redisClient = redis.NewClient(opts)
	}
	return &Worker{redis: redisClient, store: store, cfg: cfg, media: media, queue: sqs.NewFromConfig(ac), encoder: encoder, provider: providers.NewOpenRouterClient(cfg.OpenRouterKey)}, nil
}
func (w *Worker) Run(ctx context.Context) error {
	if w.redis != nil {
		defer w.redis.Close()
	}
	nextMaintenance := time.Now()
	for ctx.Err() == nil {
		if time.Now().After(nextMaintenance) {
			if e := w.Maintenance(ctx); e != nil {
				slog.Error("maintenance failed", "error", e)
			}
			nextMaintenance = time.Now().Add(5 * time.Minute)
		}
		if err := w.dispatch(ctx); err != nil {
			slog.Error("outbox dispatch failed", "error", err)
		}
		if err := w.reconcile(ctx); err != nil {
			slog.Error("job reconciliation failed", "error", err)
		}
		if w.cfg.EncodingQueueURL != "" && w.cfg.EncodingQueueURL != w.cfg.QueueURL {
			batch, e := w.queue.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{QueueUrl: aws.String(w.cfg.EncodingQueueURL), MaxNumberOfMessages: 10, WaitTimeSeconds: 1, VisibilityTimeout: 60})
			if e != nil {
				return e
			}
			for _, message := range batch.Messages {
				event, e := parseEncodingEvent([]byte(aws.ToString(message.Body)))
				if e != nil {
					slog.Error("invalid encoding event", "error", e)
					continue
				}
				if e = w.encodingComplete(ctx, event); e != nil {
					return e
				}
				if _, e = w.queue.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: aws.String(w.cfg.EncodingQueueURL), ReceiptHandle: message.ReceiptHandle}); e != nil {
					return e
				}
			}
		}
		messages, err := w.queue.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{QueueUrl: aws.String(w.cfg.QueueURL), MaxNumberOfMessages: 1, WaitTimeSeconds: 10, VisibilityTimeout: 600})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		for _, message := range messages.Messages {
			if err = w.process(ctx, aws.ToString(message.Body)); err != nil {
				slog.Error("work item failed", "error", err)
				continue
			}
			if _, err = w.queue.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: aws.String(w.cfg.QueueURL), ReceiptHandle: message.ReceiptHandle}); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}
func (w *Worker) dispatch(ctx context.Context) error {
	tx, err := w.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,kind,aggregate_id FROM cloud_outbox WHERE sent_at IS NULL ORDER BY created_at LIMIT 20 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return err
	}
	type item struct{ id, kind, aggregate string }
	items := []item{}
	for rows.Next() {
		var i item
		if err = rows.Scan(&i.id, &i.kind, &i.aggregate); err != nil {
			rows.Close()
			return err
		}
		items = append(items, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, i := range items {
		b, _ := json.Marshal(workMessage{Kind: i.kind, ID: i.aggregate})
		if _, err = w.queue.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(w.cfg.QueueURL), MessageBody: aws.String(string(b))}); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE cloud_outbox SET sent_at=now() WHERE id=$1`, i.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (w *Worker) process(ctx context.Context, body string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var msg workMessage
	if json.Unmarshal([]byte(body), &msg) == nil && msg.ID != "" {
		switch msg.Kind {
		case "generation":
			return w.generate(ctx, msg.ID)
		case "encoding":
			return w.encode(ctx, msg.ID)
		default:
			return ErrValidation
		}
	}
	event, err := parseEncodingEvent([]byte(body))
	if err != nil {
		return err
	}
	return w.encodingComplete(ctx, event)
}
func (w *Worker) generate(ctx context.Context, id string) error {
	var pipeline bool
	if err := w.store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_pipeline_plans p JOIN cloud_jobs j ON j.quote_id=p.quote_id WHERE j.id=$1)`, id).Scan(&pipeline); err != nil {
		return err
	}
	if pipeline {
		return w.runPipeline(ctx, id)
	}

	defer w.notifyProgress(ctx, id)
	var providerID, workspace string
	var attempt int
	err := w.store.DB.QueryRowContext(ctx, `UPDATE cloud_jobs SET lease_until=now()+interval '10 minutes',attempt=attempt+1,status=CASE WHEN status='queued' THEN 'running' ELSE status END,updated_at=now() WHERE id=$1 AND status IN ('queued','running') AND (lease_until IS NULL OR lease_until<now()) RETURNING provider_id,workspace_id,attempt`, id).Scan(&providerID, &workspace, &attempt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if providerID == "" {
		if w.cfg.OpenRouterKey == "" {
			return w.store.FinishJobAttempt(ctx, id, attempt, "failed", "", "Video provider is not configured")
		}
		var prompt, model, aspect, resolution string
		var maxCost int64
		var duration int
		if err = w.store.DB.QueryRowContext(ctx, `SELECT q.prompt,q.model,q.duration,q.aspect_ratio,q.provider_max_cost_micros,q.resolution FROM cloud_quotes q JOIN cloud_jobs j ON j.quote_id=q.id WHERE j.id=$1`, id).Scan(&prompt, &model, &duration, &aspect, &maxCost, &resolution); err != nil {
			return err
		}
		payload := map[string]any{"model": model, "prompt": prompt, "duration": duration, "aspect_ratio": aspect, "resolution": resolution}
		if err = w.provider.ValidateVideo(ctx, payload); err != nil {
			return w.store.FinishJobAttempt(ctx, id, attempt, "failed", "", "Requested generation capabilities are unavailable")
		}
		if maxCost > w.cfg.ProviderMaxCostMicros {
			return w.store.FinishJobAttempt(ctx, id, attempt, "failed", "", "Provider cost limit unavailable")
		}
		if err = w.store.TryProviderBudget(ctx, id, maxCost, w.cfg.ProviderDailyBudgetMicros); err != nil {
			if errors.Is(err, ErrProviderBudget) {
				return w.store.FinishJobAttempt(ctx, id, attempt, "failed", "", "Provider daily spending limit reached")
			}
			return err
		}
		if err = w.claimVideoSubmission(ctx, id, attempt); err != nil {
			if errors.Is(err, ErrProviderBudget) {
				return w.pipelineBudgetWait(ctx, id, attempt)
			}
			return err
		}
		providerID, err = w.provider.CreateVideo(ctx, payload)
		if err != nil {
			_, dbErr := w.store.DB.ExecContext(ctx, `UPDATE cloud_jobs SET status='reconciliation_required',error='Provider submission outcome is uncertain',lease_until=NULL,updated_at=now() WHERE id=$1 AND attempt=$2`, id, attempt)
			return dbErr
		}
		if _, err = w.store.DB.ExecContext(ctx, `UPDATE cloud_jobs SET provider_id=$2,status='running',updated_at=now() WHERE id=$1 AND attempt=$3 AND status='submitting' AND lease_until>now()`, id, providerID, attempt); err != nil {
			return err
		}
	}
	pollCtx, cancelPoll := context.WithTimeout(ctx, 45*time.Second)
	status, _, err := w.provider.GetVideo(pollCtx, providerID)
	cancelPoll()
	if status == "failed" || status == "cancelled" {
		return w.store.FinishJobAttempt(ctx, id, attempt, "failed", "", "Provider generation failed")
	}
	if err != nil {
		return err
	}
	switch strings.ToLower(status) {
	case "failed", "cancelled":
		return w.store.FinishJobAttempt(ctx, id, attempt, "failed", "", "Provider generation failed")
	case "completed", "succeeded":
		data, err := w.provider.VideoContent(ctx, providerID, 0)
		if err != nil {
			return err
		}
		key := "workspaces/" + workspace + "/jobs/" + id + "/output.mp4"
		if _, err = w.media.s3.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(w.cfg.OriginalsBucket), Key: aws.String(key), Body: bytes.NewReader(data), ContentType: aws.String("video/mp4")}); err != nil {
			return err
		}
		return w.store.FinishJobAttempt(ctx, id, attempt, "succeeded", key, "")
	default:
		_, err = w.store.DB.ExecContext(ctx, `UPDATE cloud_jobs SET lease_until=now()+interval '15 seconds' WHERE id=$1 AND attempt=$2`, id, attempt)
		return err
	}
}
func (w *Worker) reconcile(ctx context.Context) error {
	// Poll durable encoding IDs as well as consuming events, so lost events recover.
	encRows, encErr := w.store.DB.QueryContext(ctx, `SELECT id,encoding_id FROM cloud_videos WHERE status='processing' AND encoding_id<>'' AND encoding_id NOT LIKE 'submitting:%' LIMIT 20`)
	if encErr != nil {
		return encErr
	}
	type pendingEncoding struct{ id, provider string }
	pending := []pendingEncoding{}
	for encRows.Next() {
		var p pendingEncoding
		if encErr = encRows.Scan(&p.id, &p.provider); encErr != nil {
			encRows.Close()
			return encErr
		}
		pending = append(pending, p)
	}
	encErr = encRows.Err()
	encRows.Close()
	if encErr != nil {
		return encErr
	}
	for _, p := range pending {
		result, e := w.encoder.GetJob(ctx, &mediaconvert.GetJobInput{Id: aws.String(p.provider)})
		if e != nil {
			slog.Error("encoding reconciliation failed", "video", p.id, "error", e)
			continue
		}
		if result.Job == nil {
			continue
		}
		event := encodingEvent{}
		event.Detail.JobID = p.provider
		event.Detail.Status = string(result.Job.Status)
		event.Detail.UserMetadata = map[string]string{"video_id": p.id}
		if e = w.encodingComplete(ctx, event); e != nil {
			return e
		}
	}
	if _, err := w.store.DB.ExecContext(ctx, `DELETE FROM cloud_sessions WHERE expires_at<now() OR last_seen_at<now()-interval '7 days'`); err != nil {
		return err
	}
	if _, err := w.store.DB.ExecContext(ctx, `DELETE FROM cloud_tokens WHERE expires_at<now()`); err != nil {
		return err
	}

	// Submitting without a persisted provider ID is never automatically resubmitted.
	if _, err := w.store.DB.ExecContext(ctx, `UPDATE cloud_jobs SET status='reconciliation_required',error='Submission interrupted; provider reconciliation required',lease_until=NULL,updated_at=now() WHERE status='submitting' AND lease_until<now()`); err != nil {
		return err
	}
	rows, err := w.store.DB.QueryContext(ctx, `SELECT id FROM cloud_jobs WHERE status='running' AND (lease_until IS NULL OR lease_until<now()) LIMIT 20`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = w.generate(ctx, id); err != nil {
			slog.Error("generation poll failed", "job", id, "error", err)
		}
	}
	return nil
}
func (w *Worker) encode(ctx context.Context, id string) error {
	if w.cfg.PlaybackBucket == "" || w.cfg.MediaConvertRole == "" {
		return errors.New("encoding requires playback bucket and MediaConvert role")
	}
	tx, err := w.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var key, status, encoding, aspect, resolution string
	var captionJSON []byte
	if err = tx.QueryRowContext(ctx, `SELECT j.output_key,v.status,v.encoding_id,q.aspect_ratio,q.resolution,v.caption_segments FROM cloud_videos v JOIN cloud_jobs j ON j.id=v.job_id JOIN cloud_quotes q ON q.id=j.quote_id WHERE v.id=$1 FOR UPDATE OF v`, id).Scan(&key, &status, &encoding, &aspect, &resolution, &captionJSON); err != nil {
		return err
	}
	if status != "processing" || encoding != "" {
		return nil
	}
	if !validMediaKey(key) {
		return ErrValidation
	}
	if string(captionJSON) != "[]" && len(captionJSON) > 2 {
		vtt, e := captionsVTT(captionJSON)
		if e != nil {
			return e
		}
		if _, e = w.media.s3.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(w.cfg.PlaybackBucket), Key: aws.String("videos/" + id + "/hls/captions.vtt"), Body: strings.NewReader(vtt), ContentType: aws.String("text/vtt; charset=utf-8")}); e != nil {
			return e
		}
	}
	destination := "s3://" + w.cfg.PlaybackBucket + "/videos/" + id + "/hls/"
	hasAudio, err := w.sourceHasAudio(ctx, key)
	if err != nil {
		return err
	}
	outputs := []mt.Output{}
	for _, r := range []struct {
		name          string
		width, height int32
		bitrate       int32
	}{{"480", 854, 480, 1200000}, {"720", 1280, 720, 2500000}, {"1080", 1920, 1080, 5000000}} {
		if resolution == "480p" && r.height > 480 || resolution == "720p" && r.height > 720 {
			continue
		}
		if aspect == "9:16" {
			r.width, r.height = r.height, r.width
		} else if aspect == "1:1" {
			r.width = r.height
		}
		outputs = append(outputs, mt.Output{NameModifier: aws.String("_" + r.name), ContainerSettings: &mt.ContainerSettings{Container: mt.ContainerTypeM3u8}, VideoDescription: &mt.VideoDescription{Width: aws.Int32(r.width), Height: aws.Int32(r.height), ScalingBehavior: mt.ScalingBehaviorFitNoUpscale, CodecSettings: &mt.VideoCodecSettings{Codec: mt.VideoCodecH264, H264Settings: &mt.H264Settings{RateControlMode: mt.H264RateControlModeQvbr, MaxBitrate: aws.Int32(r.bitrate), SceneChangeDetect: mt.H264SceneChangeDetectEnabled}}}})
	}
	if hasAudio {
		for i := range outputs {
			outputs[i].AudioDescriptions = []mt.AudioDescription{{AudioSourceName: aws.String("source"), CodecSettings: &mt.AudioCodecSettings{Codec: mt.AudioCodecAac, AacSettings: &mt.AacSettings{Bitrate: aws.Int32(128000), CodingMode: mt.AacCodingModeCodingMode20, SampleRate: aws.Int32(48000)}}}}
		}
	}
	inputs := []mt.Input{{FileInput: aws.String("s3://" + w.cfg.OriginalsBucket + "/" + key)}}
	if hasAudio {
		inputs[0].AudioSelectors = map[string]mt.AudioSelector{"source": {SelectorType: mt.AudioSelectorTypeTrack, Tracks: []int32{1}, DefaultSelection: mt.AudioDefaultSelectionDefault}}
	}
	// Persist submission intent before contacting MediaConvert. An ambiguous result
	// is reconciled through the event's video metadata, never blindly submitted again.
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_videos SET encoding_id=$2,manifest_key=$3 WHERE id=$1`, id, "submitting:"+id, "videos/"+id+"/hls/output.m3u8"); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	out, err := w.encoder.CreateJob(ctx, &mediaconvert.CreateJobInput{Role: aws.String(w.cfg.MediaConvertRole), ClientRequestToken: aws.String(id), UserMetadata: map[string]string{"video_id": id}, Settings: &mt.JobSettings{Inputs: inputs, OutputGroups: []mt.OutputGroup{{Name: aws.String("HLS"), OutputGroupSettings: &mt.OutputGroupSettings{Type: mt.OutputGroupTypeHlsGroupSettings, HlsGroupSettings: &mt.HlsGroupSettings{Destination: aws.String(destination), SegmentLength: aws.Int32(4), MinSegmentLength: aws.Int32(0)}}, Outputs: outputs}}}})
	if err != nil || out.Job == nil || out.Job.Id == nil {
		_, dbErr := w.store.DB.ExecContext(ctx, `UPDATE cloud_videos SET status='encoding_reconciliation_required' WHERE id=$1 AND status='processing' AND encoding_id=$2`, id, "submitting:"+id)
		if dbErr != nil {
			return dbErr
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("MediaConvert returned no job")
	}
	_, err = w.store.DB.ExecContext(ctx, `UPDATE cloud_videos SET encoding_id=$2 WHERE id=$1 AND encoding_id=$3`, id, *out.Job.Id, "submitting:"+id)
	return err
}
func (w *Worker) encodingComplete(ctx context.Context, event encodingEvent) error {
	switch event.Detail.Status {
	case "COMPLETE":
		_, err := w.store.DB.ExecContext(ctx, `UPDATE cloud_videos SET status='published',encoding_id=$2,published_at=COALESCE(published_at,now()) WHERE id=$1 AND (encoding_id=$2 OR encoding_id='submitting:'||id) AND status IN ('processing','encoding_reconciliation_required')`, event.Detail.UserMetadata["video_id"], event.Detail.JobID)
		return err
	case "ERROR", "CANCELED":
		_, err := w.store.DB.ExecContext(ctx, `UPDATE cloud_videos SET status='encoding_failed',encoding_id=$2 WHERE id=$1 AND (encoding_id=$2 OR encoding_id='submitting:'||id) AND status IN ('processing','encoding_reconciliation_required')`, event.Detail.UserMetadata["video_id"], event.Detail.JobID)
		return err
	default:
		return nil
	}
}

// sourceHasAudio parses MP4 atom boundaries instead of assuming every model emits audio.
func (w *Worker) sourceHasAudio(ctx context.Context, key string) (bool, error) {
	source, err := w.media.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(w.cfg.OriginalsBucket), Key: aws.String(key)})
	if err != nil {
		return false, err
	}
	defer source.Body.Close()
	const limit = 256 << 20
	data, err := io.ReadAll(io.LimitReader(source.Body, limit+1))
	if err != nil {
		return false, err
	}
	if len(data) > limit {
		return false, errors.New("source exceeds encoding inspection limit")
	}
	return mp4HasAudio(data, 0)
}
func mp4HasAudio(data []byte, depth int) (bool, error) {
	if depth > 4 {
		return false, ErrValidation
	}
	for len(data) > 0 {
		if len(data) < 8 {
			return false, ErrValidation
		}
		size := uint64(binary.BigEndian.Uint32(data[:4]))
		header := uint64(8)
		kind := string(data[4:8])
		if size == 1 {
			if len(data) < 16 {
				return false, ErrValidation
			}
			size = binary.BigEndian.Uint64(data[8:16])
			header = 16
		}
		if size == 0 {
			size = uint64(len(data))
		}
		if size < header || size > uint64(len(data)) {
			return false, ErrValidation
		}
		body := data[header:size]
		switch kind {
		case "moov", "trak", "mdia":
			found, err := mp4HasAudio(body, depth+1)
			if err != nil {
				return false, err
			}
			if found {
				return true, nil
			}
		case "hdlr":
			if len(body) < 12 {
				return false, ErrValidation
			}
			if string(body[8:12]) == "soun" {
				return true, nil
			}
		}
		data = data[size:]
	}
	return false, nil
}
