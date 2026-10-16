package cloud

import (
	"context"
	"testing"
)

func TestMagicLoginExpiryReplayAndMFA(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	token, err := store.IssueAccountToken(ctx, u.Email, "magic_login")
	if err != nil {
		t.Fatal(err)
	}
	got, session, challenge, err := store.consumeMagicLogin(ctx, token)
	if err != nil || got.ID != u.ID || session == "" || challenge != "" {
		t.Fatalf("plain sign-in %v %q %q", err, session, challenge)
	}
	if _, _, _, err = store.consumeMagicLogin(ctx, token); err == nil {
		t.Fatal("magic token replay accepted")
	}
	expired, err := store.IssueAccountToken(ctx, u.Email, "magic_login")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.ExecContext(ctx, `UPDATE cloud_tokens SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, tokenHash(expired)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = store.consumeMagicLogin(ctx, expired); err == nil {
		t.Fatal("expired magic token accepted")
	}
	if _, err = store.DB.ExecContext(ctx, `INSERT INTO cloud_totp(user_id,secret_ciphertext,enabled)VALUES($1,'test-only',true)`, u.ID); err != nil {
		t.Fatal(err)
	}
	guarded, err := store.IssueAccountToken(ctx, u.Email, "magic_login")
	if err != nil {
		t.Fatal(err)
	}
	_, session, challenge, err = store.consumeMagicLogin(ctx, guarded)
	if err != nil || session != "" || challenge == "" {
		t.Fatalf("magic link bypassed MFA %v %q %q", err, session, challenge)
	}
	if _, _, _, err = store.consumeMagicLogin(ctx, guarded); err == nil {
		t.Fatal("MFA magic token replay accepted")
	}
}
func TestDevelopmentProfileImageValidation(t *testing.T) {
	server := &Server{Config: Config{DevMode: true, PublicOrigin: "http://localhost:5173"}}
	local := "http://localhost:5173/api/v1/profile-images/20553229-0260-416b-9f56-dc831960b5b2"
	if !server.validateProfilePatch(profilePatch{Avatar: &local}) {
		t.Fatal("local owned image URL rejected")
	}
	for _, bad := range []string{"http://evil.example/api/v1/profile-images/20553229-0260-416b-9f56-dc831960b5b2", "http://localhost:5173/anything.png", "http://localhost:5173/api/v1/profile-images/not-an-id"} {
		if server.validateProfilePatch(profilePatch{Avatar: &bad}) {
			t.Fatalf("unsafe HTTP image accepted %s", bad)
		}
	}
	server.Config.DevMode = false
	if server.validateProfilePatch(profilePatch{Avatar: &local}) {
		t.Fatal("production HTTP accepted")
	}
}
