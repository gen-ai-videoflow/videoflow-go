package cloud

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestNormalizeProfileImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 128, 64))
	img.Set(4, 4, color.White)
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		t.Fatal(err)
	}
	data, err := normalizeProfileImage(pngData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width != 128 || cfg.Height != 64 {
		t.Fatalf("invalid normalized JPEG %v %v", cfg, err)
	}
	for _, bad := range [][]byte{[]byte("<svg onload=evil/>"), []byte("not an image"), nil} {
		if _, err := normalizeProfileImage(bad); err == nil {
			t.Fatal("accepted invalid image")
		}
	}
	var tiny bytes.Buffer
	png.Encode(&tiny, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	if _, err := normalizeProfileImage(tiny.Bytes()); err == nil {
		t.Fatal("accepted undersized image")
	}
}
func TestPresetSocialLinksMatchTheirPlatform(t *testing.T) {
	for _, v := range []struct {
		link ProfileLink
		want bool
	}{{ProfileLink{"YouTube", "https://youtube.com/@name"}, true}, {ProfileLink{"YouTube", "https://youtube.com.evil.test/@name"}, false}, {ProfileLink{"TikTok", "https://example.test/@name"}, false}, {ProfileLink{"Portfolio", "https://example.test"}, true}, {ProfileLink{"X", "https://twitter.com/name"}, true}} {
		p := profilePatch{Links: &[]ProfileLink{v.link}}
		if p.validate() != v.want {
			t.Errorf("platform link %q %q expected %v", v.link.Label, v.link.URL, v.want)
		}
	}
}
