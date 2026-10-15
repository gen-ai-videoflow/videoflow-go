package cloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/pquerna/otp/totp"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func securityTestServer(t *testing.T) (*Server, User, string) {
	store, u := testPlatform(t)
	cfg := Config{DevMode: true, PublicOrigin: "http://localhost:5173", AuthEncryptionKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))}
	s := NewServer(store, cfg)
	_, token, e := store.Login(context.Background(), u.Email, "Test-password-123!")
	if e != nil {
		t.Fatal(e)
	}
	return s, u, token
}
func securityRequest(s *Server, method, path, token string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, "/v1"+path, bytes.NewReader(b))
	req.Header.Set("Origin", s.Config.PublicOrigin)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "videoflow_session", Value: token})
	}
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, req)
	return w
}
func securityCookie(w *httptest.ResponseRecorder) string {
	for _, c := range w.Result().Cookies() {
		if c.Name == "videoflow_session" {
			return c.Value
		}
	}
	return ""
}
func TestSecurityTOTPEnrollmentMFARecoveryAndReset(t *testing.T) {
	s, u, token := securityTestServer(t)
	ctx := context.Background()
	w := securityRequest(s, "POST", "/security/totp/begin", token, map[string]string{"password": "Test-password-123!"})
	if w.Code != 200 {
		t.Fatalf("begin %d %s", w.Code, w.Body.String())
	}
	var begin map[string]string
	json.Unmarshal(w.Body.Bytes(), &begin)
	if begin["secret"] == "" {
		t.Fatal("missing secret")
	}
	var encrypted string
	s.Store.DB.QueryRowContext(ctx, `SELECT secret_ciphertext FROM cloud_totp WHERE user_id=$1`, u.ID).Scan(&encrypted)
	if strings.Contains(encrypted, begin["secret"]) {
		t.Fatal("unencrypted secret")
	}
	if _, e := s.decryptTOTP("other-user", encrypted); e == nil {
		t.Fatal("ciphertext not userbound")
	}
	code, _ := totp.GenerateCode(begin["secret"], time.Now())
	w = securityRequest(s, "POST", "/security/totp/confirm", token, map[string]string{"code": code})
	if w.Code != 200 {
		t.Fatalf("confirm %d %s", w.Code, w.Body.String())
	}
	var result struct {
		Codes []string `json:"recovery_codes"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Codes) != 10 {
		t.Fatal("missing recovery codes")
	}
	if _, e := s.Store.Authenticate(ctx, token); e == nil {
		t.Fatal("old session survived enrollment")
	}
	var count int
	s.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_recovery_codes WHERE user_id=$1 AND code_hash=$2`, u.ID, result.Codes[0]).Scan(&count)
	if count != 0 {
		t.Fatal("plaintext recovery stored")
	}
	if _, _, e := s.Store.Login(ctx, u.Email, "Test-password-123!"); e != ErrMFARequired {
		t.Fatal("Store.Login bypassed MFA", e)
	}
	login := func() string {
		w := securityRequest(s, "POST", "/auth/login", "", map[string]string{"email": u.Email, "password": "Test-password-123!"})
		if w.Code != 202 || securityCookie(w) != "" {
			t.Fatalf("password issued fullsession %d %s", w.Code, w.Body.String())
		}
		var b struct {
			Challenge string `json:"challenge"`
		}
		json.Unmarshal(w.Body.Bytes(), &b)
		return b.Challenge
	}
	challenge := login()
	w = securityRequest(s, "POST", "/auth/mfa", "", map[string]string{"challenge": challenge, "code": code})
	if w.Code != 401 {
		t.Fatal("enrollment code replay accepted")
	}
	w = securityRequest(s, "POST", "/auth/mfa", "", map[string]string{"challenge": challenge, "code": result.Codes[0]})
	if w.Code != 200 || securityCookie(w) == "" {
		t.Fatalf("recovery login %d %s", w.Code, w.Body.String())
	}
	for _, c := range []string{challenge, login()} {
		w = securityRequest(s, "POST", "/auth/mfa", "", map[string]string{"challenge": c, "code": result.Codes[0]})
		if w.Code != 401 {
			t.Fatal("recovery replay accepted")
		}
	}
	freshToken := securityCookie(securityRequest(s, "POST", "/auth/mfa", "", map[string]string{"challenge": login(), "code": result.Codes[1]}))
	if freshToken == "" {
		t.Fatal("second recovery failed")
	}
	w = securityRequest(s, "POST", "/security/totp/disable", freshToken, map[string]string{"password": "Test-password-123!", "code": result.Codes[2]})
	if w.Code != 401 {
		t.Fatal("recovery code disabled MFA")
	}
	reset, e := s.Store.IssueAccountToken(ctx, u.Email, "reset_password")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Store.ResetPassword(ctx, reset, "New-test-password-123!"); e != nil {
		t.Fatal(e)
	}
	required, e := s.Store.mfaRequired(ctx, u.ID)
	if e != nil || !required {
		t.Fatal("reset disabled MFA")
	}
	if _, e = s.Store.Authenticate(ctx, freshToken); e == nil {
		t.Fatal("reset preserved old session")
	}
}
func TestSecurityTOTPReplayAndMissingEncryptionFailClosed(t *testing.T) {
	s, u, _ := securityTestServer(t)
	ctx := context.Background()
	secret := "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	encrypted, e := s.encryptTOTP(u.ID, secret)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_totp(user_id,secret_ciphertext,enabled)VALUES($1,$2,true)`, u.ID, encrypted); e != nil {
		t.Fatal(e)
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	for i := 0; i < 2; i++ {
		tx, e := s.Store.DB.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		e = s.verifyFactorTx(ctx, tx, u.ID, code, false)
		if i == 0 && e != nil {
			t.Fatal(e)
		}
		if i == 1 && e == nil {
			t.Fatal("TOTP replay accepted")
		}
		if e == nil {
			e = tx.Commit()
		} else {
			tx.Rollback()
		}
		if i == 0 && e != nil {
			t.Fatal(e)
		}
	}
	s.Config.AuthEncryptionKey = ""
	tx, _ := s.Store.DB.BeginTx(ctx, nil)
	defer tx.Rollback()
	if e = s.verifyFactorTx(ctx, tx, u.ID, code, false); e == nil {
		t.Fatal("missing encryption key bypassed MFA")
	}
}
func TestSecurityMFAChallengeExpiresAndRecoveryConsumesConcurrently(t *testing.T) {
	s, u, _ := securityTestServer(t)
	ctx := context.Background()
	secret := "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	encrypted, _ := s.encryptTOTP(u.ID, secret)
	if _, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_totp(user_id,secret_ciphertext,enabled)VALUES($1,$2,true)`, u.ID, encrypted); e != nil {
		t.Fatal(e)
	}
	_, _, challenge, e := s.Store.beginSecurityLogin(ctx, u.Email, "Test-password-123!")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.ExecContext(ctx, `UPDATE cloud_auth_challenges SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, tokenHash(challenge)); e != nil {
		t.Fatal(e)
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	w := securityRequest(s, "POST", "/auth/mfa", "", map[string]string{"challenge": challenge, "code": code})
	if w.Code != 401 {
		t.Fatal("expired MFA challenge accepted")
	}
	_, _, challenge, e = s.Store.beginSecurityLogin(ctx, u.Email, "Test-password-123!")
	if e != nil {
		t.Fatal(e)
	}
	w = securityRequest(s, "POST", "/auth/mfa", "", map[string]string{"challenge": challenge, "code": code})
	if w.Code != 200 || securityCookie(w) == "" {
		t.Fatalf("TOTP signin %d %s", w.Code, w.Body.String())
	}
	raw := "SINGLE-USE-RECOVERY-TEST"
	if _, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_recovery_codes(user_id,code_hash)VALUES($1,$2)`, u.ID, tokenHash(raw)); e != nil {
		t.Fatal(e)
	}
	challenges := []string{}
	for i := 0; i < 2; i++ {
		_, _, c, e := s.Store.beginSecurityLogin(ctx, u.Email, "Test-password-123!")
		if e != nil {
			t.Fatal(e)
		}
		challenges = append(challenges, c)
	}
	results := make(chan int, 2)
	for _, c := range challenges {
		go func(c string) {
			results <- securityRequest(s, "POST", "/auth/mfa", "", map[string]string{"challenge": c, "code": raw}).Code
		}(c)
	}
	ok := 0
	for range challenges {
		status := <-results
		if status == 200 {
			ok++
		} else if status != 401 {
			t.Fatalf("unexpected race status%d", status)
		}
	}
	if ok != 1 {
		t.Fatalf("recovery granted%d concurrent sessions", ok)
	}
}
func TestSecurityRecentUVPasskeyCanDisableLostTOTP(t *testing.T) {
	s, u, token := securityTestServer(t)
	ctx := context.Background()
	secret := "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	encrypted, _ := s.encryptTOTP(u.ID, secret)
	if _, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_totp(user_id,secret_ciphertext,enabled)VALUES($1,$2,true)`, u.ID, encrypted); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_passkeys(id,user_id,credential,name)VALUES($1,$2,'{}','UV proof fixture')`, "uv-proof-"+u.ID, u.ID); e != nil {
		t.Fatal(e)
	}
	w := securityRequest(s, "POST", "/security/totp/disable", token, map[string]string{"password": "Test-password-123!"})
	if w.Code != 401 {
		t.Fatal("password-only session disabled MFA")
	}
	if _, e := s.Store.DB.ExecContext(ctx, `UPDATE cloud_sessions SET passkey_verified_at=now()-interval '6 minutes' WHERE token_hash=$1`, tokenHash(token)); e != nil {
		t.Fatal(e)
	}
	w = securityRequest(s, "POST", "/security/totp/disable", token, map[string]string{"password": "Test-password-123!"})
	if w.Code != 401 {
		t.Fatal("stale passkey proof disabled MFA")
	}
	if _, e := s.Store.DB.ExecContext(ctx, `UPDATE cloud_sessions SET passkey_verified_at=now() WHERE token_hash=$1`, tokenHash(token)); e != nil {
		t.Fatal(e)
	}
	w = securityRequest(s, "POST", "/security/totp/disable", token, map[string]string{"password": "wrong-password"})
	if w.Code != 401 {
		t.Fatal("passkey proof bypassed password")
	}
	w = securityRequest(s, "POST", "/security/totp/disable", token, map[string]string{"password": "Test-password-123!"})
	if w.Code != 200 || securityCookie(w) == "" {
		t.Fatalf("freshUV couldnotdisablelostTOTP %d %s", w.Code, w.Body.String())
	}
	var enabled bool
	s.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_totp WHERE user_id=$1 AND enabled)`, u.ID).Scan(&enabled)
	if enabled {
		t.Fatal("TOTP remainedenabled")
	}
	if _, e := s.Store.Authenticate(ctx, token); e == nil {
		t.Fatal("old session survived disable")
	}
	required, e := s.Store.mfaRequired(ctx, u.ID)
	if e != nil || !required {
		t.Fatal("disabling TOTP removed passkey MFA")
	}
}
func TestSecurityTOTPAccountWithPasskeyCanReauthenticateWithTOTP(t *testing.T) {
	s, u, token := securityTestServer(t)
	ctx := context.Background()
	secret := "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	encrypted, _ := s.encryptTOTP(u.ID, secret)
	if _, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_totp(user_id,secret_ciphertext,enabled)VALUES($1,$2,true)`, u.ID, encrypted); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_passkeys(id,user_id,credential,name)VALUES($1,$2,'{}','Unavailable passkey')`, "unavailable-"+u.ID, u.ID); e != nil {
		t.Fatal(e)
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	w := securityRequest(s, "POST", "/security/totp/disable", token, map[string]string{"password": "Test-password-123!", "code": code})
	if w.Code != 200 {
		t.Fatalf("freshTOTP didnot satisfyfactor %d %s", w.Code, w.Body.String())
	}
}
