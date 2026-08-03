package collaboration

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const sweepTimeout = 10 * time.Second

// Sweeper converges commands and sync requests that outlive their dispatch
// window. Charges are immutable, so expiry never changes the user balance.
type Sweeper struct {
	repository Repository
	enabled    bool
	interval   time.Duration
	now        func() time.Time

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

func NewSweeper(repository Repository, cfg config.CollaborationConfig) *Sweeper {
	interval := 30 * time.Second
	if cfg.CommandTTLSeconds > 0 {
		candidate := time.Duration(cfg.CommandTTLSeconds) * time.Second / 2
		if candidate < 5*time.Second {
			candidate = 5 * time.Second
		}
		if candidate > time.Minute {
			candidate = time.Minute
		}
		interval = candidate
	}
	return &Sweeper{
		repository: repository,
		enabled:    cfg.Enabled,
		interval:   interval,
		now:        time.Now,
		stopCh:     make(chan struct{}),
	}
}

func (s *Sweeper) Start() {
	if s == nil || !s.enabled || s.repository == nil {
		return
	}
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go s.run()
	})
}

func (s *Sweeper) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

func (s *Sweeper) run() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.runOnce()
	for {
		select {
		case <-ticker.C:
			s.runOnce()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Sweeper) runOnce() SweepResult {
	ctx, cancel := context.WithTimeout(context.Background(), sweepTimeout)
	defer cancel()
	result, err := s.repository.ExpirePending(ctx, s.now().UTC())
	if err != nil {
		slog.Error("expire pending collaboration records failed", "error", err)
		return SweepResult{}
	}
	if result.ExpiredCommands > 0 || result.ExpiredSyncs > 0 {
		slog.Info(
			"expired pending collaboration records",
			"commands", result.ExpiredCommands,
			"syncs", result.ExpiredSyncs,
		)
	}
	return result
}
