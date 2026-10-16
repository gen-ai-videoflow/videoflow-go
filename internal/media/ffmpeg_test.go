package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"6eek-go/internal/models"
)

func TestAudioExtractionAndCaptionBurn(t *testing.T) {
	m := NewMediaEngine()
	if !m.Available() {
		t.Skip("FFmpeg not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "video.mp4")
	cmd := exec.Command(m.ffmpegPath, "-y", "-f", "lavfi", "-i", "color=c=blue:s=320x240:d=2", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:v", "libx264", "-c:a", "aac", "-shortest", video)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture %v %s", err, out)
	}
	audio := filepath.Join(dir, "audio.wav")
	if err := m.ExtractAudio(context.Background(), video, audio); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(audio); err != nil || len(data) < 44 {
		t.Fatal("missing WAV", err)
	}
	captioned := filepath.Join(dir, "captioned.mp4")
	if err := m.BurnCaptions(context.Background(), video, captioned, []models.CaptionSegment{{Start: 0.2, End: 1.5, Text: "Actual captions"}}); err != nil {
		t.Fatal(err)
	}
	bold := filepath.Join(dir, "bold.mp4")
	if err := m.BurnCaptions(context.Background(), video, bold, []models.CaptionSegment{{Start: 0.2, End: 1.5, Text: "Actual captions"}}, "bold"); err != nil {
		t.Fatal(err)
	}
	normalBytes, _ := os.ReadFile(captioned)
	boldBytes, _ := os.ReadFile(bold)
	if string(normalBytes) == string(boldBytes) {
		t.Fatal("caption style has no effect")
	}
	before, _ := os.ReadFile(video)
	after, _ := os.ReadFile(captioned)
	if len(after) == 0 || string(before) == string(after) {
		t.Fatal("caption video not generated")
	}
}

func TestCaptionBurnUsesBundledFont(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("Unix media package")
	}
	root := filepath.Join(t.TempDir(), "6eek with spaces")
	if err := os.MkdirAll(filepath.Join(root, "fonts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fonts", "NotoSans-Regular.ttf"), []byte("font fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	args := filepath.Join(root, "arguments")
	executable := filepath.Join(root, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + args + "\"\n"
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	m := &MediaEngine{ffmpegPath: executable}
	if err := m.BurnCaptions(context.Background(), "input.mp4", "output.mp4", []models.CaptionSegment{{Start: 0, End: 1, Text: "Bundled font"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "fontsdir='") || !strings.Contains(string(data), "FontName=Noto Sans") {
		t.Fatalf("bundled caption font was not passed to FFmpeg: %s", data)
	}
	if err := os.WriteFile(filepath.Join(root, "fonts", "NotoSansCJKsc-Regular.otf"), []byte("CJK font fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.BurnCaptions(context.Background(), "input.mp4", "output.mp4", []models.CaptionSegment{{Start: 0, End: 1, Text: "English and 中文"}}); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "FontName=Noto Sans CJK SC") {
		t.Fatalf("bundled Chinese font was not selected: %s", data)
	}

}
