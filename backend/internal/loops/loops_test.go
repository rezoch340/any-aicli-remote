package loops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseAndNormalizeInterval(testContext *testing.T) {
	tests := map[string]struct {
		seconds int
		label   string
	}{
		"5m":          {300, "5m"},
		"every 2 hrs": {7200, "2h"},
		"10":          {600, "10m"},
		"1s":          {60, "1m"},
		"99d":         {MaxInterval, "7d"},
		"90s":         {90, "90s"},
	}
	for raw, want := range tests {
		seconds, label, operationError := ParseInterval(raw)
		if operationError != nil || seconds != want.seconds || label != want.label {
			testContext.Errorf("ParseInterval(%q) = %d, %q, %v; want %d, %q", raw, seconds, label, operationError, want.seconds, want.label)
		}
	}
	if _, _, operationError := ParseInterval("soon"); !errors.Is(operationError, BadIntervalError) {
		testContext.Fatalf("bad interval error = %v", operationError)
	}
}

func TestManagerPersistsCreatesListsAndStops(testContext *testing.T) {
	store := filepath.Join(testContext.TempDir(), "nested", "loops.json")
	manager, operationError := New(store, nil)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	first, operationError := manager.Create("session-a", "first", 1, "", "/workspace")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	second, operationError := manager.Create("session-b", "second", 7200, "", "")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if first.IntervalSeconds != MinInterval || first.IntervalLabel != "1m" {
		testContext.Fatalf("first = %#v", first)
	}
	if second.IntervalLabel != "2h" {
		testContext.Fatalf("second = %#v", second)
	}
	info, operationError := os.Stat(store)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if info.Mode().Perm() != 0o600 {
		testContext.Fatalf("store mode = %o", info.Mode().Perm())
	}

	reloaded, operationError := New(store, nil)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if jobs := reloaded.List(""); len(jobs) != 2 {
		testContext.Fatalf("reloaded jobs = %#v", jobs)
	}
	if jobs := reloaded.List("session-a"); len(jobs) != 1 || jobs[0].ID != first.ID {
		testContext.Fatalf("filtered jobs = %#v", jobs)
	}
	removed, operationError := reloaded.Stop("", "session-a")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(removed) != 1 || removed[0].ID != first.ID || len(reloaded.List("")) != 1 {
		testContext.Fatalf("removed = %#v, remaining = %#v", removed, reloaded.List(""))
	}
	removed, operationError = reloaded.Stop(second.ID, "session-a")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if len(removed) != 1 || len(reloaded.List("")) != 0 {
		testContext.Fatalf("removed second = %#v", removed)
	}
}

func TestManagerFiresImmediatelyAndUpdatesJob(testContext *testing.T) {
	store := filepath.Join(testContext.TempDir(), "loops.json")
	fired := make(chan string, 1)
	manager, operationError := New(store, func(_ context.Context, job Job, note string) error {
		if job.SessionID != "session-1" {
			testContext.Errorf("callback job = %#v", job)
		}
		fired <- note
		return nil
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	job, operationError := manager.Create("session-1", "check status", 60, "1m", "/tmp/work")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := manager.Start(context.Background()); operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Cleanup(manager.Close)
	select {
	case note := <-fired:
		want := "[REMOTE LOOP · 1m · fire 1]\ncheck status"
		if note != want {
			testContext.Fatalf("note = %q, want %q", note, want)
		}
	case <-time.After(2 * time.Second):
		testContext.Fatal("loop did not fire")
	}
	waitFor(testContext, func() bool {
		jobs := manager.List("")
		return len(jobs) == 1 && jobs[0].ID == job.ID && jobs[0].Fires == 1 && jobs[0].LastFire > 0
	})
}

func TestManagerRecordsCallbackError(testContext *testing.T) {
	store := filepath.Join(testContext.TempDir(), "loops.json")
	called := make(chan struct{}, 1)
	manager, operationError := New(store, func(_ context.Context, _ Job, _ string) error {
		called <- struct{}{}
		return errors.New(strings.Repeat("failure", 40))
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := manager.Create("session", "prompt", 60, "", ""); operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := manager.Start(context.Background()); operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Cleanup(manager.Close)
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		testContext.Fatal("loop did not fire")
	}
	waitFor(testContext, func() bool {
		jobs := manager.List("")
		return len(jobs) == 1 && jobs[0].Fires == 0 && len([]rune(jobs[0].LastError)) == 200
	})
}

func TestManagerPrunesExpiredJobsAtStart(testContext *testing.T) {
	store := filepath.Join(testContext.TempDir(), "loops.json")
	stored := storeFile{Jobs: []Job{{
		ID: "loop-old", SessionID: "session", Prompt: "old", IntervalSeconds: 60,
		CreatedAt: 1, ExpiresAt: 2,
	}}}
	data, operationError := json.Marshal(stored)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := os.WriteFile(store, data, 0o600); operationError != nil {
		testContext.Fatal(operationError)
	}
	manager, operationError := New(store, func(context.Context, Job, string) error {
		testContext.Fatal("expired job fired")
		return nil
	})
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := manager.Start(context.Background()); operationError != nil {
		testContext.Fatal(operationError)
	}
	manager.Close()
	if jobs := manager.List(""); len(jobs) != 0 {
		testContext.Fatalf("expired jobs = %#v", jobs)
	}
}

func TestManagerLimitsAndValidation(testContext *testing.T) {
	manager, operationError := New(filepath.Join(testContext.TempDir(), "loops.json"), nil)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := manager.Create("", "prompt", 60, "", ""); !errors.Is(operationError, SessionRequiredError) {
		testContext.Fatalf("empty session error = %v", operationError)
	}
	if _, operationError := manager.Create("session", "", 60, "", ""); !errors.Is(operationError, PromptRequiredError) {
		testContext.Fatalf("empty prompt error = %v", operationError)
	}
	for itemIndex := 0; itemIndex < MaxJobs; itemIndex++ {
		if _, operationError := manager.Create("session", "prompt", 60, "", ""); operationError != nil {
			testContext.Fatal(operationError)
		}
	}
	if _, operationError := manager.Create("session", "one too many", 60, "", ""); !errors.Is(operationError, MaximumJobsError) {
		testContext.Fatalf("max jobs error = %v", operationError)
	}
}

func waitFor(testContext *testing.T, condition func() bool) {
	testContext.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	testContext.Fatal("condition was not met")
}
