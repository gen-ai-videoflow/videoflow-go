package cloud

import (
	"fmt"
	"github.com/redis/go-redis/v9"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	SessionCookieDomain                                                                                                                                                                                   string
	BrowserOrigins                                                                                                                                                                                        []string
	GoogleClientID, GoogleClientSecret, AppleClientID, AppleClientSecret                                                                                                                                  string
	MaxConcurrentRequests                                                                                                                                                                                 int
	VideoUploadsEnabled                                                                                                                                                                                   bool
	MediaProbePath                                                                                                                                                                                        string
	AuthEncryptionKey                                                                                                                                                                                     string
	CryptoRailsEnabled, CryptoRailsUSDPegAccepted                                                                                                                                                         bool
	CryptoRailsAPIURL, CryptoRailsAPIKey, CryptoRailsWebhookSecret, CryptoRailsNetwork, CryptoRailsVMType, CryptoRailsToken, CryptoRailsTokenAddress, CryptoRailsTreasuryAddress, CryptoRailsDepositFixed string
	CryptoRailsTokenDecimals                                                                                                                                                                              int
	CryptoRailsDepositFeeBPS                                                                                                                                                                              int64
	PremiumPriceCents                                                                                                                                                                                     int64
	StoryModel, FFmpegPath                                                                                                                                                                                string
	StoryCredits, StoryMaxCostMicros, AssemblyCredits                                                                                                                                                     int64
	S3PublicEndpoint                                                                                                                                                                                      string
	AnalyticsBucket                                                                                                                                                                                       string
	CashPayoutsEnabled                                                                                                                                                                                    bool
	UploadsBucket, TrustedProxyCIDRs                                                                                                                                                                      string
	EncodingQueueURL                                                                                                                                                                                      string
	MembershipsEnabled, MembershipPolicyConfigured, ConnectEnabled, RewardsEnabled                                                                                                                        bool
	MembershipCommissionBPS, RewardBPS, RewardMonthlyCapCents                                                                                                                                             int64
	SupportedPayoutCountries                                                                                                                                                                              string
	ProviderDailyBudgetMicros, ProviderMaxCostMicros                                                                                                                                                      int64

	DatabaseURL, RedisURL, Region, OriginalsBucket, PlaybackBucket, QueueURL, MediaConvertRole, MediaConvertEndpoint, CloudFrontDomain, CloudFrontKeyPairID, CloudFrontPrivateKey, OpenRouterKey, VideoModel, PublicOrigin string
	StripeSecret, StripeWebhookSecret, MailFrom, MailEndpoint                                                                                                                                                              string
	DevMode                                                                                                                                                                                                                bool
	Port                                                                                                                                                                                                                   int
	CreditsPerSecond                                                                                                                                                                                                       int64
	BillingEnabled                                                                                                                                                                                                         bool
}

func EnvConfig() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), RedisURL: os.Getenv("REDIS_URL"), Region: os.Getenv("AWS_REGION"), OriginalsBucket: os.Getenv("S3_BUCKET_ORIGINALS"), PlaybackBucket: os.Getenv("S3_BUCKET_PLAYBACK"), QueueURL: os.Getenv("SQS_QUEUE_URL"), MediaConvertRole: os.Getenv("MEDIACONVERT_ROLE_ARN"), MediaConvertEndpoint: os.Getenv("MEDIACONVERT_ENDPOINT"), CloudFrontDomain: os.Getenv("CLOUDFRONT_DOMAIN"), CloudFrontKeyPairID: os.Getenv("CLOUDFRONT_KEY_PAIR_ID"), CloudFrontPrivateKey: os.Getenv("CLOUDFRONT_PRIVATE_KEY"), OpenRouterKey: os.Getenv("OPENROUTER_API_KEY"), VideoModel: os.Getenv("OPENROUTER_VIDEO_MODEL"), PublicOrigin: os.Getenv("PUBLIC_ORIGIN"), StripeSecret: os.Getenv("STRIPE_SECRET_KEY"), StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"), MailFrom: os.Getenv("MAIL_FROM"), MailEndpoint: os.Getenv("MAIL_ENDPOINT"), DevMode: os.Getenv("CLOUD_DEV_MODE") == "true", BillingEnabled: os.Getenv("BILLING_ENABLED") == "true", Port: 8000, CreditsPerSecond: 10}
	c.GoogleClientID = os.Getenv("GOOGLE_CLIENT_ID")
	c.GoogleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	c.AppleClientID = os.Getenv("APPLE_CLIENT_ID")
	c.AppleClientSecret = os.Getenv("APPLE_CLIENT_SECRET")
	c.MaxConcurrentRequests = 128
	if raw := os.Getenv("MAX_CONCURRENT_REQUESTS"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > 4096 {
			return c, fmt.Errorf("MAX_CONCURRENT_REQUESTS must be 1..4096")
		}
		c.MaxConcurrentRequests = v
	}
	if c.RedisURL != "" {
		if _, err := redis.ParseURL(c.RedisURL); err != nil {
			return c, fmt.Errorf("REDIS_URL must be a valid Redis URL")
		}
	}
	c.VideoUploadsEnabled = os.Getenv("VIDEO_UPLOADS_ENABLED") == "true"
	c.MediaProbePath = os.Getenv("FFPROBE_PATH")
	if c.MediaProbePath == "" {
		c.MediaProbePath = "ffprobe"
	}
	c.AuthEncryptionKey = os.Getenv("AUTH_ENCRYPTION_KEY")
	if c.AuthEncryptionKey != "" {
		if _, err := authCipher(c.AuthEncryptionKey); err != nil {
			return c, fmt.Errorf("AUTH_ENCRYPTION_KEY must be base64 encoded 32 bytes")
		}
	}
	c.CryptoRailsEnabled = os.Getenv("CRYPTORAILS_ENABLED") == "true"
	c.CryptoRailsUSDPegAccepted = os.Getenv("CRYPTORAILS_USD_PEG_ACCEPTED") == "true"
	c.CryptoRailsAPIURL = os.Getenv("CRYPTORAILS_API_URL")
	if c.CryptoRailsAPIURL == "" {
		c.CryptoRailsAPIURL = "https://api.cryptorails.tech/v1"
	}
	c.CryptoRailsAPIKey = os.Getenv("CRYPTORAILS_API_KEY")
	c.CryptoRailsWebhookSecret = os.Getenv("CRYPTORAILS_WEBHOOK_SECRET")
	c.CryptoRailsNetwork = os.Getenv("CRYPTORAILS_NETWORK")
	c.CryptoRailsVMType = os.Getenv("CRYPTORAILS_VM_TYPE")
	if raw := os.Getenv("CRYPTORAILS_TOKEN_DECIMALS"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value != 0 && (value < 5 || value > 18) {
			return c, fmt.Errorf("invalid CRYPTORAILS_TOKEN_DECIMALS")
		}
		c.CryptoRailsTokenDecimals = value
	}
	c.CryptoRailsToken = os.Getenv("CRYPTORAILS_TOKEN")
	c.CryptoRailsTokenAddress = os.Getenv("CRYPTORAILS_TOKEN_ADDRESS")
	c.CryptoRailsTreasuryAddress = os.Getenv("CRYPTORAILS_TREASURY_ADDRESS")
	c.CryptoRailsDepositFixed = os.Getenv("CRYPTORAILS_DEPOSIT_FIXED")
	if raw := os.Getenv("CRYPTORAILS_DEPOSIT_FEE_BPS"); raw != "" {
		n, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || n < 0 || n >= 10000 {
			return c, fmt.Errorf("invalid CRYPTORAILS_DEPOSIT_FEE_BPS")
		}
		c.CryptoRailsDepositFeeBPS = n
	}
	if c.CryptoRailsEnabled {
		if err := c.validateCryptoRails(); err != nil {
			return c, err
		}
	}
	c.StoryModel = os.Getenv("OPENROUTER_STORY_MODEL")
	c.FFmpegPath = os.Getenv("FFMPEG_PATH")
	if c.FFmpegPath == "" {
		c.FFmpegPath = "ffmpeg"
	}
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
	for key, target := range map[string]*int64{"PREMIUM_PRICE_CENTS": &c.PremiumPriceCents, "STORY_CREDITS": &c.StoryCredits, "STORY_MAX_COST_MICROS": &c.StoryMaxCostMicros, "ASSEMBLY_CREDITS": &c.AssemblyCredits, "MEMBERSHIP_COMMISSION_BPS": &c.MembershipCommissionBPS, "REWARD_BPS": &c.RewardBPS, "REWARD_MONTHLY_CAP_CENTS": &c.RewardMonthlyCapCents, "PROVIDER_DAILY_BUDGET_MICROS": &c.ProviderDailyBudgetMicros, "PROVIDER_MAX_COST_MICROS": &c.ProviderMaxCostMicros} {
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
	if c.PremiumPriceCents != 0 && (c.PremiumPriceCents < 100 || c.PremiumPriceCents > 100000) {
		return c, fmt.Errorf("PREMIUM_PRICE_CENTS must be 0 (disabled) or 100..100000")
	}
	if c.MembershipCommissionBPS > 10000 || c.RewardBPS > 10000 {
		return c, fmt.Errorf("basis points must be <=10000")
	}
	if c.Region == "" {
		c.Region = "ap-southeast-1"
	}
	c.SessionCookieDomain = strings.TrimSpace(os.Getenv("SESSION_COOKIE_DOMAIN"))
	for _, origin := range strings.Split(os.Getenv("BROWSER_ORIGINS"), ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			c.BrowserOrigins = append(c.BrowserOrigins, origin)
		}
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
	if c.VideoUploadsEnabled && (c.UploadsBucket == "" || c.OriginalsBucket == "" || c.QueueURL == "") {
		return c, fmt.Errorf("video uploads require private upload/original storage and a validation queue")
	}
	c.PublicOrigin = strings.TrimSuffix(c.PublicOrigin, "/")
	if e := validateSessionConfig(c); e != nil {
		return c, e
	}
	return c, nil
}
