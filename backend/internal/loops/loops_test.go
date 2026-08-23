package loops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testPolicy() Policy {
	return Policy{MinInterval: 60 * time.Second, MaxInterval: 168 * time.Hour, DefaultInterval: 5 * time.Minute, Retention: 168 * time.Hour, MaxJobs: 50, FireTimeout: 10 * time.Minute, LastErrorRunes: 200}
}

func TestParseAndNormalizeInterval(testContext *testing.T) {
	tests := map[string]struct {
		seconds int
		label   string
	}{
		"5m":          {300, "5m"},
		"every 2 hrs": {7200, "2h"},
		"10":          {600, "10m"},
		"1s":          {60, "1m"},
		"99d":         {(7 * 24 * 60 * 60), "7d"},
		"90s":         {90, "90s"},
	}
	for raw, want := range tests {
		seconds, label, operationError := testPolicy().ParseInterval(raw)
		if operationError != nil || seconds != want.seconds || label != want.label {
			testContext.Errorf("ParseInterval(%q) = %d, %q, %v; want %d, %q", raw, seconds, label, operationError, want.seconds, want.label)
		}
	}
	if _, _, operationError := testPolicy().ParseInterval("soon"); !errors.Is(operationError, BadIntervalError) {
		testContext.Fatalf("bad interval error = %v", operationError)
	}
}

func TestManagerPersistsCreatesListsAndStops(testContext *testing.T) {
	store := filepath.Join(testContext.TempDir(), "nested", "loops.json")
	manager, operationError := New(store, nil, testPolicy())
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
	if first.IntervalSeconds != 60 || first.IntervalLabel != "1m" {
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

	reloaded, operationError := New(store, nil, testPolicy())
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
	}, testPolicy())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := manager.Start(context.Background()); operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Cleanup(manager.Close)
	job, operationError := manager.Create("session-1", "check status", 60, "1m", "/tmp/work")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
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
	}, testPolicy())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := manager.Start(context.Background()); operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Cleanup(manager.Close)
	if _, operationError := manager.Create("session", "prompt", 60, "", ""); operationError != nil {
		testContext.Fatal(operationError)
	}
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

func TestManagerDoesNotFirePersistedJobImmediatelyOnStart(testContext *testing.T) {
	store := filepath.Join(testContext.TempDir(), "loops.json")
	persistedManager, operationError := New(store, nil, testPolicy())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := persistedManager.Create("session", "prompt", 60, "", ""); operationError != nil {
		testContext.Fatal(operationError)
	}

	fired := make(chan struct{}, 1)
	reloadedManager, operationError := New(store, func(context.Context, Job, string) error {
		fired <- struct{}{}
		return nil
	}, testPolicy())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := reloadedManager.Start(context.Background()); operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Cleanup(reloadedManager.Close)

	select {
	case <-fired:
		testContext.Fatal("persisted loop fired during daemon startup")
	case <-time.After(250 * time.Millisecond):
	}
	jobs := reloadedManager.List("")
	if len(jobs) != 1 || jobs[0].Fires != 0 || jobs[0].LastFire != 0 {
		testContext.Fatalf("persisted loop changed before its interval: %#v", jobs)
	}
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
	}, testPolicy())
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
	manager, operationError := New(filepath.Join(testContext.TempDir(), "loops.json"), nil, testPolicy())
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError := manager.Create("", "prompt", 60, "", ""); !errors.Is(operationError, SessionRequiredError) {
		testContext.Fatalf("empty session error = %v", operationError)
	}
	if _, operationError := manager.Create("session", "", 60, "", ""); !errors.Is(operationError, PromptRequiredError) {
		testContext.Fatalf("empty prompt error = %v", operationError)
	}
	for itemIndex := 0; itemIndex < testPolicy().MaxJobs; itemIndex++ {
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

func TestCustomPolicyControlsLifecycle(testContext *testing.T) {
	policy := Policy{MinInterval: 2 * time.Second, MaxInterval: 9 * time.Second, DefaultInterval: 4 * time.Second, Retention: 30 * time.Second, MaxJobs: 2, FireTimeout: 25 * time.Millisecond, LastErrorRunes: 7}
	if _, operationError := New(filepath.Join(testContext.TempDir(), "invalid.json"), nil, Policy{}); operationError == nil {
		testContext.Fatal("zero policy accepted")
	}
	seconds, label, operationError := policy.ParseInterval("1s")
	if operationError != nil || seconds != 2 || label != "2s" {
		testContext.Fatalf("clamp = %d %s %v", seconds, label, operationError)
	}
	blockStarted := make(chan struct{})
	deadlineRemaining := make(chan time.Duration, 1)
	callbackDone := make(chan struct{})
	var startOnce sync.Once
	var doneOnce sync.Once
	manager, operationError := New(filepath.Join(testContext.TempDir(), "loops.json"), func(operationContext context.Context, _ Job, _ string) error {
		startOnce.Do(func() { close(blockStarted) })
		deadline, deadlineSet := operationContext.Deadline()
		if deadlineSet {
			deadlineRemaining <- time.Until(deadline)
		}
		<-operationContext.Done()
		doneOnce.Do(func() { close(callbackDone) })
		return operationContext.Err()
	}, policy)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	defer manager.Close()
	if operationError = manager.Start(context.Background()); operationError != nil {
		testContext.Fatal(operationError)
	}
	job, operationError := manager.Create("session", "prompt", 0, "", "")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if difference := job.ExpiresAt - job.CreatedAt; difference < 29.9 || difference > 30.1 {
		testContext.Fatalf("retention = %v", difference)
	}
	_, operationError = manager.Create("session", "prompt", 4, "", "")
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if _, operationError = manager.Create("session", "prompt", 4, "", ""); !errors.Is(operationError, MaximumJobsError) || !strings.Contains(operationError.Error(), "2") {
		testContext.Fatalf("max jobs error = %v", operationError)
	}
	if operationError = manager.Start(context.Background()); operationError != nil {
		testContext.Fatal(operationError)
	}
	select {
	case <-blockStarted:
	case <-time.After(time.Second):
		testContext.Fatal("callback did not start")
	}
	select {
	case remaining := <-deadlineRemaining:
		if remaining <= 0 || remaining > 50*time.Millisecond {
			testContext.Fatalf("deadline remaining = %v", remaining)
		}
	case <-time.After(time.Second):
		testContext.Fatal("deadline missing")
	}
	select {
	case <-callbackDone:
	case <-time.After(time.Second):
		testContext.Fatal("callback did not observe timeout")
	}
}

func TestPolicyControlsLastErrorRunes(testContext *testing.T) {
	policy := testPolicy()
	policy.LastErrorRunes = 3
	manager, operationError := New(filepath.Join(testContext.TempDir(), "loops.json"), func(context.Context, Job, string) error { return errors.New("界界界界") }, policy)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if operationError := manager.Start(context.Background()); operationError != nil {
		testContext.Fatal(operationError)
	}
	testContext.Cleanup(manager.Close)
	if _, operationError := manager.Create("session", "prompt", 60, "", ""); operationError != nil {
		testContext.Fatal(operationError)
	}
	waitFor(testContext, func() bool { jobs := manager.List(""); return len(jobs) == 1 && jobs[0].LastError == "界界界" })
}
