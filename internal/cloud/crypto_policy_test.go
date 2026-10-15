package cloud

import (
	"context"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCryptoLivePolicyRejectsFeeAndAssetDrift(t *testing.T) {
	cfg := Config{CryptoRailsAPIURL: "https://provider.example/v1", CryptoRailsAPIKey: "secret", CryptoRailsVMType: "EVM", CryptoRailsTokenDecimals: 6, CryptoRailsTokenAddress: "0xasset", CryptoRailsTreasuryAddress: "0xtreasury", CryptoRailsNetwork: "base-mainnet", CryptoRailsToken: "USDC", CryptoRailsDepositFixed: "0.10", CryptoRailsDepositFeeBPS: 100}
	server := NewServer(nil, cfg)
	defer server.Close()
	body := `{"data":[{"isActive":true,"depositProviderFeeFixed":"0.10000","depositProviderFeePer":"1.000","tenantConsolidationAddress":"0xTREASURY","tokenConfig":{"name":"USDC","decimal":6,"tokenContractAddress":"0xASSET","blockchainName":"base-mainnet","isActive":true,"type":"ERC20"}}],"meta":{"hasNextPage":false}}`
	current := body
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/api/tenant-token-statuses" || r.URL.Query().Get("tokenAddress") != "0xasset" || r.Header.Get("x-tenant-api-key") != "secret" {
			t.Fatalf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(current))}, nil
	})}
	if err := server.validateLiveCryptoPolicy(context.Background()); err != nil {
		t.Fatalf("valid policy %v", err)
	}
	current = `{"data":[{"isActive":true,"depositProviderFeeFixed":"0.10","depositProviderFeePer":"1","tenantConsolidationAddress":"0xtreasury"}],"meta":{"hasNextPage":false}}`
	if err := server.validateLiveCryptoPolicy(context.Background()); err != nil {
		t.Fatalf("documented unexpanded response rejected: %v", err)
	}
	for _, change := range [][2]string{{`"1.000"`, `"1.001"`}, {`"0.10000"`, `"0.10001"`}, {`"0xASSET"`, `"0xwrong"`}, {`"0xTREASURY"`, `"0xwrong"`}, {`"decimal":6`, `"decimal":2`}, {`"isActive":true`, `"isActive":false`}, {`"hasNextPage":false`, `"hasNextPage":true`}, {`"base-mainnet"`, `"eth-mainnet"`}} {
		current = strings.Replace(body, change[0], change[1], 1)
		if err := server.validateLiveCryptoPolicy(context.Background()); err == nil {
			t.Fatalf("accepted drift %s", change[1])
		}
	}
}

func TestCryptoClientNeverFollowsCredentialRedirect(t *testing.T) {
	leaked := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true; w.WriteHeader(200) }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer origin.Close()
	server := NewServer(nil, Config{CryptoRailsAPIURL: origin.URL, CryptoRailsAPIKey: "secret"})
	defer server.Close()
	status, _, err := server.cryptoCall(context.Background(), http.MethodGet, "/api/transaction", nil, nil)
	if err == nil || status != http.StatusFound || leaked {
		t.Fatalf("redirect followed or accepted: %d %v leaked=%v", status, err, leaked)
	}
}

func TestCryptoClientHonorsRetryAfterWithoutSleeping(t *testing.T) {
	server := NewServer(nil, Config{CryptoRailsAPIURL: "https://provider.example/v1"})
	defer server.Close()
	calls := 0
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"120"}}, Body: io.NopCloser(strings.NewReader(`{"message":"rate limit"}`))}, nil
	})}
	for i := 0; i < 2; i++ {
		status, _, err := server.cryptoCall(context.Background(), http.MethodGet, "/api/transaction", nil, nil)
		if status != 429 || err == nil {
			t.Fatalf("throttle not returned: %d %v", status, err)
		}
	}
	if calls != 1 {
		t.Fatalf("retried during provider cooldown: %d", calls)
	}
}

func TestCryptoProviderCooldownSharedThroughRedis(t *testing.T) {
	dsn := os.Getenv("CLOUD_TEST_REDIS_URL")
	if dsn == "" {
		t.Skip("Redis integration DSN required")
	}
	cfg := Config{RedisURL: dsn, CryptoRailsAPIURL: "https://provider.example/v1", CryptoRailsAPIKey: uuid.NewString()}
	first := NewServer(nil, cfg)
	second := NewServer(nil, cfg)
	defer first.Close()
	defer second.Close()
	ctx := context.Background()
	defer first.Redis.Del(ctx, first.cryptoCooldownKey())
	if first.cryptoCoolingDown(ctx) {
		t.Fatal("new key already in cooldown")
	}
	first.cryptoRecordCooldown(ctx, "120")
	if !first.cryptoCoolingDown(ctx) || !second.cryptoCoolingDown(ctx) {
		t.Fatal("cooldown not shared across replicas")
	}
}

func TestCryptoDisabledConfigurationAcceptsZeroPrecision(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example/database")
	t.Setenv("CLOUD_DEV_MODE", "true")
	t.Setenv("PUBLIC_ORIGIN", "http://localhost:5173")
	t.Setenv("CRYPTORAILS_ENABLED", "false")
	t.Setenv("CRYPTORAILS_TOKEN_DECIMALS", "0")
	cfg, err := EnvConfig()
	if err != nil {
		t.Fatalf("disabled deployment defaults rejected: %v", err)
	}
	if NewServer(nil, cfg).cryptoEnabled() {
		t.Fatal("disabled payment activated")
	}
}
