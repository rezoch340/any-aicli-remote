package server

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/config"
)

func TestNewMapsCanonicalTuning(testingContext *testing.T) {
	fixture := newRouteTestServer(testingContext)
	raw := fixture.server.configuration
	canonical := config.DefaultDocument(testingContext.TempDir())
	canonical.Tuning.Filesystem.MaxReadBytes = 101
	canonical.Tuning.Filesystem.MaxWriteBytes = 202
	canonical.Tuning.Filesystem.MaxListItems = 303
	canonical.Tuning.Git.CommandTimeout.Duration = 13 * time.Millisecond
	canonical.Tuning.Git.DiffTimeout.Duration = 17 * time.Millisecond
	canonical.Tuning.Git.DirtyFileLimit = 3
	canonical.Tuning.Git.DiffRuneLimit = 4
	canonical.Tuning.Git.LogDefaultLimit = 2
	canonical.Tuning.Git.LogMaxLimit = 5
	canonical.Tuning.Git.ContextFileReadBytes = 303
	canonical.Tuning.Git.ContextPreviewRunes = 6
	canonical.Tuning.Git.CommandOutputMaxBytes = 404
	canonical.Tuning.Hub.PendingLimit = 11
	canonical.Tuning.Hub.PendingClientLimit = 5
	canonical.Tuning.Hub.NotificationEnsure.Duration = 37 * time.Millisecond
	canonical.Tuning.Lifecycle.KillGrace.Duration = 41 * time.Millisecond
	canonical.Tuning.Lifecycle.RestartWait.Duration = 43 * time.Millisecond
	canonical.Tuning.Lifecycle.RestartPoll.Duration = 5 * time.Millisecond
	canonical.Tuning.Lifecycle.PostKillDelay.Duration = 7 * time.Millisecond
	canonical.Tuning.Lifecycle.StopWait.Duration = 47 * time.Millisecond
	canonical.Tuning.Lifecycle.StopPoll.Duration = 11 * time.Millisecond
	canonical.Tuning.Loops.MinInterval.Duration = 2 * time.Second
	canonical.Tuning.Loops.DefaultInterval.Duration = 4 * time.Second
	canonical.Tuning.Loops.MaxInterval.Duration = 9 * time.Second
	canonical.Tuning.Loops.Retention.Duration = 30 * time.Second
	canonical.Tuning.Loops.MaxJobs = 2
	canonical.Tuning.Loops.FireTimeout.Duration = 25 * time.Millisecond
	canonical.Tuning.Loops.LastErrorRunes = 17
	canonical.Tuning.Room.MessageRuneLimit = 19
	canonical.Tuning.Room.SpeakerRuneLimit = 18
	canonical.Tuning.Room.KindRuneLimit = 17
	canonical.Tuning.Room.CompactionThreshold = 16
	canonical.Tuning.Room.CompactionRetainMessages = 15
	canonical.Tuning.Room.FeedDefaultLimit = 14
	canonical.Tuning.Room.FeedMaxLimit = 15
	canonical.Tuning.Room.MemberWindow.Duration = 13 * time.Minute
	canonical.Tuning.Room.ScannerInitialBytes = 12
	canonical.Tuning.Room.ScannerMaxBytes = 13
	canonical.Tuning.History.DefaultLimit = 10
	canonical.Tuning.History.LiveLimit = 20
	canonical.Tuning.History.MinLimit = 5
	canonical.Tuning.History.MaxLimit = 30
	canonical.Tuning.History.DefaultMaxBytes = 100
	canonical.Tuning.History.LiveMaxBytes = 200
	canonical.Tuning.History.BeforeMaxBytes = 300
	canonical.Tuning.History.MinMaxBytes = 80
	canonical.Tuning.History.MaxMaxBytes = 400
	canonical.Tuning.History.ProviderEventLimit = 40
	canonical.Tuning.History.ProviderReadBytes = 500
	canonical.Tuning.History.TitleBatchLimit = 6
	canonical.Tuning.History.ChatTextMaxRunes = 7
	canonical.Tuning.History.MessageScanInitialBytes = 8
	canonical.Tuning.History.MessageScanMaxBytes = 9
	canonical.Tuning.History.MetadataTitleMaxRunes = 10
	canonical.Tuning.History.MetadataSummaryMaxRunes = 11
	canonical.Tuning.History.RenameTitleMaxRunes = 12
	canonical.Tuning.Voice.RequestTimeout.Duration = 13 * time.Millisecond
	canonical.Tuning.Voice.TextMaxRunes = 14
	canonical.Tuning.Voice.TruncatedTextRunes = 13
	canonical.Tuning.Voice.SuccessBodyMaxBytes = 15
	canonical.Tuning.Voice.ErrorBodyMaxBytes = 16
	canonical.Tuning.Voice.ErrorBodyMaxRunes = 17
	canonical.Tuning.Skills.MaxFileBytes = 18
	canonical.Tuning.Skills.DescriptionMaxRunes = 19
	canonical.Tuning.Skills.MaxItems = 20
	raw.Canonical = canonical
	serverInstance, operationError := New(raw, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	testingContext.Cleanup(serverInstance.Close)
	policy := serverInstance.hub.Policy()
	if serverInstance.hub == nil || policy.PendingLimit != 11 || policy.PendingClientLimit != 5 || policy.NotificationEnsure != 37*time.Millisecond {
		testingContext.Fatalf("hub tuning was not mapped: %#v", policy)
	}
	if serverInstance.filesystemPolicy.MaxReadBytes != 101 || serverInstance.filesystemPolicy.MaxWriteBytes != 202 || serverInstance.filesystemPolicy.MaxListItems != 303 || policy.FilesystemPolicy != serverInstance.filesystemPolicy {
		testingContext.Fatalf("filesystem policy was not mapped: %#v / %#v", serverInstance.filesystemPolicy, policy.FilesystemPolicy)
	}
	if serverInstance.gitPolicy.CommandTimeout != 13*time.Millisecond || serverInstance.gitPolicy.DiffTimeout != 17*time.Millisecond || serverInstance.gitPolicy.DirtyFileLimit != 3 || serverInstance.gitPolicy.DiffRuneLimit != 4 || serverInstance.gitPolicy.LogDefaultLimit != 2 || serverInstance.gitPolicy.LogMaxLimit != 5 || serverInstance.gitPolicy.ContextFileReadBytes != 303 || serverInstance.gitPolicy.ContextPreviewRunes != 6 || serverInstance.gitPolicy.CommandOutputMaxBytes != 404 {
		testingContext.Fatalf("git policy was not mapped: %#v", serverInstance.gitPolicy)
	}
	lifecycle := serverInstance.process.Config.LifecyclePolicy
	if lifecycle.KillGrace != 41*time.Millisecond || lifecycle.RestartWait != 43*time.Millisecond || lifecycle.RestartPoll != 5*time.Millisecond || lifecycle.PostKillDelay != 7*time.Millisecond || lifecycle.StopWait != 47*time.Millisecond || lifecycle.StopPoll != 11*time.Millisecond {
		testingContext.Fatalf("process lifecycle was not mapped: %#v", lifecycle)
	}
	historyPolicy := serverInstance.session.HistoryPolicy()
	if historyPolicy.DefaultLimit != 10 || historyPolicy.LiveLimit != 20 || historyPolicy.MinLimit != 5 || historyPolicy.MaxLimit != 30 || historyPolicy.DefaultMaxBytes != 100 || historyPolicy.LiveMaxBytes != 200 || historyPolicy.BeforeMaxBytes != 300 || historyPolicy.MinMaxBytes != 80 || historyPolicy.MaxMaxBytes != 400 || historyPolicy.AdapterEventLimit != 40 || historyPolicy.AdapterReadBytes != 500 || historyPolicy.TitleBatchLimit != 6 || historyPolicy.ChatTextMaxRunes != 7 || historyPolicy.MessageScanInitialBytes != 8 || historyPolicy.MessageScanMaxBytes != 9 || historyPolicy.MetadataTitleMaxRunes != 10 || historyPolicy.MetadataSummaryMaxRunes != 11 || historyPolicy.RenameTitleMaxRunes != 12 {
		testingContext.Fatalf("history policy was not mapped: %#v", historyPolicy)
	}
	if serverInstance.skillsPolicy.MaxFileBytes != 18 || serverInstance.skillsPolicy.DescriptionMaxRunes != 19 || serverInstance.skillsPolicy.MaxItems != 20 {
		testingContext.Fatalf("skills policy was not mapped: %#v", serverInstance.skillsPolicy)
	}
	if serverInstance.voicePolicy.RequestTimeout != 13*time.Millisecond || serverInstance.voicePolicy.TextMaxRunes != 14 || serverInstance.voicePolicy.TruncatedTextRunes != 13 || serverInstance.voicePolicy.SuccessBodyMaxBytes != 15 || serverInstance.voicePolicy.ErrorBodyMaxBytes != 16 || serverInstance.voicePolicy.ErrorBodyMaxRunes != 17 {
		testingContext.Fatalf("voice policy was not mapped: %#v", serverInstance.voicePolicy)
	}
	loopPolicy := serverInstance.loops.Policy()
	if loopPolicy.MinInterval != 2*time.Second || loopPolicy.DefaultInterval != 4*time.Second || loopPolicy.MaxInterval != 9*time.Second || loopPolicy.Retention != 30*time.Second || loopPolicy.MaxJobs != 2 || loopPolicy.FireTimeout != 25*time.Millisecond || loopPolicy.LastErrorRunes != 17 {
		testingContext.Fatalf("loops policy was not mapped: %#v", loopPolicy)
	}
	roomPolicy := serverInstance.room.Policy()
	if roomPolicy.MessageRuneLimit != 19 || roomPolicy.SpeakerRuneLimit != 18 || roomPolicy.KindRuneLimit != 17 || roomPolicy.CompactionThreshold != 16 || roomPolicy.CompactionRetainMessages != 15 || roomPolicy.FeedDefaultLimit != 14 || roomPolicy.FeedMaxLimit != 15 || roomPolicy.MemberWindow != 13*time.Minute || roomPolicy.ScannerInitialBytes != 12 || roomPolicy.ScannerMaxBytes != 13 {
		testingContext.Fatalf("room policy was not mapped: %#v", roomPolicy)
	}
	seconds, label, intervalError := loopInterval(loopPolicy, nil)
	if intervalError != nil || seconds != 4 || label != "4s" {
		testingContext.Fatalf("default loop interval = %d %s %v", seconds, label, intervalError)
	}
}
