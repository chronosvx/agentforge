package automation

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSchedulerTriggersOnShortInterval(t *testing.T) {
	var mu sync.Mutex
	triggered := 0

	s := NewScheduler(func(job Job) {
		mu.Lock()
		triggered++
		mu.Unlock()
	})
	if err := s.AddJob(Job{
		ID:       "j1",
		Name:     "test",
		Command:  "echo hello",
		Interval: 30 * time.Millisecond,
		Enabled:  true,
	}); err != nil {
		t.Fatalf("add job: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("start scheduler: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	s.Stop()

	mu.Lock()
	defer mu.Unlock()
	if triggered < 1 {
		t.Fatalf("expected at least 1 trigger, got %d", triggered)
	}
}

func TestSchedulerSkipsDisabledJob(t *testing.T) {
	var mu sync.Mutex
	triggered := 0

	s := NewScheduler(func(job Job) {
		mu.Lock()
		triggered++
		mu.Unlock()
	})
	if err := s.AddJob(Job{
		ID:       "j2",
		Name:     "disabled",
		Command:  "echo nope",
		Interval: 20 * time.Millisecond,
		Enabled:  false,
	}); err != nil {
		t.Fatalf("add job: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("start scheduler: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	s.Stop()

	mu.Lock()
	defer mu.Unlock()
	if triggered != 0 {
		t.Fatalf("disabled job triggered %d times, want 0", triggered)
	}
}

func TestAddJobRequiresSchedule(t *testing.T) {
	s := NewScheduler(func(job Job) {})
	if err := s.AddJob(Job{ID: "j3", Command: "echo hi"}); err == nil {
		t.Fatal("expected error when neither cron nor interval is set")
	}
}

func TestNextCronEveryMinute(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 30, 0, time.UTC)
	next, err := nextCron("* * * * *", start)
	if err != nil {
		t.Fatalf("next cron: %v", err)
	}
	want := time.Date(2025, 1, 1, 0, 1, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}
}
