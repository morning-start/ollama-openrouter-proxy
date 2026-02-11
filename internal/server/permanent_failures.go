package server

import (
	"log/slog"
	"sync"
	"time"
)

type PermanentFailureTracker struct {
	mu              sync.RWMutex
	permanentFailed map[string]time.Time
	temporaryFailed map[string]time.Time
}

func NewPermanentFailureTracker() *PermanentFailureTracker {
	return &PermanentFailureTracker{
		permanentFailed: make(map[string]time.Time),
		temporaryFailed: make(map[string]time.Time),
	}
}

func (p *PermanentFailureTracker) MarkPermanentFailure(model string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.permanentFailed[model] = time.Now()
	slog.Warn("Model marked as permanently unavailable", "model", model)
}

func (p *PermanentFailureTracker) MarkTemporaryFailure(model string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.temporaryFailed[model] = time.Now()
}

func (p *PermanentFailureTracker) IsPermanentlyFailed(model string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, exists := p.permanentFailed[model]
	return exists
}

func (p *PermanentFailureTracker) ShouldSkip(model string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if _, exists := p.permanentFailed[model]; exists {
		return true
	}

	if failTime, exists := p.temporaryFailed[model]; exists {
		if time.Since(failTime) < 5*time.Minute {
			return true
		}
	}

	return false
}

func (p *PermanentFailureTracker) ClearTemporaryFailure(model string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.temporaryFailed, model)
}

func (p *PermanentFailureTracker) GetStats() (permanent int, temporary int) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	permanent = len(p.permanentFailed)

	now := time.Now()
	for _, failTime := range p.temporaryFailed {
		if now.Sub(failTime) < 5*time.Minute {
			temporary++
		}
	}

	return permanent, temporary
}
