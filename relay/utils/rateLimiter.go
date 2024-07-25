package utils

import (
	"sync"

	"golang.org/x/time/rate"
)

type KindLimiter struct {
	Limiter *rate.Limiter
	Limit   rate.Limit
	Burst   int
}

type RateLimiter struct {
	eventLimiter *rate.Limiter
	wsLimiter    *rate.Limiter
	kindLimiters map[int]*KindLimiter
	mu           sync.RWMutex
}

func NewRateLimiter(eventLimit rate.Limit, eventBurst int, wsLimit rate.Limit, wsBurst int) *RateLimiter {
	return &RateLimiter{
		eventLimiter: rate.NewLimiter(eventLimit, eventBurst),
		wsLimiter:    rate.NewLimiter(wsLimit, wsBurst),
		kindLimiters: make(map[int]*KindLimiter),
	}
}

func (rl *RateLimiter) AddKindLimit(kind int, limit rate.Limit, burst int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.kindLimiters[kind] = &KindLimiter{
		Limiter: rate.NewLimiter(limit, burst),
		Limit:   limit,
		Burst:   burst,
	}
}

func (rl *RateLimiter) AllowEvent(kind int) bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if !rl.eventLimiter.Allow() {
		return false
	}

	if kindLimiter, exists := rl.kindLimiters[kind]; exists {
		if !kindLimiter.Limiter.Allow() {
			return false
		}
	}

	return true
}

func (rl *RateLimiter) AllowWs() bool {
	return rl.wsLimiter.Allow()
}
