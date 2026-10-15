package cloud

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	S3PublicEndpoint                                                               string
	AnalyticsBucket                                                                string
	CashPayoutsEnabled                                                             bool
	UploadsBucket, TrustedProxyCIDRs                                               string
	EncodingQueueURL                                                               string
	MembershipsEnabled, MembershipPolicyConfigured, ConnectEnabled, RewardsEnabled bool
	MembershipCommissionBPS, RewardBPS, RewardMonthlyCapCents                      int64
	SupportedPayoutCountries                                                       string
	ProviderDailyBudgetMicros, ProviderMaxCostMicros                               int64

	DatabaseURL, RedisURL, Region, OriginalsBucket, PlaybackBucket, QueueURL, MediaConvertRole, MediaConvertEndpoint, CloudFrontDomain, CloudFrontKeyPairID, CloudFrontPrivateKey, OpenRouterKey, VideoModel, PublicOrigin string
	StripeSecret, StripeWebhookSecret, MailFrom, MailEndpoint                                                                                                                                                              string
	DevMode                                                                                                                                                                                                                bool
	Port                                                                                                                                                                                                                   int
	CreditsPerSecond                                                                                                                                                                                                       int64
	BillingEnabled                                                                                                                                                                                                         bool
}

func EnvConfig() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), RedisURL: os.Getenv("REDIS_URL"), Region: os.Getenv("AWS_REGION"), OriginalsBucket: os.Getenv("S3_BUCKET_ORIGINALS"), PlaybackBucket: os.Getenv("S3_BUCKET_PLAYBACK"), QueueURL: os.Getenv("SQS_QUEUE_URL"), MediaConvertRole: os.Getenv("MEDIACONVERT_ROLE_ARN"), MediaConvertEndpoint: os.Getenv("MEDIACONVERT_ENDPOINT"), CloudFrontDomain: os.Getenv("CLOUDFRONT_DOMAIN"), CloudFrontKeyPairID: os.Getenv("CLOUDFRONT_KEY_PAIR_ID"), CloudFrontPrivateKey: os.Getenv("CLOUDFRONT_PRIVATE_KEY"), OpenRouterKey: os.Getenv("OPENROUTER_API_KEY"), VideoModel: os.Getenv("OPENROUTER_VIDEO_MODEL"), PublicOrigin: os.Getenv("PUBLIC_ORIGIN"), StripeSecret: os.Getenv("STRIPE_SECRET_KEY"), StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"), MailFrom: os.Getenv("MAIL_FROM"), MailEndpoint: os.Getenv("MAIL_ENDPOINT"), DevMode: os.Getenv("CLOUD_DEV_MODE") == "true", BillingEnabled: os.Getenv("BILLING_ENABLED") == "true", Port: 8000, CreditsPerSecond: 10}
	c.CashPayoutsEnabled = os.Getenv("CASH_PAYOUTS_ENABLED") == "true"
	c.S3PublicEndpoint = os.Getenv("S3_PUBLIC_ENDPOINT")
	c.AnalyticsBucket = os.Getenv("S3_BUCKET_ANALYTICS")
	c.UploadsBucket = os.Getenv("S3_BUCKET_UPLOADS")
	c.TrustedProxyCIDRs = os.Getenv("TRUSTED_PROXY_CIDRS")
	c.EncodingQueueURL = os.Getenv("ENCODING_QUEUE_URL")
	c.MembershipsEnabled = os.Getenv("MEMBERSHIPS_ENABLED") == "true"
	c.ConnectEnabled = os.Getenv("CONNECT_ENABLED") == "true"
	c.SupportedPayoutCountries = os.Getenv("SUPPORTED_PAYOUT_COUNTRIES")
	c.RewardsEnabled = os.Getenv("REWARDS_ENABLED") == "true"
	for key, target := range map[string]*int64{"MEMBERSHIP_COMMISSION_BPS": &c.MembershipCommissionBPS, "REWARD_BPS": &c.RewardBPS, "REWARD_MONTHLY_CAP_CENTS": &c.RewardMonthlyCapCents, "PROVIDER_DAILY_BUDGET_MICROS": &c.ProviderDailyBudgetMicros, "PROVIDER_MAX_COST_MICROS": &c.ProviderMaxCostMicros} {
		if raw := os.Getenv(key); raw != "" {
			n, e := strconv.ParseInt(raw, 10, 64)
			if e != nil || n < 0 {
				return c, fmt.Errorf("invalid %s", key)
			}
			*target = n
			if key == "MEMBERSHIP_COMMISSION_BPS" {
				c.MembershipPolicyConfigured = true
			}
		}
	}
	if c.MembershipCommissionBPS > 10000 || c.RewardBPS > 10000 {
		return c, fmt.Errorf("basis points must be <=10000")
	}
	if c.Region == "" {
		c.Region = "ap-southeast-1"
	}
	if c.PublicOrigin == "" && c.DevMode {
		c.PublicOrigin = "http://localhost:5173"
	}
	if p := os.Getenv("PORT"); p != "" {
		v, e := strconv.Atoi(p)
		if e != nil || v < 1 || v > 65535 {
			return c, fmt.Errorf("invalid PORT")
		}
		c.Port = v
	}
	if v := os.Getenv("CREDITS_PER_SECOND"); v != "" {
		n, e := strconv.ParseInt(v, 10, 64)
		if e != nil || n < 1 || n > 10000 {
			return c, fmt.Errorf("invalid CREDITS_PER_SECOND")
		}
		c.CreditsPerSecond = n
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL required")
	}
	if c.RedisURL == "" && !c.DevMode {
		return c, fmt.Errorf("REDIS_URL required")
	}
	u, e := url.Parse(c.PublicOrigin)
	if e != nil || u.Host == "" || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "https" && !(c.DevMode && u.Scheme == "http") {
		return c, fmt.Errorf("PUBLIC_ORIGIN must be an HTTPS origin (HTTP allowed only in development)")
	}
	c.PublicOrigin = strings.TrimSuffix(c.PublicOrigin, "/")
	return c, nil
}
