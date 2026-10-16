package cloud

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

func TestOIDCSignatureAudienceNonceAndExpiry(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/jwks":
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "test-key", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL
	provider, err := oidc.NewProvider(context.Background(), issuer)
	if err != nil {
		t.Fatal(err)
	}
	p := oauthSettings{Issuer: issuer, ClientID: "videoflow-test"}
	sign := func(aud, nonce, iss string, expiry time.Time) string {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": iss, "aud": aud, "sub": "provider-user-123", "email": "verified@example.test", "email_verified": true, "nonce": nonce, "iat": time.Now().Unix(), "exp": expiry.Unix()})
		token.Header["kid"] = "test-key"
		raw, e := token.SignedString(key)
		if e != nil {
			t.Fatal(e)
		}
		return raw
	}
	valid := sign(p.ClientID, "expected-nonce", issuer, time.Now().Add(time.Minute))
	if subject, _, err := verifyOAuthToken(context.Background(), provider, p, valid, tokenHash("expected-nonce")); err != nil || subject != "provider-user-123" {
		t.Fatalf("valid OIDC denied %v", err)
	}
	for name, raw := range map[string]string{"audience": sign("attacker-client", "expected-nonce", issuer, time.Now().Add(time.Minute)), "nonce": sign(p.ClientID, "wrong-nonce", issuer, time.Now().Add(time.Minute)), "issuer": sign(p.ClientID, "expected-nonce", "https://evil.example", time.Now().Add(time.Minute)), "expired": sign(p.ClientID, "expected-nonce", issuer, time.Now().Add(-time.Hour)), "unsigned": "eyJhbGciOiJub25lIn0.eyJzdWIiOiJhdHRhY2tlciJ9."} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := verifyOAuthToken(context.Background(), provider, p, raw, tokenHash("expected-nonce")); err == nil {
				t.Fatal("invalid identity token accepted")
			}
		})
	}
}
func TestOAuthStateBindingReplayAndIdentityOwnership(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	server := &Server{Store: store}
	state, _ := rawToken()
	browser, _ := rawToken()
	if _, err := store.DB.ExecContext(ctx, `INSERT INTO cloud_oauth_states(token_hash,provider,browser_hash,nonce_hash,verifier_ciphertext)VALUES($1,'google',$2,$3,'cipher')`, tokenHash(state), tokenHash(browser), tokenHash("nonce")); err != nil {
		t.Fatal(err)
	}
	wrongBrowser, _ := rawToken()
	if _, err := server.consumeOAuthState(ctx, state, wrongBrowser, "google"); err == nil {
		t.Fatal("wrong browser accepted")
	}
	wrongState, _ := rawToken()
	if _, err := server.consumeOAuthState(ctx, wrongState, browser, "google"); err == nil {
		t.Fatal("wrong state accepted")
	}
	if _, err := server.consumeOAuthState(ctx, state, browser, "apple"); err == nil {
		t.Fatal("wrong provider accepted")
	}
	if _, err := server.consumeOAuthState(ctx, state, browser, "google"); err != nil {
		t.Fatal(err)
	}
	if _, err := server.consumeOAuthState(ctx, state, browser, "google"); err == nil {
		t.Fatal("state replay accepted")
	}
	state, _ = rawToken()
	if _, err := store.DB.ExecContext(ctx, `INSERT INTO cloud_oauth_states(token_hash,provider,browser_hash,nonce_hash,verifier_ciphertext,expires_at)VALUES($1,'google',$2,$3,'cipher',now()-interval '1 minute')`, tokenHash(state), tokenHash(browser), tokenHash("nonce")); err != nil {
		t.Fatal(err)
	}
	if _, err := server.consumeOAuthState(ctx, state, browser, "google"); err == nil {
		t.Fatal("expired state accepted")
	}
	// A verified provider email collision must not attach a new provider subject.
	if _, err := store.oauthIdentity(ctx, "google", "unlinked-subject-"+u.ID, oauthClaims{Email: u.Email}); err != ErrConflict {
		t.Fatalf("existing account auto merged: %v", err)
	}
	if err := store.linkOAuthIdentity(ctx, u.ID, "google", "owned-subject-"+u.ID); err != nil {
		t.Fatal(err)
	}
	other, err := store.Register(ctx, "other-oauth-"+u.ID+"@example.test", "Long-password-123!", "Other", "oauth_"+u.ID[:8])
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.ExecContext(ctx, `DELETE FROM cloud_users WHERE id=$1`, other.ID)
	if err = store.linkOAuthIdentity(ctx, other.ID, "google", "owned-subject-"+u.ID); err == nil {
		t.Fatal("provider identity moved to another account")
	}
	got, err := store.oauthIdentity(ctx, "google", "owned-subject-"+u.ID, oauthClaims{Email: other.Email})
	if err != nil || got.ID != u.ID {
		t.Fatalf("provider subject ownership lost %v %s", err, got.ID)
	}

	created, err := store.oauthIdentity(ctx, "apple", "new-apple-"+u.ID, oauthClaims{Email: "new-oauth-" + u.ID + "@example.test", Name: "New member"})
	if err != nil || !created.Verified || created.WorkspaceID == "" || created.ChannelID == "" {
		t.Fatalf("new provider account failed: %v", err)
	}
	newSession, newChallenge, err := store.oauthSession(ctx, created)
	if err != nil || newSession == "" || newChallenge != "" {
		t.Fatalf("new account session failed: %v", err)
	}
	if authenticated, err := store.Authenticate(ctx, newSession); err != nil || authenticated.ID != created.ID {
		t.Fatalf("new provider session not authenticatable %v", err)
	}
	// Linking must remain bound to the exact still-active session that passed reauthentication.
	session, err := store.newSecuritySession(ctx, u.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	pending := oauthPending{LinkUserID: sql.NullString{String: u.ID, Valid: true}, LinkSessionHash: sql.NullString{String: tokenHash(session), Valid: true}}
	if valid, err := server.validOAuthLink(ctx, pending); err != nil || !valid {
		t.Fatalf("valid link proof denied %v", err)
	}
	pending.LinkUserID.String = other.ID
	if valid, err := server.validOAuthLink(ctx, pending); err != nil || valid {
		t.Fatalf("foreign session accepted %v", err)
	}
	pending.LinkUserID.String = u.ID
	if err = store.Logout(ctx, session); err != nil {
		t.Fatal(err)
	}
	if valid, err := server.validOAuthLink(ctx, pending); err != nil || valid {
		t.Fatalf("revoked link session accepted %v", err)
	}

}
