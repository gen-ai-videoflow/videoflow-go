package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"6eek-go/internal/models"
)

type MediaEngine struct {
	ffmpegPath string
}

func NewMediaEngine() *MediaEngine {
	path, _ := exec.LookPath("ffmpeg")
	return &MediaEngine{ffmpegPath: path}
}

func (m *MediaEngine) Available() bool {
	return m.ffmpegPath != ""
}

func (m *MediaEngine) MakeThumbnail(ctx context.Context, videoPath, destPath string, atSeconds float64) error {
	if !m.Available() {
		return fmt.Errorf("ffmpeg not found")
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, m.ffmpegPath,
		"-y",
		"-ss", fmt.Sprintf("%.2f", atSeconds),
		"-i", videoPath,
		"-frames:v", "1",
		"-q:v", "3",
		destPath,
	)
	return cmd.Run()
}

func (m *MediaEngine) ExtractLastFrame(ctx context.Context, videoPath, destPath string) error {
	if !m.Available() {
		return fmt.Errorf("ffmpeg not found")
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, m.ffmpegPath,
		"-y",
		"-sseof", "-3",
		"-i", videoPath,
		"-update", "1",
		"-q:v", "2",
		destPath,
	)
	return cmd.Run()
}

func (m *MediaEngine) ExtractAudio(ctx context.Context, videoPath, destPath string) error {
	if !m.Available() {
		return fmt.Errorf("FFmpeg is required to extract video audio")
	}
	cmd := exec.CommandContext(ctx, m.ffmpegPath, "-y", "-i", videoPath, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", destPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract audio: %w: %s", err, out)
	}
	return nil
}

func subtitleTime(seconds float64) string {
	ms := int64(seconds * 1000)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, (ms/60000)%60, (ms/1000)%60, ms%1000)
}
func (m *MediaEngine) BurnCaptions(ctx context.Context, videoPath, destPath string, segments []models.CaptionSegment, styles ...string) error {
	if !m.Available() {
		return fmt.Errorf("FFmpeg with libass is required to burn captions")
	}
	if len(segments) == 0 {
		return fmt.Errorf("transcribe or write captions before burning them")
	}
	dir, err := os.MkdirTemp("", "videoflow-subtitles-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	var content strings.Builder
	for index, segment := range segments {
		if segment.Start < 0 || segment.End <= segment.Start {
			return fmt.Errorf("invalid caption timestamps")
		}
		fmt.Fprintf(&content, "%d\n%s --> %s\n%s\n\n", index+1, subtitleTime(segment.Start), subtitleTime(segment.End), strings.ReplaceAll(segment.Text, "\r", ""))
	}
	subtitles := filepath.Join(dir, "captions.srt")
	if err = os.WriteFile(subtitles, []byte(content.String()), 0600); err != nil {
		return err
	}
	style := "clean"
	if len(styles) > 0 && styles[0] != "" {
		style = styles[0]
	}
	if style == "default" {
		style = "clean"
	}
	styleMap := map[string]string{
		"clean":     "FontName=DejaVu Sans,FontSize=20,Outline=2,Shadow=0,MarginV=20",
		"bold":      "FontName=DejaVu Sans,Bold=1,FontSize=26,Outline=3,Shadow=1,MarginV=20",
		"minimal":   "FontName=DejaVu Sans,FontSize=18,Outline=1,Shadow=0,MarginV=14",
		"cinematic": "FontName=DejaVu Serif,FontSize=22,Spacing=1,Outline=2,Shadow=0,MarginV=24",
		"neon":      "FontName=DejaVu Sans,Bold=1,FontSize=24,PrimaryColour=&H00FFFF00,OutlineColour=&H00FF00FF,Outline=3,Shadow=0",
		"kids":      "FontName=DejaVu Sans,Bold=1,FontSize=26,PrimaryColour=&H0000FFFF,Outline=3,Shadow=1",
		"classic":   "FontName=DejaVu Serif,FontSize=20,Outline=1,Shadow=1",
		"comic":     "FontName=DejaVu Sans,Bold=1,FontSize=24,Outline=4,Shadow=2",
	}
	formatting, ok := styleMap[style]
	if !ok {
		return fmt.Errorf("caption style %q is unsupported; choose clean, bold, minimal, cinematic, neon, kids, classic or comic", style)
	}
	escaped := strings.NewReplacer("\\", "\\\\", ":", "\\:", "'", "\\'").Replace(subtitles)
	filter := "subtitles='" + escaped + "'"
	// Bundled FFmpeg has no dependency on a system font provider. Supply the
	// packaged font directory explicitly so captions work on a clean computer.
	fontDir := filepath.Join(filepath.Dir(m.ffmpegPath), "fonts")
	if info, err := os.Stat(filepath.Join(fontDir, "NotoSans-Regular.ttf")); err == nil && !info.IsDir() {
		escapedFontDir := strings.NewReplacer("\\", "\\\\", ":", "\\:", "'", "\\'").Replace(fontDir)
		filter += ":fontsdir='" + escapedFontDir + "'"
		fontName := "Noto Sans"
		// The CJK family includes Latin glyphs as well. Select it directly because
		// a build without a system font provider cannot discover fallback families.
		if info, err := os.Stat(filepath.Join(fontDir, "NotoSansCJKsc-Regular.otf")); err == nil && !info.IsDir() {
			fontName = "Noto Sans CJK SC"
		}
		formatting = strings.ReplaceAll(formatting, "FontName=DejaVu Sans", "FontName="+fontName)
		formatting = strings.ReplaceAll(formatting, "FontName=DejaVu Serif", "FontName="+fontName)
	}
	filter += ":force_style='" + formatting + "'"
	cmd := exec.CommandContext(ctx, m.ffmpegPath, "-y", "-i", videoPath, "-vf", filter, "-c:v", "libx264", "-c:a", "copy", destPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("burn captions: %w: %s", err, out)
	}
	return nil
}
