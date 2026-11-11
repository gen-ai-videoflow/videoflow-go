package media

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAssembleNormalizesSilentAndAudioClips(t *testing.T) {
	m := NewMediaEngine()
	if !m.Available() {
		t.Skip("FFmpeg fixture required")
	}
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.mp4"), filepath.Join(dir, "b.mp4")
	for i, path := range []string{a, b} {
		args := []string{"-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=160x120:d=1"}
		if i == 1 {
			args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:duration=1")
		}
		args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-t", "1", "-y", path)
		if out, err := exec.Command(m.ffmpegPath, args...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	output := filepath.Join(dir, "combined.mp4")
	if err := m.Assemble(context.Background(), []string{a, b}, output, "16:9"); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(m.ffmpegPath, "-v", "error", "-i", output, "-c:v", "rawvideo", "-c:a", "pcm_s16le", "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("combined video does not decode: %v %s", err, out)
	}
	info, err := m.probeClip(context.Background(), output)
	if err != nil || !info.Audio || info.Duration < 1.9 || info.Duration > 2.3 {
		t.Fatalf("bad combined media: %+v %v", info, err)
	}
}

func TestAssemblyPadsShortAudioWithoutTruncatingVideo(t *testing.T) {
	m := NewMediaEngine()
	if !m.Available() {
		t.Skip("FFmpeg required")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "short-audio.mp4")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=160x120:d=4", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:v", "libx264", "-c:a", "aac", "-t", "4", source}
	if b, err := exec.Command(m.ffmpegPath, args...).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	output := filepath.Join(dir, "combined.mp4")
	if err := m.Assemble(context.Background(), []string{source, source}, output, "16:9"); err != nil {
		t.Fatal(err)
	}
	info, err := m.probeClip(context.Background(), output)
	if err != nil || info.Duration < 7.9 {
		t.Fatalf("visual footage was truncated by short audio: %+v %v", info, err)
	}
}
