package cloud

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

func captionClock(seconds float64) string {
	ms := int64(math.Round(seconds * 1000))
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, (ms/60000)%60, (ms/1000)%60, ms%1000)
}
func captionsVTT(raw []byte) (string, error) {
	var segments []CaptionSegment
	if json.Unmarshal(raw, &segments) != nil {
		return "", ErrValidation
	}
	var out strings.Builder
	out.WriteString("WEBVTT\n\n")
	for i, c := range segments {
		if c.Start < 0 || c.End <= c.Start || math.IsNaN(c.Start) || math.IsNaN(c.End) || math.IsInf(c.Start, 0) || math.IsInf(c.End, 0) || len(c.Text) > 2000 {
			return "", ErrValidation
		}
		text := strings.TrimSpace(c.Text)
		text = strings.ReplaceAll(text, "\r", "")
		text = strings.ReplaceAll(text, "-->", "→")
		text = strings.ReplaceAll(text, "\n\n", "\n")
		text = strings.ReplaceAll(text, "&", "&amp;")
		text = strings.ReplaceAll(text, "<", "&lt;")
		text = strings.ReplaceAll(text, ">", "&gt;")
		fmt.Fprintf(&out, "%d\n%s --> %s\n%s\n\n", i+1, captionClock(c.Start), captionClock(c.End), text)
	}
	return out.String(), nil
}
