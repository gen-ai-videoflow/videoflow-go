package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

var ErrUnavailable = errors.New("service not configured or temporarily unavailable")

type userContext struct{}
type Server struct {
	Store        *Store
	Config       Config
	HTTP         *http.Client
	Redis        *redis.Client
	Media        *Media
	limMu        sync.Mutex
	imagePermits chan struct{}
	metrics      capacityMetrics
	permits      chan struct{}
	limits       map[string]localLimit
}
type localLimit struct {
	count   int
	expires time.Time
}

func NewServer(store *Store, cfg Config) *Server {
	capacity := cfg.MaxConcurrentRequests
	if capacity <= 0 {
		capacity = 128
	}
	s := &Server{imagePermits: make(chan struct{}, 2), permits: make(chan struct{}, capacity), Store: store, Config: cfg, HTTP: &http.Client{Timeout: 30 * time.Second}, limits: make(map[string]localLimit)}
	if cfg.RedisURL != "" {
		if options, err := redis.ParseURL(cfg.RedisURL); err == nil {
			options.DialTimeout = 2 * time.Second
			options.ReadTimeout = time.Second
			options.WriteTimeout = time.Second
			options.PoolTimeout = time.Second
			options.PoolSize = 16
			options.MaxRetries = -1
			options.ContextTimeoutEnabled = true
			s.Redis = redis.NewClient(options)
		}
	}
	return s
}
func (s *Server) Close() {
	if s.Redis != nil {
		s.Redis.Close()
	}
}
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer, s.security, s.admission)
	r.Get("/live", func(w http.ResponseWriter, r *http.Request) { s.json(w, 200, map[string]string{"status": "alive"}) })
	r.Get("/ready", s.ready)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if s.Store.DB.PingContext(ctx) != nil {
			s.json(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		s.json(w, 200, map[string]string{"status": "ok"})
	})
	r.Post("/v1/webhooks/stripe", s.paymentWebhook)
	r.Post("/v1/webhooks/cryptorails", s.cryptoWebhook)
	r.Get("/authorize", s.authorizeMedia)
	r.Route("/v1", func(r chi.Router) {
		s.premiumRoutes(r)
		s.cryptoRoutes(r)
		s.securityRoutes(r)
		s.magicLoginRoutes(r)
		s.oauthRoutes(r)
		s.adminConsoleRoutes(r)
		r.Get("/profile-images/{id}", s.profileImage)
		r.Post("/auth/register", s.register)
		r.Post("/auth/login", s.login)
		r.Post("/auth/logout", s.auth(s.logout))
		r.Post("/auth/verify-email", s.verifyEmail)
		r.Post("/auth/forgot-password", s.accountToken("reset_password"))
		r.Post("/auth/resend-verification", s.accountToken("verify_email"))
		r.Post("/auth/reset-password", s.resetPassword)
		r.Get("/feed", s.feed)
		r.Get("/videos", s.feed)
		r.Get("/search", s.feed)
		r.Get("/videos/{id}", s.video)
		r.Get("/videos/{id}/poster", s.videoPoster)
		r.Get("/videos/{id}/actions", s.videoActions)
		r.Get("/videos/{id}/related", s.relatedVideos)
		r.Post("/videos/{id}/playback-grant", s.issuePlayback)
		r.Get("/channels/{handle}", s.publicChannel)
		r.Get("/billing/packs", func(w http.ResponseWriter, r *http.Request) {
			s.json(w, 200, map[string]any{"packs": Packs, "checkout_enabled": s.cryptoEnabled() || s.stripeEnabled()})
		})
		r.Group(func(r chi.Router) {
			r.Use(s.requireUser, s.mutationLimits)
			s.profileRoutes(r)
			r.Get("/viewer/{kind:saved|history}", s.viewerLibrary)
			r.Put("/videos/{id}/{action:save|like}", s.changeVideoAction)
			r.Delete("/videos/{id}/{action:save|like}", s.changeVideoAction)
			r.Put("/videos/{id}/progress", s.saveVideoProgress)
			r.Get("/video-uploads/options", s.uploadOptions)
			r.Post("/video-uploads", s.beginVideoUpload)
			r.Get("/video-uploads/{id}", s.videoUploadStatus)
			r.Post("/video-uploads/{id}/parts", s.uploadPart)
			r.Post("/video-uploads/{id}/complete", s.completeVideoUpload)
			r.Delete("/video-uploads/{id}", s.abortVideoUpload)
			r.Get("/settings/preferences", s.getPreferences)
			r.Patch("/settings/preferences", s.patchPreferences)
			s.studioRoutes(r)
			s.canvasRoutes(r)
			s.pipelineRoutes(r)
			r.Delete("/viewer/history", s.clearHistory)
			r.Post("/account/deletion", s.requestAccountDeletion)
			r.Get("/me", func(w http.ResponseWriter, r *http.Request) { s.json(w, 200, currentUser(r)) })
			r.Get("/projects", func(w http.ResponseWriter, r *http.Request) {
				v, e := s.Store.Projects(r.Context(), currentUser(r))
				s.result(w, v, e)
			})
			r.Post("/projects", func(w http.ResponseWriter, r *http.Request) {
				var b struct {
					Name        string `json:"name"`
					Description string `json:"description"`
				}
				if !s.decode(w, r, &b) {
					return
				}
				v, e := s.Store.CreateProject(r.Context(), currentUser(r), b.Name, b.Description)
				s.result(w, v, e)
			})
			r.Get("/projects/{project}/prompt-templates", s.listPromptTemplates)
			r.Put("/projects/{project}/prompt-templates", s.savePromptTemplate)
			r.Delete("/projects/{project}/prompt-templates/{template}", s.deletePromptTemplate)
			r.Get("/projects/{project}/editor", s.getEditorDocument)
			r.Put("/projects/{project}/editor", s.saveEditorDocument)
			r.Get("/billing/history", s.billingHistory)
			r.Get("/credits", func(w http.ResponseWriter, r *http.Request) {
				v, e := s.Store.Credits(r.Context(), currentUser(r).WorkspaceID)
				s.result(w, v, e)
			})
			r.Post("/billing/checkout", s.billingCheckout)
			r.Get("/channel/videos", s.creatorVideos)
			r.Post("/watch-sessions", s.beginWatch)
			r.Post("/watch-events", s.watchEvent)
			r.Post("/viewer/preferences", s.viewerPreferences)
			r.Get("/jobs", func(w http.ResponseWriter, r *http.Request) {
				v, e := s.Store.Jobs(r.Context(), currentUser(r))
				s.result(w, v, e)
			})
			r.Get("/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
				v, e := s.Store.Job(r.Context(), currentUser(r), chi.URLParam(r, "id"))
				s.result(w, v, e)
			})
			r.Get("/jobs/{id}/events", s.streamJob)
			r.Post("/generation/quotes", s.quote)
			r.Post("/generation/jobs", s.submitJob)
			r.Post("/jobs/{id}/cancel", s.cancelJob)
			r.Get("/jobs/{id}/download", s.download)
			r.Get("/jobs/{id}/output", s.outputDetail)
			r.Get("/media-assets/{id}/{action:download|preview}", s.assetDownload)
			r.Get("/media-assets/{id}", func(w http.ResponseWriter, r *http.Request) {
				a, e := s.Store.ownedAsset(r.Context(), currentUser(r), chi.URLParam(r, "id"))
				s.result(w, a, e)
			})
			r.Post("/publications", s.createPublication)
			r.Post("/videos", s.createVideo)
			r.Post("/videos/{id}/publish", s.publishVideo)
			r.Patch("/videos/{id}", s.updateVideo)

			r.Post("/videos/{id}/report", s.report)
			r.Post("/channels/{id}/follow", s.follow)
			r.Delete("/channels/{id}/follow", func(w http.ResponseWriter, r *http.Request) {
				_, e := s.Store.DB.ExecContext(r.Context(), `DELETE FROM cloud_follows WHERE user_id=$1 AND channel_id=$2`, currentUser(r).ID, chi.URLParam(r, "id"))
				s.result(w, map[string]bool{"ok": true}, e)
			})
			r.Get("/channel", s.channel)
			r.Patch("/channel", s.updateChannel)
			r.Get("/creator/analytics", s.analytics)
			r.Get("/creator/analytics/detail", s.creatorAnalyticsDetail)
			r.Get("/creator/earnings", s.creatorEarnings)
			r.Get("/creator/rewards/program", s.rewardProgram)
			r.Post("/creator/rewards/enroll", s.rewardEnroll)
			r.Get("/creator/rewards/videos", s.rewardVideos)
			r.Get("/creator/rewards/videos/{id}", s.rewardVideoDetail)
			r.Get("/creator/rewards/payouts", s.rewardPayoutHistory)
			r.Get("/creator/rewards/summary", s.rewardSummary)
			r.Post("/memberships/checkout", s.membershipCheckout)
			r.Get("/memberships", s.memberships)
			r.Get("/creator/membership-plan", s.membershipPlan)
			r.Put("/creator/membership-plan", s.membershipPlan)
			r.Get("/creator/payout-account", s.payoutAccount)
			r.Post("/creator/payout-account", s.payoutAccount)
			r.Post("/assets/upload-grant", s.createUploadGrant)
		})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/v1/") || req.URL.Path == "/api/v1" {
			clone := req.Clone(req.Context())
			u := *req.URL
			u.Path = strings.TrimPrefix(u.Path, "/api")
			clone.URL = &u
			r.ServeHTTP(w, clone)
			return
		}
		r.ServeHTTP(w, req)
	})
}
func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", middleware.GetReqID(r.Context()))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Cache-Control", "no-store")
		if origin := r.Header.Get("Origin"); origin != "" && origin == s.Config.PublicOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == "OPTIONS" {
			if r.Header.Get("Origin") != s.Config.PublicOrigin {
				s.fail(w, ErrUnauthorized)
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Idempotency-Key")
			w.WriteHeader(204)
			return
		}
		appleCallback := r.Method == "POST" && r.URL.Path == "/v1/auth/oauth/apple/callback" && r.Header.Get("Origin") == "https://appleid.apple.com" && strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
		if r.Method != "GET" && r.Method != "HEAD" && !strings.HasPrefix(r.URL.Path, "/v1/webhooks/") && !appleCallback {
			if r.Header.Get("Origin") != s.Config.PublicOrigin {
				s.fail(w, ErrUnauthorized)
				return
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				s.fail(w, ErrValidation)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) json(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func (s *Server) result(w http.ResponseWriter, v any, err error) {
	if err != nil {
		s.fail(w, err)
		return
	}
	s.json(w, 200, v)
}
func (s *Server) fail(w http.ResponseWriter, err error) {
	status, code, message := 500, "internal_error", "Something went wrong. Please try again."
	switch {
	case errors.Is(err, ErrValidation):
		status, code, message = 400, "invalid_input", err.Error()
	case errors.Is(err, ErrUnauthorized), errors.Is(err, ErrInvalidCredentials):
		status, code, message = 401, "unauthorized", "Sign in or check your credentials."
	case errors.Is(err, ErrNotFound):
		status, code, message = 404, "not_found", "Resource not found."
	case errors.Is(err, ErrConflict):
		status, code, message = 409, "conflict", "This action has already been completed or conflicts with current state."
	case errors.Is(err, ErrInsufficientCredits):
		status, code, message = 402, "insufficient_credits", "You need more credits for this generation."
	case errors.Is(err, ErrUnavailable):
		status, code, message = 503, "unavailable", "This service is not available yet. Please try again later."
	}
	if status == 500 {
		slog.Error("cloud request failed", "error", err)
	}
	s.json(w, status, map[string]any{"code": code, "message": message, "request_id": w.Header().Get("X-Request-ID")})
}
func (s *Server) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		s.fail(w, ErrValidation)
		return false
	}
	if d.Decode(&struct{}{}) != io.EOF {
		s.fail(w, ErrValidation)
		return false
	}
	return true
}
func currentUser(r *http.Request) User { u, _ := r.Context().Value(userContext{}).(User); return u }
func (s *Server) optionalUser(r *http.Request) User {
	cookie, e := r.Cookie("videoflow_session")
	if e != nil {
		return User{}
	}
	u, _ := s.Store.Authenticate(r.Context(), cookie.Value)
	return u
}
func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := s.optionalUser(r)
		if u.ID == "" {
			s.fail(w, ErrUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContext{}, u)))
	})
}
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc { return s.requireUser(next).ServeHTTP }
func (s *Server) cookie(w http.ResponseWriter, token string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "videoflow_session", Value: token, Path: "/", MaxAge: age, HttpOnly: true, Secure: !s.Config.DevMode, SameSite: http.SameSiteLaxMode})
}
func (s *Server) rate(w http.ResponseWriter, r *http.Request, key string, max int, window time.Duration, strict bool) bool {
	hash := sha256.Sum256([]byte(key))
	key = "vf:limit:" + hex.EncodeToString(hash[:])
	if s.Redis != nil {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		n, err := s.Redis.Eval(ctx, `local n=redis.call('INCR',KEYS[1]);if n==1 then redis.call('PEXPIRE',KEYS[1],ARGV[1]) end;return n`, []string{key}, window.Milliseconds()).Int()
		if err == nil {
			if n <= max {
				return true
			}
			w.Header().Set("Retry-After", fmt.Sprint(int(window.Seconds())+1))
			s.json(w, 429, map[string]string{"message": "Too many requests. Please try again shortly."})
			return false
		}
	}
	if strict && !s.Config.DevMode {
		s.fail(w, ErrUnavailable)
		return false
	}
	s.limMu.Lock()
	defer s.limMu.Unlock()
	now := time.Now()
	if _, exists := s.limits[key]; !exists && len(s.limits) >= localLimitCapacity {
		for k, v := range s.limits {
			if now.After(v.expires) {
				delete(s.limits, k)
			}
		}
		if len(s.limits) >= localLimitCapacity {
			w.Header().Set("Retry-After", "60")
			s.json(w, 503, map[string]string{"message": "Request capacity reached. Please try again shortly."})
			return false
		}
	}
	v := s.limits[key]
	if now.After(v.expires) {
		v = localLimit{expires: now.Add(window)}
	}
	v.count++
	s.limits[key] = v
	if len(s.limits) > 10000 {
		for k, v := range s.limits {
			if now.After(v.expires) {
				delete(s.limits, k)
			}
		}
	}
	if v.count > max {
		w.Header().Set("Retry-After", fmt.Sprint(int(window.Seconds())+1))
		s.json(w, 429, map[string]string{"message": "Too many requests. Please try again shortly."})
		return false
	}
	return true
}
func (s *Server) peer(r *http.Request) string {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		return r.RemoteAddr
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	for _, raw := range strings.Split(s.Config.TrustedProxyCIDRs, ",") {
		prefix, e := netip.ParsePrefix(strings.TrimSpace(raw))
		if e != nil || !prefix.Contains(address) {
			continue
		}
		// Only the CloudFront-generated header is accepted, behind the restricted ALB.
		viewer, _, e := net.SplitHostPort(r.Header.Get("CloudFront-Viewer-Address"))
		if e == nil {
			if a, e := netip.ParseAddr(viewer); e == nil {
				return a.String()
			}
		}
	}
	return host
}
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if !s.rate(w, r, "register:"+s.peer(r), 5, time.Hour, true) {
		return
	}
	var b struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
		Handle   string `json:"handle"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	u, e := s.Store.Register(r.Context(), b.Email, b.Password, b.Name, b.Handle)
	if e != nil {
		s.fail(w, e)
		return
	}
	_, token, e := s.Store.Login(r.Context(), b.Email, b.Password)
	if e != nil {
		s.fail(w, e)
		return
	}
	if !s.Config.DevMode && s.Config.MailFrom != "" {
		if verifyToken, err := s.Store.IssueAccountToken(r.Context(), u.Email, "verify_email"); err == nil {
			if err = s.sendAccountEmail(r.Context(), u.Email, verifyToken, "verify_email"); err != nil {
				slog.Error("verification delivery failed", "error", err)
			}
		}
	}
	s.cookie(w, token, 30*24*3600)
	s.json(w, 201, u)
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.rate(w, r, "login:"+s.peer(r), 10, 15*time.Minute, true) {
		return
	}
	var b struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if !s.rate(w, r, "email-login:"+strings.ToLower(strings.TrimSpace(b.Email)), 10, 15*time.Minute, true) {
		return
	}
	u, token, challenge, e := s.Store.beginSecurityLogin(r.Context(), b.Email, b.Password)
	if e != nil {
		s.fail(w, e)
		return
	}
	if challenge != "" {
		methods := []string{}
		var totpEnabled, keys bool
		if err := s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_totp WHERE user_id=$1 AND enabled),EXISTS(SELECT 1 FROM cloud_passkeys WHERE user_id=$1)`, u.ID).Scan(&totpEnabled, &keys); err != nil {
			s.fail(w, err)
			return
		}
		if totpEnabled {
			methods = append(methods, "totp", "recovery")
		}
		if keys {
			methods = append(methods, "passkey")
		}
		s.json(w, 202, map[string]any{"mfa_required": true, "challenge": challenge, "methods": methods})
		return
	}
	s.cookie(w, token, 30*24*3600)
	s.json(w, 200, u)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("videoflow_session")
	if c != nil {
		if e := s.Store.Logout(r.Context(), c.Value); e != nil {
			s.fail(w, e)
			return
		}
	}
	s.cookie(w, "", -1)
	s.json(w, 200, map[string]bool{"ok": true})
}
func (s *Server) accountToken(purpose string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.rate(w, r, "email:"+s.peer(r), 3, time.Hour, true) {
			return
		}
		var b struct {
			Email string `json:"email"`
		}
		if !s.decode(w, r, &b) {
			return
		}
		token, e := s.Store.IssueAccountToken(r.Context(), b.Email, purpose)
		if e != nil && !errors.Is(e, ErrNotFound) {
			s.fail(w, e)
			return
		}
		result := map[string]any{"message": "If the account exists, a message will be sent."}
		if s.Config.DevMode {
			result["development_token"] = token
		} else if token != "" {
			if err := s.sendAccountEmail(r.Context(), b.Email, token, purpose); err != nil {
				slog.Error("account email delivery failed", "error", err)
			}
		}
		s.json(w, 200, result)
	}
}
func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Token string `json:"token"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	e := s.Store.VerifyEmail(r.Context(), b.Token)
	s.result(w, map[string]bool{"ok": true}, e)
}
func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	if !s.rate(w, r, "reset:"+s.peer(r), 5, time.Hour, true) {
		return
	}
	var b struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	e := s.Store.ResetPassword(r.Context(), b.Token, b.Password)
	s.result(w, map[string]bool{"ok": true}, e)
}
func (s *Server) quote(w http.ResponseWriter, r *http.Request) {
	cfg, configErr := s.effectiveConfig(r.Context())
	if configErr != nil {
		s.fail(w, configErr)
		return
	}
	u := currentUser(r)
	if !s.rate(w, r, "quote:"+u.ID, 20, time.Minute, true) {
		return
	}
	if cfg.VideoModel == "" || cfg.OpenRouterKey == "" || cfg.QueueURL == "" {
		s.fail(w, ErrUnavailable)
		return
	}
	var b QuoteInput
	if !s.decode(w, r, &b) {
		return
	}
	if cfg.ProviderMaxCostMicros <= 0 || cfg.ProviderDailyBudgetMicros <= 0 {
		s.fail(w, ErrUnavailable)
		return
	}
	for _, model := range strings.Split(cfg.VideoModel, ",") {
		if e := s.Store.ConfigureVideoPolicy(r.Context(), strings.TrimSpace(model), cfg.CreditsPerSecond, cfg.ProviderMaxCostMicros); e != nil {
			s.fail(w, ErrUnavailable)
			return
		}
	}
	if e := s.Store.RefreshVideoCatalog(r.Context()); e != nil {
		s.fail(w, ErrUnavailable)
		return
	}
	policy, e := s.Store.ActiveVideoPolicy(r.Context(), b.Duration, b.AspectRatio, strings.Split(cfg.VideoModel, ",")...)
	if e != nil {
		s.fail(w, fmt.Errorf("%w: this duration or aspect ratio is currently unavailable", ErrValidation))
		return
	}
	v, e := s.Store.CreatePolicyQuote(r.Context(), u, b, policy)
	s.result(w, v, e)
}
func (s *Server) submitJob(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if !u.Verified {
		s.fail(w, fmt.Errorf("%w: verify your email before generating", ErrValidation))
		return
	}
	if !s.rate(w, r, "generate:"+u.ID, 5, time.Minute, true) {
		return
	}
	var b struct {
		QuoteID string `json:"quote_id"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	v, e := s.Store.SubmitJob(r.Context(), u, b.QuoteID, r.Header.Get("Idempotency-Key"))
	s.result(w, v, e)
}
func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	j, e := s.Store.Job(r.Context(), currentUser(r), chi.URLParam(r, "id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	if !j.CanCancel {
		s.fail(w, ErrConflict)
		return
	}
	s.result(w, map[string]bool{"ok": true}, s.Store.FinishJob(r.Context(), j.ID, "cancelled", "", "Cancelled before generation"))
}
func (s *Server) feed(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("rank") != "" || r.URL.Query().Get("format") != "" || r.URL.Query().Get("cursor") != "" {
		s.viewingFeed(w, r)
		return
	}
	v, e := s.Store.DiscoveryFeed(r.Context(), s.optionalUser(r), r.URL.Query().Get("q"), r.URL.Query().Get("category"), r.URL.Query().Get("filter"))
	s.result(w, v, e)
}
func (s *Server) video(w http.ResponseWriter, r *http.Request) {
	v, e := s.Store.Video(r.Context(), chi.URLParam(r, "id"), s.optionalUser(r).ID)
	s.result(w, v, e)
}
func (s *Server) createVideo(w http.ResponseWriter, r *http.Request) {
	var b Video
	if !s.decode(w, r, &b) {
		return
	}
	v, e := s.Store.CreateVideo(r.Context(), currentUser(r), b)
	s.result(w, v, e)
}
func (s *Server) publishVideo(w http.ResponseWriter, r *http.Request) {
	if s.Config.MediaConvertRole == "" || s.Config.PlaybackBucket == "" {
		s.fail(w, ErrUnavailable)
		return
	}
	e := s.Store.PublishVideo(r.Context(), currentUser(r), chi.URLParam(r, "id"))
	s.result(w, map[string]bool{"ok": true}, e)
}
func (s *Server) updateVideo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Visibility  string `json:"visibility"`
		Audience    string `json:"audience"`
		Format      string `json:"format"`
		AI          *bool  `json:"ai_generated"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	u := currentUser(r)
	v, e := s.Store.Video(r.Context(), chi.URLParam(r, "id"), u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	if v.ChannelID != u.ChannelID {
		s.fail(w, ErrNotFound)
		return
	}
	if in.Format == "" {
		in.Format = v.Format
	}
	if in.Title == "" || len(in.Title) > 180 || len(in.Description) > 5000 || (in.Visibility != "public" && in.Visibility != "unlisted" && in.Visibility != "private") || (in.Audience != "free" && in.Audience != "members") || (in.Format != "standard" && in.Format != "short") {
		s.fail(w, ErrValidation)
		return
	}
	if in.Format == "short" {
		a, e := s.Store.ownedAsset(r.Context(), u, v.SourceAssetID)
		if e != nil || !a.Measured || a.Height <= a.Width || a.DurationMS < 3000 || a.DurationMS > 180000 {
			s.fail(w, ErrValidation)
			return
		}
	}
	ai := v.AIGenerated
	if in.AI != nil {
		ai = *in.AI
	}
	if v.Origin == "generated" {
		ai = true
	}
	_, e = s.Store.DB.ExecContext(r.Context(), `UPDATE cloud_videos SET title=$3,description=$4,visibility=$5,audience=$6,format=$7,ai_generated=$8 WHERE id=$1 AND channel_id=$2`, v.ID, u.ChannelID, in.Title, in.Description, in.Visibility, in.Audience, in.Format, ai)
	s.result(w, map[string]bool{"ok": true}, e)
}
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	j, e := s.Store.Job(r.Context(), currentUser(r), chi.URLParam(r, "id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	if j.Status != "succeeded" || s.Media == nil {
		s.fail(w, ErrUnavailable)
		return
	}
	u, e := s.Media.DownloadGrant(r.Context(), j.OutputKey)
	s.result(w, map[string]string{"url": u}, e)
}
func (s *Server) channelData(ctx context.Context, handle string) (map[string]any, error) {
	return s.richChannelData(ctx, handle)
}
func (s *Server) publicChannel(w http.ResponseWriter, r *http.Request) {
	c, e := s.channelData(r.Context(), chi.URLParam(r, "handle"))
	if e != nil {
		s.fail(w, e)
		return
	}
	var following bool
	if err := s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_follows WHERE user_id=$1 AND channel_id=$2)`, s.optionalUser(r).ID, c["id"]).Scan(&following); err != nil {
		s.fail(w, err)
		return
	}
	c["is_following"] = following
	v, e := s.Store.Videos(r.Context(), c["id"].(string), false)
	var price int64
	var currency string
	var enabled bool
	if err := s.Store.DB.QueryRowContext(r.Context(), `SELECT price_cents,currency,enabled FROM cloud_membership_plans WHERE channel_id=$1`, c["id"]).Scan(&price, &currency, &enabled); err == nil {
		c["membership_plan"] = map[string]any{"price_cents": price, "currency": currency, "enabled": enabled, "sales_enabled": s.membershipSalesEnabled()}
	}
	c["videos"] = v
	s.result(w, c, e)
}
func (s *Server) channel(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	c, e := s.channelData(r.Context(), u.Handle)
	if e != nil {
		s.fail(w, e)
		return
	}
	v, e := s.Store.Videos(r.Context(), u.ChannelID, true)
	c["videos"] = v
	s.result(w, c, e)
}
func (s *Server) updateChannel(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if strings.TrimSpace(b.Name) == "" || len(b.Name) > 100 || len(b.Description) > 2000 {
		s.fail(w, ErrValidation)
		return
	}
	_, e := s.Store.DB.ExecContext(r.Context(), `UPDATE cloud_channels SET name=$2,description=$3 WHERE id=$1`, currentUser(r).ChannelID, b.Name, b.Description)
	s.result(w, map[string]bool{"ok": true}, e)
}
func (s *Server) analytics(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var views, seconds, videos, followers int64
	e := s.Store.DB.QueryRowContext(r.Context(), `SELECT coalesce(sum(views),0),coalesce(sum(watch_seconds),0),count(*) FROM cloud_videos WHERE channel_id=$1`, u.ChannelID).Scan(&views, &seconds, &videos)
	if e == nil {
		e = s.Store.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM cloud_follows WHERE channel_id=$1`, u.ChannelID).Scan(&followers)
	}
	s.result(w, map[string]any{"views": views, "watch_minutes": seconds / 60, "videos": videos, "followers": followers}, e)
}
func (s *Server) follow(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id := chi.URLParam(r, "id")
	if id == u.ChannelID {
		s.fail(w, ErrValidation)
		return
	}
	_, e := s.Store.DB.ExecContext(r.Context(), `INSERT INTO cloud_follows(user_id,channel_id)SELECT $1,id FROM cloud_channels WHERE id=$2 ON CONFLICT DO NOTHING`, u.ID, id)
	s.result(w, map[string]bool{"ok": true}, e)
}
func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if len(b.Reason) < 5 || len(b.Reason) > 2000 {
		s.fail(w, ErrValidation)
		return
	}
	_, e := s.Store.DB.ExecContext(r.Context(), `INSERT INTO cloud_reports(id,user_id,video_id,reason)VALUES($1,$2,$3,$4)`, uuid.NewString(), currentUser(r).ID, chi.URLParam(r, "id"), b.Reason)
	s.result(w, map[string]bool{"ok": true}, e)
}

func (s *Server) mutationLimits(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" && !s.rate(w, r, "mutations:"+currentUser(r).ID, 60, time.Minute, true) {
			return
		}
		if r.Method == "POST" {
			path := r.URL.Path
			key := ""
			if strings.HasSuffix(path, "/generation/quotes") || strings.HasSuffix(path, "/generation/jobs") || strings.HasSuffix(path, "/pipelines/quotes") {
				key = "generation_enabled"
			}
			if strings.HasSuffix(path, "/publications") || strings.HasSuffix(path, "/videos") || strings.HasSuffix(path, "/publish") {
				key = "publishing_enabled"
			}
			if key != "" && !s.adminFeatureEnabled(r.Context(), key) {
				s.fail(w, ErrUnavailable)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
