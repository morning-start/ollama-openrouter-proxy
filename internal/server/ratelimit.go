package server

import (
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"
)

type RateLimiter struct {
	mu              sync.RWMutex
	lastRequestTime time.Time
	requestCount    int
	resetTime       time.Time
	backoffUntil    time.Time
	failureCount    int
	maxRetries      int
	baseDelay       time.Duration
	maxDelay        time.Duration
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		maxRetries: 3,
		baseDelay:  100 * time.Millisecond,
		maxDelay:   10 * time.Second,
	}
}

func (r *RateLimiter) Wait() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	if now.Before(r.backoffUntil) {
		waitTime := r.backoffUntil.Sub(now)
		slog.Debug("rate limiter waiting", "duration", waitTime)
		time.Sleep(waitTime)
		return
	}

	minInterval := 50 * time.Millisecond
	if elapsed := now.Sub(r.lastRequestTime); elapsed < minInterval {
		waitTime := minInterval - elapsed
		slog.Debug("rate limiting", "wait", waitTime)
		time.Sleep(waitTime)
	}

	r.lastRequestTime = time.Now()
}

func (r *RateLimiter) RecordSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.failureCount = 0
	r.backoffUntil = time.Time{}
}

func (r *RateLimiter) RecordFailure(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.failureCount++

	if isRateLimitError(err) {
		backoffDuration := r.calculateBackoff()
		r.backoffUntil = time.Now().Add(backoffDuration)

		slog.Warn("rate limit detected, backing off",
			"duration", backoffDuration,
			"failures", r.failureCount,
			"until", r.backoffUntil.Format(time.RFC3339))
	}
}

func (r *RateLimiter) calculateBackoff() time.Duration {
	multiplier := math.Pow(2, float64(r.failureCount-1))
	backoff := time.Duration(float64(r.baseDelay) * multiplier)

	if backoff > r.maxDelay {
		backoff = r.maxDelay
	}

	jitter := time.Duration(float64(backoff) * 0.25 * (0.5 - float64(time.Now().UnixNano()%100)/100))
	backoff += jitter

	return backoff
}

func (r *RateLimiter) ShouldRetry() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.failureCount < r.maxRetries
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "too many requests") ||
		strings.Contains(errStr, "quota exceeded")
}

func isPermanentError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
		return true
	}

	if strings.Contains(errStr, "no endpoints found") {
		return true
	}

	if strings.Contains(errStr, "model not available") || strings.Contains(errStr, "model does not exist") {
		return true
	}

	return false
}

type GlobalRateLimiter struct {
	mu         sync.RWMutex
	limiters   map[string]*RateLimiter
	globalWait time.Duration
	lastGlobal time.Time
}

func NewGlobalRateLimiter() *GlobalRateLimiter {
	return &GlobalRateLimiter{
		limiters:   make(map[string]*RateLimiter),
		globalWait: 50 * time.Millisecond,
	}
}

func (g *GlobalRateLimiter) GetLimiter(model string) *RateLimiter {
	g.mu.Lock()
	defer g.mu.Unlock()

	if limiter, exists := g.limiters[model]; exists {
		return limiter
	}

	limiter := NewRateLimiter()
	g.limiters[model] = limiter
	return limiter
}

func (g *GlobalRateLimiter) WaitGlobal() {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	if elapsed := now.Sub(g.lastGlobal); elapsed < g.globalWait {
		waitTime := g.globalWait - elapsed
		time.Sleep(waitTime)
	}
	g.lastGlobal = time.Now()
}
