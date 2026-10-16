package cloud

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCapacityRejectsAndReleases(t *testing.T) {
	s := NewServer(nil, Config{DevMode: true, MaxConcurrentRequests: 1})
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	handler := s.admission(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; w.WriteHeader(204) }))
	go func() {
		defer close(done)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/feed", nil))
	}()
	<-entered
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/feed", nil))
	if rr.Code != 503 || rr.Header().Get("Retry-After") == "" {
		t.Fatalf("expected controlled overload, got %d", rr.Code)
	}
	close(release)
	<-done
	rr = httptest.NewRecorder()
	s.admission(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(rr, httptest.NewRequest("GET", "/v1/feed", nil))
	if rr.Code != 204 {
		t.Fatalf("permit leaked: %d", rr.Code)
	}
}
func TestCapacityReleasesAfterPanic(t *testing.T) {
	s := NewServer(nil, Config{DevMode: true, MaxConcurrentRequests: 1})
	func() {
		defer func() { recover() }()
		s.admission(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("test") })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/feed", nil))
	}()
	rr := httptest.NewRecorder()
	s.admission(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(rr, httptest.NewRequest("GET", "/v1/feed", nil))
	if rr.Code != 204 {
		t.Fatal("permit leaked on panic")
	}
}
func TestFallbackLimiterBounded(t *testing.T) {
	s := NewServer(nil, Config{DevMode: true})
	for i := 0; i < localLimitCapacity; i++ {
		s.limits[string(rune(i))] = localLimit{count: 1, expires: time.Now().Add(time.Hour)}
	}
	rr := httptest.NewRecorder()
	if s.rate(rr, httptest.NewRequest("GET", "/", nil), "new-key", 3, time.Minute, false) {
		t.Fatal("new keys should fail safely when fallback limiter is full")
	}
	if len(s.limits) > localLimitCapacity {
		t.Fatal("limiter grew without bound")
	}
}
func TestCapacityConfigurationRejectsInvalidRedis(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example/db")
	t.Setenv("CLOUD_DEV_MODE", "true")
	t.Setenv("PUBLIC_ORIGIN", "http://localhost:5173")
	t.Setenv("REDIS_URL", "http://bad")
	if _, err := EnvConfig(); err == nil {
		t.Fatal("malformed Redis must fail startup")
	}
}

func TestAdmissionBurstIsBounded(t *testing.T) {
	s := NewServer(nil, Config{DevMode: true, MaxConcurrentRequests: 8})
	var active, peak, rejected atomic.Int64
	handler := s.admission(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		defer active.Add(-1)
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(204)
	}))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/example", nil))
			if rr.Code == 503 {
				rejected.Add(1)
			} else if rr.Code != 204 {
				t.Errorf("unexpected status %d", rr.Code)
			}
		}()
	}
	close(start)
	wg.Wait()
	if peak.Load() > 8 || peak.Load() < 1 || rejected.Load() == 0 || active.Load() != 0 {
		t.Fatalf("unbounded burst peak=%d rejected=%d active=%d", peak.Load(), rejected.Load(), active.Load())
	}
	if s.metrics.completed.Load()+s.metrics.rejected.Load() != 200 || s.metrics.active.Load() != 0 {
		t.Fatal("request counters or permits leaked")
	}
	t.Logf("200 concurrent attempts: peak work %d/8, controlled rejection %d, all permits released", peak.Load(), rejected.Load())
}
func TestRedisClientHasNoHiddenRetryAmplification(t *testing.T) {
	s := NewServer(nil, Config{DevMode: true, RedisURL: "redis://127.0.0.1:1/0"})
	defer s.Close()
	options := s.Redis.Options()
	if options.MaxRetries != 0 {
		t.Fatalf("unexpected Redis retry count %d", options.MaxRetries)
	}
	if options.PoolSize > 16 || options.PoolTimeout > time.Second || !options.ContextTimeoutEnabled {
		t.Fatal("Redis client is not bounded")
	}
}
func TestAdmissionBoundsOrdinaryRequestLifetime(t *testing.T) {
	s := NewServer(nil, Config{DevMode: true})
	s.admission(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 31*time.Second {
			t.Error("ordinary request lacks bounded lifetime")
		}
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/feed", nil))
}
