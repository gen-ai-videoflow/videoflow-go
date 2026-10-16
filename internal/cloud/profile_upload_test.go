package cloud

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http/httptest"
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
func TestProfileImageCapacityRejectsBeforeReadingBody(t *testing.T) {
	server := NewServer(nil, Config{DevMode: true, OriginalsBucket: "test"})
	server.Media = &Media{}
	server.imagePermits <- struct{}{}
	server.imagePermits <- struct{}{}
	req := httptest.NewRequest("POST", "/v1/profile/images", nil)
	req.Body = panicImageReader{}
	response := httptest.NewRecorder()
	server.uploadProfileImage(response, req)
	if response.Code != 503 || response.Header().Get("Retry-After") == "" {
		t.Fatalf("expected bounded image processing admission, got%d", response.Code)
	}
}

type panicImageReader struct{}

func (panicImageReader) Read([]byte) (int, error) {
	panic("overloaded upload must not decode or buffer input")
}
func (panicImageReader) Close() error { return nil }

func TestBannerPresetWhitelist(t *testing.T) {
	server := &Server{}
	for _, preset := range []string{"/banners/coastal-dawn.webp", "/banners/paper-studio.webp", "/banners/forest-light.webp", "/banners/after-hours.webp"} {
		if !server.validateProfilePatch(profilePatch{Banner: &preset}) {
			t.Fatalf("rejected preset %s", preset)
		}
	}
	for _, bad := range []string{"/banners/../private/file.webp", "/banners/other.webp", "//attacker.example/image.webp", "/banners/coastal-dawn.webp?url=evil"} {
		if server.validateProfilePatch(profilePatch{Banner: &bad}) {
			t.Fatalf("accepted unlisted path %s", bad)
		}
	}
}
