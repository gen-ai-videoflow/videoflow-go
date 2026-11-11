package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type clipInfo struct {
	Duration float64
	Audio    bool
}

func (m *MediaEngine) probeClip(ctx context.Context, path string) (clipInfo, error) {
	var info clipInfo
	probe := filepath.Join(filepath.Dir(m.ffmpegPath), "ffprobe")
	if _, err := os.Stat(probe); err != nil {
		probe, err = exec.LookPath("ffprobe")
		if err != nil {
			return info, fmt.Errorf("FFprobe is required to assemble clips")
		}
	}
	b, err := exec.CommandContext(ctx, probe, "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm", "-show_streams", "-show_format", "-of", "json", path).Output()
	if err != nil || len(b) > 1<<20 {
		return info, fmt.Errorf("could not inspect video clip")
	}
	var raw struct {
		Streams []struct {
			Type     string `json:"codec_type"`
			Duration string `json:"duration"`
		}
		Format struct{ Duration string }
	}
	if json.Unmarshal(b, &raw) != nil {
		return info, fmt.Errorf("invalid video metadata")
	}
	video := false
	for _, s := range raw.Streams {
		info.Audio = info.Audio || s.Type == "audio"
		video = video || s.Type == "video"
		if s.Type == "video" {
			info.Duration, _ = strconv.ParseFloat(s.Duration, 64)
		}
	}
	if info.Duration <= 0 {
		info.Duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	}
	if !video || info.Duration <= 0 || info.Duration > 600 {
		return info, fmt.Errorf("each input must be a video clip of at most 600 seconds")
	}
	return info, nil
}

// Assemble encodes each clip once to matching dimensions/audio, then concatenates without re-encoding.
func (m *MediaEngine) Assemble(ctx context.Context, inputs []string, dest, aspect string) error {
	if !m.Available() {
		return fmt.Errorf("FFmpeg is required to assemble clips")
	}
	if len(inputs) < 2 || len(inputs) > 20 {
		return fmt.Errorf("choose 2 to 20 clips")
	}
	width, height := 1280, 720
	switch aspect {
	case "9:16":
		width, height = 720, 1280
	case "1:1":
		width, height = 720, 720
	case "16:9":
	default:
		return fmt.Errorf("choose 9:16, 16:9 or 1:1")
	}
	release, err := m.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err = os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(filepath.Dir(dest), "assemble-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	var list strings.Builder
	total := float64(0)
	for i, input := range inputs {
		info, err := m.probeClip(ctx, input)
		if err != nil {
			return err
		}
		total += info.Duration
		if total > 1200 {
			return fmt.Errorf("combined video exceeds 20 minutes")
		}
		name := fmt.Sprintf("clip-%03d.mp4", i)
		target := filepath.Join(dir, name)
		args := []string{"-v", "error", "-y", "-protocol_whitelist", "file,pipe", "-i", input}
		if !info.Audio {
			args = append(args, "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo")
		}
		args = append(args, "-map", "0:v:0")
		if info.Audio {
			args = append(args, "-map", "0:a:0")
		} else {
			args = append(args, "-map", "1:a:0")
		}
		filter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=30,format=yuv420p", width, height, width, height)
		args = append(args, "-vf", filter, "-c:v", "libx264", "-preset", "veryfast", "-threads", "2", "-crf", "23", "-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2", "-af", "aresample=async=1:first_pts=0,apad", "-t", strconv.FormatFloat(info.Duration, 'f', 3, 64), "-shortest", target)
		if _, err = exec.CommandContext(ctx, m.ffmpegPath, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("could not normalize clip %d: %w", i+1, err)
		}
		fmt.Fprintf(&list, "file '%s'\n", name)
	}
	manifest := filepath.Join(dir, "inputs.txt")
	if err = os.WriteFile(manifest, []byte(list.String()), 0600); err != nil {
		return err
	}
	if _, err = exec.CommandContext(ctx, m.ffmpegPath, "-v", "error", "-y", "-f", "concat", "-safe", "1", "-i", manifest, "-c", "copy", "-movflags", "+faststart", dest).CombinedOutput(); err != nil {
		os.Remove(dest)
		return fmt.Errorf("could not assemble video: %w", err)
	}
	return nil
}
func (m *MediaEngine) acquire(ctx context.Context) (func(), error) {
	m.transcodeOnce.Do(func() {
		if m.transcode == nil {
			m.transcode = make(chan struct{}, 1)
		}
	})
	select {
	case m.transcode <- struct{}{}:
		return func() { <-m.transcode }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
