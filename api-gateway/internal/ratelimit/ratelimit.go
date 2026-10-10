package ratelimit

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/topinambur02/apigateway/internal/config"
)

const clientIdleTime = 2 * time.Minute

type client struct {
	tokens float64
	last   time.Time
}

type limiter struct {
	mu          sync.Mutex
	clients     map[string]*client
	rate        float64
	burst       float64
	lastCleanup time.Time
	now         func() time.Time
}

func New(cfg config.RateLimit, next http.Handler) (http.Handler, error) {
	if cfg.RequestsPerSecond <= 0 || cfg.Burst <= 0 {
		return nil, fmt.Errorf("rate_limit requests_per_second and burst must be positive")
	}

	return newHandler(cfg, next, time.Now), nil
}

func newHandler(cfg config.RateLimit, next http.Handler, now func() time.Time) http.Handler {
	l := &limiter{
		clients: make(map[string]*client),
		rate:    float64(cfg.RequestsPerSecond),
		burst:   float64(cfg.Burst),
		now:     now,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		if allowed, retryAfter := l.allow(ip); !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (l *limiter) allow(ip string) (bool, int) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastCleanup) >= time.Minute {
		for key, state := range l.clients {
			if now.Sub(state.last) >= clientIdleTime {
				delete(l.clients, key)
			}
		}
		l.lastCleanup = now
	}

	state, ok := l.clients[ip]
	if !ok {
		l.clients[ip] = &client{tokens: l.burst - 1, last: now}
		return true, 0
	}

	state.tokens = math.Min(l.burst, state.tokens+math.Max(0, now.Sub(state.last).Seconds())*l.rate)
	state.last = now
	if state.tokens >= 1 {
		state.tokens--
		return true, 0
	}

	return false, int(math.Ceil((1 - state.tokens) / l.rate))
}
