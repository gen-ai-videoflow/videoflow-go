package cloud

import (
	"strings"
	"testing"
)

func TestWebVTTEscapesMarkupAndPreservesTiming(t *testing.T) {
	s, e := captionsVTT([]byte(`[{"start":1.25,"end":3.5,"text":"<script> & hello --> world"}]`))
	if e != nil || !strings.Contains(s, "00:00:01.250 --> 00:00:03.500") || strings.Contains(s, "<script>") || !strings.Contains(s, "&lt;script&gt;") {
		t.Fatalf("%s %v", s, e)
	}
	if _, e = captionsVTT([]byte(`[{"start":2,"end":1,"text":"bad"}]`)); e == nil {
		t.Fatal("invalid timing accepted")
	}
}
