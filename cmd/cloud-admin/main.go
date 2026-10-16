package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
	"6eek-go/internal/cloud"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: cloud-admin report-list|block-video|resolve-job|reward-close|payout-prepare|payout-execute|crypto-inspect|crypto-recover-address|crypto-replay|crypto-report")
	}
	flags := flag.NewFlagSet(os.Args[1], flag.ContinueOnError)
	id := flags.String("id", "", "resource or batch ID")
	reason := flags.String("reason", "", "moderation reason")
	address := flags.String("address", "", "provider-confirmed recovered deposit address")
	signature := flags.String("signature", "", "original CryptoRails webhook HMAC signature")
	provider := flags.String("provider-id", "", "reconciled provider job ID")
	fail := flags.Bool("fail", false, "explicitly fail an ambiguous job and release its reserved credits")
	input := flags.String("input", "", "approved reward period JSON input")
	execute := flags.Bool("execute", false, "explicitly execute real money transfer")
	actor := flags.String("actor", os.Getenv("AWS_ROLE_ARN"), "audited operator identity (normally AWS_ROLE_ARN)")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *actor == "" {
		return fmt.Errorf("operator --actor or AWS_ROLE_ARN required")
	}
	cfg, err := cloud.EnvConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	store, err := cloud.OpenStore(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if cfg.DevMode {
		err = store.Migrate(ctx)
	} else {
		err = store.CheckMigrations(ctx)
	}
	if err != nil {
		return err
	}
	var result any = map[string]bool{"ok": true}
	switch os.Args[1] {
	case "crypto-report":
		result, err = store.AuditedCryptoPaymentHealth(ctx, *actor)
	case "crypto-inspect", "crypto-recover-address", "crypto-replay":
		server := cloud.NewServer(store, cfg)
		defer server.Close()
		switch os.Args[1] {
		case "crypto-inspect":
			result, err = server.InspectCryptoPayment(ctx, *id, *actor)
		case "crypto-recover-address":
			err = server.RecoverCryptoAddress(ctx, *id, *address, *actor, *reason)
		case "crypto-replay":
			var raw []byte
			raw, err = os.ReadFile(*input)
			if err == nil {
				err = server.ReplayCryptoWebhook(ctx, raw, *signature, *actor)
			}
		}
	case "report-list":
		result, err = store.OpenReports(ctx)
	case "block-video":
		err = store.BlockVideo(ctx, *id, *reason, *actor)
	case "resolve-job":
		err = store.ResolveProviderJob(ctx, *id, *provider, *actor, *fail)
	case "reward-close":
		var body struct {
			Period       string           `json:"period"`
			Contribution int64            `json:"contribution_cents"`
			RewardBPS    int64            `json:"reward_bps"`
			Cap          int64            `json:"cap_cents"`
			Weights      map[string]int64 `json:"weights"`
		}
		file, e := os.Open(*input)
		if e != nil {
			return e
		}
		defer file.Close()
		decoder := json.NewDecoder(file)
		decoder.DisallowUnknownFields()
		if e = decoder.Decode(&body); e != nil {
			return e
		}
		err = store.CloseRewardPeriod(ctx, body.Period, body.Contribution, body.RewardBPS, body.Cap, body.Weights)
	case "payout-prepare":
		result, err = store.PreparePayout(ctx, *id, *actor)
	case "payout-execute":
		server := cloud.NewServer(store, cfg)
		defer server.Close()
		err = server.ExecutePayout(ctx, *id, *actor, *execute)
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
