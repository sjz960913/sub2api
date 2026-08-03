package collaboration

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type sweepingRepositoryStub struct {
	repositoryStub
	now    time.Time
	result SweepResult
}

func (r *sweepingRepositoryStub) ExpirePending(_ context.Context, now time.Time) (SweepResult, error) {
	r.now = now
	return r.result, nil
}

func TestSweeperExpiresPendingRecordsWithoutBillingMutation(t *testing.T) {
	t.Parallel()

	repository := &sweepingRepositoryStub{result: SweepResult{ExpiredCommands: 2, ExpiredSyncs: 3}}
	sweeper := NewSweeper(repository, config.CollaborationConfig{Enabled: true, CommandTTLSeconds: 120})
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	sweeper.now = func() time.Time { return now }

	result := sweeper.runOnce()
	if result != repository.result {
		t.Fatalf("runOnce() = %#v, want %#v", result, repository.result)
	}
	if !repository.now.Equal(now.UTC()) {
		t.Fatalf("ExpirePending() time = %s, want %s", repository.now, now.UTC())
	}
}

func TestSweeperDisabledDoesNotStart(t *testing.T) {
	t.Parallel()

	repository := &sweepingRepositoryStub{}
	sweeper := NewSweeper(repository, config.CollaborationConfig{Enabled: false})
	sweeper.Start()
	sweeper.Stop()
	if !repository.now.IsZero() {
		t.Fatal("disabled sweeper called repository")
	}
}
