package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

func (s *Server) cryptoCooldownKey() string {
	hash := sha256.Sum256([]byte(s.Config.CryptoRailsAPIURL + "\x00" + s.Config.CryptoRailsAPIKey))
	return "videoflow:cryptorails:cooldown:" + hex.EncodeToString(hash[:16])
}
func (s *Server) cryptoCoolingDown(ctx context.Context) bool {
	key := s.cryptoCooldownKey()
	s.limMu.Lock()
	local := s.limits[key].expires
	s.limMu.Unlock()
	if time.Now().Before(local) {
		return true
	}
	if s.Redis != nil {
		_, err := s.Redis.Get(ctx, key).Result()
		// A missing cache key permits requests; unavailable production rate storage
		// fails closed without affecting already-verified settlement processing.
		return err != redis.Nil
	}
	s.limMu.Lock()
	defer s.limMu.Unlock()
	return time.Now().Before(s.limits[key].expires)
}
func (s *Server) cryptoRecordCooldown(ctx context.Context, header string) {
	delay := 60 * time.Second
	if seconds, err := strconv.ParseInt(header, 10, 64); err == nil && seconds >= 0 && seconds <= int64((1<<63-1)/int64(time.Second)) {
		delay = time.Duration(seconds) * time.Second
	} else if when, err := http.ParseTime(header); err == nil {
		delay = time.Until(when)
	}
	if delay < time.Second {
		delay = time.Second
	}
	key := s.cryptoCooldownKey()
	s.limMu.Lock()
	until := time.Now().Add(delay)
	if until.After(s.limits[key].expires) {
		s.limits[key] = localLimit{expires: until}
	}
	s.limMu.Unlock()
	if s.Redis != nil {
		_ = s.Redis.Eval(ctx, `if redis.call('pttl',KEYS[1]) < tonumber(ARGV[1]) then redis.call('psetex',KEYS[1],ARGV[1],'1') end return 1`, []string{key}, delay.Milliseconds()).Err()
	}
}
