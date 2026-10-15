package cloud

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	st "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func (w *Worker) cleanupVideoUploads(ctx context.Context) error {
	if w.media == nil || w.media.s3 == nil || w.cfg.UploadsBucket == "" {
		return nil
	}
	_, e := w.store.DB.ExecContext(ctx, `UPDATE cloud_video_uploads SET state='aborting' WHERE (expires_at<now() AND state IN ('creating','uploading','completing')) OR state='rejected'`)
	if e != nil {
		return e
	}
	rows, e := w.store.DB.QueryContext(ctx, `SELECT id FROM cloud_video_uploads WHERE state='aborting' AND NOT quota_released LIMIT 20`)
	if e != nil {
		return e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if e = w.cleanupOneVideoUpload(ctx, id); e != nil && !errors.Is(e, ErrUnavailable) {
			return e
		}
	}
	return nil
}
func (w *Worker) cleanupOneVideoUpload(ctx context.Context, id string) error {
	tx, e := w.store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	v, e := scanUpload(tx.QueryRowContext(ctx, `SELECT `+uploadCols+` FROM cloud_video_uploads WHERE id=$1 AND state='aborting' FOR UPDATE`, id))
	if e == ErrNotFound {
		return nil
	}
	if e != nil {
		return e
	}
	// Reconcile even an ambiguous CreateMultipartUpload by exact owned object key.
	list, e := w.media.s3.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{Bucket: aws.String(w.cfg.UploadsBucket), Prefix: aws.String(v.Key), MaxUploads: aws.Int32(50)})
	if e != nil {
		return e
	}
	if aws.ToBool(list.IsTruncated) {
		return ErrUnavailable
	}
	// Also abort the known identifier directly; do not rely on list visibility.
	if v.StorageID != "" {
		list.Uploads = append(list.Uploads, st.MultipartUpload{Key: aws.String(v.Key), UploadId: aws.String(v.StorageID)})
	}
	seen := map[string]bool{}
	for _, part := range list.Uploads {
		if aws.ToString(part.Key) != v.Key || seen[aws.ToString(part.UploadId)] {
			continue
		}
		seen[aws.ToString(part.UploadId)] = true
		_, e = w.media.s3.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(w.cfg.UploadsBucket), Key: aws.String(v.Key), UploadId: part.UploadId})
		if e != nil && !objectAbsent(e) {
			return e
		}
		_, e = w.media.s3.ListParts(ctx, &s3.ListPartsInput{Bucket: aws.String(w.cfg.UploadsBucket), Key: aws.String(v.Key), UploadId: part.UploadId, MaxParts: aws.Int32(1)})
		if e == nil {
			return ErrUnavailable
		}
		if !objectAbsent(e) {
			return e
		}
	}
	// A failed or interrupted creation can have an ambiguous remote outcome.
	// Absence in one listing is insufficient proof that a delayed creation cannot
	// arrive later. Retain its reservation for operator reconciliation.
	if v.StorageID == "" && len(list.Uploads) == 0 {
		return ErrUnavailable
	}
	var original string
	e = tx.QueryRowContext(ctx, `SELECT object_key FROM cloud_media_assets WHERE id=$1 AND status<>'ready'`, v.AssetID).Scan(&original)
	if e != nil {
		return e
	}
	for _, object := range []struct{ bucket, key string }{{w.cfg.UploadsBucket, v.Key}, {w.cfg.OriginalsBucket, original}} {
		if object.bucket == "" {
			return ErrUnavailable
		}
		if _, e = w.media.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(object.bucket), Key: aws.String(object.key)}); e != nil {
			return e
		}
	}
	if w.cfg.PlaybackBucket != "" {
		if _, e = w.media.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(w.cfg.PlaybackBucket), Key: aws.String("posters/assets/" + v.AssetID + ".jpg")}); e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, `UPDATE cloud_media_assets SET status='rejected',error='Upload cancelled or expired.' WHERE id=$1 AND status<>'ready'`, v.AssetID)
	if e == nil {
		_, e = tx.ExecContext(ctx, `UPDATE cloud_video_uploads SET state='aborted',quota_released=true WHERE id=$1 AND state='aborting'`, id)
	}
	if e == nil {
		e = tx.Commit()
	}
	return e
}
