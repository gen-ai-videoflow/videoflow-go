package cloud

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRealMediaProbe(t *testing.T) {
	binary := os.Getenv("MEDIA_FFPROBE_TEST_BINARY")
	ffmpeg := os.Getenv("PIPELINE_FFMPEG_TEST_BINARY")
	if binary == "" || ffmpeg == "" {
		t.Skip("media fixture binaries not configured")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "portrait.mp4")
	if e := runFFmpeg(context.Background(), ffmpeg, dir, []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=90x160:d=3", "-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", file}); e != nil {
		t.Fatal(e)
	}
	m, e := probeMedia(context.Background(), binary, file)
	if e != nil || m.Width != 90 || m.Height != 160 || m.DurationMS < 2900 || m.HasAudio {
		t.Fatalf("metadata %+v %v", m, e)
	}
	bad := filepath.Join(dir, "bad.mp4")
	os.WriteFile(bad, []byte("not video"), 0600)
	if _, e = probeMedia(context.Background(), binary, bad); e == nil {
		t.Fatal("invalid video accepted")
	}
}
