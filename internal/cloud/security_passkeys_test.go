package cloud

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"github.com/fxamacker/cbor/v2"
	"testing"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

type virtualPasskey struct {
	key  *ecdsa.PrivateKey
	id   []byte
	user User
}

func (v virtualPasskey) registration(challenge, origin string, uv bool) map[string]any {
	client, _ := json.Marshal(map[string]any{"type": "webauthn.create", "challenge": challenge, "origin": origin, "crossOrigin": false})
	rp := sha256.Sum256([]byte("localhost"))
	flags := byte(0x41)
	if uv {
		flags |= 4
	}
	auth := append(rp[:], flags, 0, 0, 0, 0)
	auth = append(auth, make([]byte, 16)...)
	auth = append(auth, byte(len(v.id)>>8), byte(len(v.id)))
	auth = append(auth, v.id...)
	cose, _ := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: v.key.X.FillBytes(make([]byte, 32)), -3: v.key.Y.FillBytes(make([]byte, 32))})
	auth = append(auth, cose...)
	att, _ := cbor.Marshal(map[string]any{"fmt": "none", "authData": auth, "attStmt": map[string]any{}})
	return map[string]any{"id": b64(v.id), "rawId": b64(v.id), "type": "public-key", "response": map[string]any{"clientDataJSON": b64(client), "attestationObject": b64(att), "transports": []string{"internal"}}, "clientExtensionResults": map[string]any{}}
}
func (v virtualPasskey) assertion(challenge, origin string, counter uint32, uv bool) map[string]any {
	client, _ := json.Marshal(map[string]any{"type": "webauthn.get", "challenge": challenge, "origin": origin, "crossOrigin": false})
	rp := sha256.Sum256([]byte("localhost"))
	flags := byte(1)
	if uv {
		flags |= 4
	}
	auth := append(rp[:], flags)
	auth = binary.BigEndian.AppendUint32(auth, counter)
	hash := sha256.Sum256(client)
	payload := append(append([]byte{}, auth...), hash[:]...)
	digest := sha256.Sum256(payload)
	signature, _ := ecdsa.SignASN1(rand.Reader, v.key, digest[:])
	return map[string]any{"id": b64(v.id), "rawId": b64(v.id), "type": "public-key", "response": map[string]any{"clientDataJSON": b64(client), "authenticatorData": b64(auth), "signature": b64(signature), "userHandle": b64([]byte(v.user.ID))}, "clientExtensionResults": map[string]any{}}
}
func securityOptions(t *testing.T, s *Server, path, token string, body any) (string, string) {
	t.Helper()
	w := securityRequest(s, "POST", path, token, body)
	if w.Code != 200 {
		t.Fatalf("options%d %s", w.Code, w.Body.String())
	}
	var out struct {
		Challenge string `json:"challenge"`
		Options   struct {
			Challenge string `json:"challenge"`
		} `json:"options"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Challenge == "" || out.Options.Challenge == "" {
		t.Fatal("missing challenges")
	}
	return out.Challenge, out.Options.Challenge
}
func TestSecurityPasskeyRegistrationAndVerifiedSignIn(t *testing.T) {
	s, u, token := securityTestServer(t)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	id := make([]byte, 32)
	rand.Read(id)
	v := virtualPasskey{key: key, id: id, user: u}
	challenge, protocolChallenge := securityOptions(t, s, "/security/passkeys/register/options", token, map[string]string{"password": "Test-password-123!"})
	w := securityRequest(s, "POST", "/security/passkeys/register/verify", token, map[string]any{"challenge": challenge, "credential": v.registration(protocolChallenge, s.Config.PublicOrigin, true), "name": "Virtual test authenticator"})
	if w.Code != 200 {
		t.Fatalf("registration%d %s", w.Code, w.Body.String())
	}
	token = securityCookie(w)
	if token == "" {
		t.Fatal("no registration session")
	}
	if _, _, e := s.Store.Login(t.Context(), u.Email, "Test-password-123!"); e != ErrMFARequired {
		t.Fatal("password bypassed passkey", e)
	}
	for _, test := range []struct {
		name, origin string
		counter      uint32
		uv           bool
		want         int
	}{{"valid", s.Config.PublicOrigin, 1, true, 200}, {"wrongorigin", "https://evil.example", 2, true, 401}, {"withoutUV", s.Config.PublicOrigin, 2, false, 401}, {"counterclone", s.Config.PublicOrigin, 1, true, 401}, {"validnext", s.Config.PublicOrigin, 2, true, 200}} {
		t.Run(test.name, func(t *testing.T) {
			challenge, protocolChallenge := securityOptions(t, s, "/auth/passkeys/options", "", map[string]any{})
			credential := v.assertion(protocolChallenge, test.origin, test.counter, test.uv)
			w := securityRequest(s, "POST", "/auth/passkeys/verify", "", map[string]any{"challenge": challenge, "credential": credential})
			if w.Code != test.want {
				t.Fatalf("got%d want%d %s", w.Code, test.want, w.Body.String())
			}
			if w.Code == 200 && securityCookie(w) == "" {
				t.Fatal("missing authenticated session")
			}
			again := securityRequest(s, "POST", "/auth/passkeys/verify", "", map[string]any{"challenge": challenge, "credential": credential})
			if again.Code != 401 {
				t.Fatal("challenge replay accepted")
			}
		})
	}
	_, other, otherToken := securityTestServer(t)
	_ = other
	w = securityRequest(s, "DELETE", "/security/passkeys/"+b64(id), otherToken, map[string]string{"password": "Test-password-123!"})
	if w.Code != 404 {
		t.Fatalf("foreign delete%d %s", w.Code, w.Body.String())
	}
}
func TestSecurityPasskeyEnrollmentRejectsMissingUVAndWrongUserChallenge(t *testing.T) {
	s, u, token := securityTestServer(t)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	id := make([]byte, 32)
	rand.Read(id)
	v := virtualPasskey{key: key, id: id, user: u}
	challenge, protocolChallenge := securityOptions(t, s, "/security/passkeys/register/options", token, map[string]string{"password": "Test-password-123!"})
	w := securityRequest(s, "POST", "/security/passkeys/register/verify", token, map[string]any{"challenge": challenge, "credential": v.registration(protocolChallenge, s.Config.PublicOrigin, false), "name": "No UV"})
	if w.Code != 401 {
		t.Fatalf("unverified registration%d %s", w.Code, w.Body.String())
	}
	_, _, otherToken := securityTestServer(t)
	challenge, protocolChallenge = securityOptions(t, s, "/security/passkeys/register/options", token, map[string]string{"password": "Test-password-123!"})
	w = securityRequest(s, "POST", "/security/passkeys/register/verify", otherToken, map[string]any{"challenge": challenge, "credential": v.registration(protocolChallenge, s.Config.PublicOrigin, true), "name": "Wrong user"})
	if w.Code != 401 {
		t.Fatal("crossuser challenge accepted")
	}
}
