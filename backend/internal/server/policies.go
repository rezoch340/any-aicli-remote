package server

import (
	"github.com/rezoch340/any-aicli-remote/backend/internal/config"
	"github.com/rezoch340/any-aicli-remote/backend/internal/fsapi"
	"github.com/rezoch340/any-aicli-remote/backend/internal/gitapi"
	"github.com/rezoch340/any-aicli-remote/backend/internal/hub"
	"github.com/rezoch340/any-aicli-remote/backend/internal/loops"
	processapi "github.com/rezoch340/any-aicli-remote/backend/internal/process"
	providerapi "github.com/rezoch340/any-aicli-remote/backend/internal/provider"
	"github.com/rezoch340/any-aicli-remote/backend/internal/room"
	"github.com/rezoch340/any-aicli-remote/backend/internal/skills"
	"github.com/rezoch340/any-aicli-remote/backend/internal/voice"
)

func hubPolicy(tuning config.TuningDocument) hub.Policy {
	document := tuning.Hub
	return hub.Policy{
		ReadBufferBytes: document.ReadBufferBytes, WriteBufferBytes: document.WriteBufferBytes,
		MaxMessageBytes: document.MaxMessageBytes, AgentMaxMessageBytes: document.AgentMaxMessageBytes,
		Heartbeat:              document.Heartbeat.Duration,
		ClientReadTimeout:      document.ClientReadTimeout.Duration,
		WatcherEnsureInterval:  document.WatcherEnsureInterval.Duration,
		StateBroadcastInterval: document.StateBroadcastInterval.Duration,
		EnsureAttempt:          document.EnsureAttempt.Duration,
		ClientConnectEnsure:    document.ClientConnectEnsure.Duration,
		DialAttempts:           document.DialAttempts, DialHandshake: document.DialHandshake.Duration,
		RetryDelay: document.RetryDelay.Duration, WriteTimeout: document.WriteTimeout.Duration,
		ControlWriteTimeout: document.ControlWriteTimeout.Duration,
		PendingLimit:        document.PendingLimit, PendingClientLimit: document.PendingClientLimit,
		PendingTimeout: document.PendingTimeout.Duration, NormalEnsure: document.NormalEnsure.Duration,
		PatientEnsure:           document.PatientEnsure.Duration,
		NotificationEnsure:      document.NotificationEnsure.Duration,
		ReverseOperationTimeout: document.ReverseOperationTimeout.Duration,
		ReverseReadBytes:        document.ReverseReadBytes, TerminalOutputBytes: document.TerminalOutputBytes,
		FilesystemPolicy: filesystemPolicy(tuning.Filesystem),
	}
}

func processLifecyclePolicy(document config.LifecycleDocument) processapi.LifecyclePolicy {
	return processapi.LifecyclePolicy{
		KillGrace:     document.KillGrace.Duration,
		RestartWait:   document.RestartWait.Duration,
		RestartPoll:   document.RestartPoll.Duration,
		PostKillDelay: document.PostKillDelay.Duration,
		StopWait:      document.StopWait.Duration,
		StopPoll:      document.StopPoll.Duration,
	}
}

func loopsPolicy(document config.LoopsDocument) loops.Policy {
	return loops.Policy{MinInterval: document.MinInterval.Duration, MaxInterval: document.MaxInterval.Duration, DefaultInterval: document.DefaultInterval.Duration, Retention: document.Retention.Duration, MaxJobs: document.MaxJobs, FireTimeout: document.FireTimeout.Duration, LastErrorRunes: document.LastErrorRunes}
}

func historyPolicy(document config.HistoryDocument) providerapi.HistoryPolicy {
	return providerapi.HistoryPolicy{DefaultLimit: document.DefaultLimit, LiveLimit: document.LiveLimit, MinLimit: document.MinLimit, MaxLimit: document.MaxLimit, DefaultMaxBytes: document.DefaultMaxBytes, LiveMaxBytes: document.LiveMaxBytes, BeforeMaxBytes: document.BeforeMaxBytes, MinMaxBytes: document.MinMaxBytes, MaxMaxBytes: document.MaxMaxBytes, AdapterEventLimit: document.ProviderEventLimit, AdapterReadBytes: document.ProviderReadBytes, TitleBatchLimit: document.TitleBatchLimit, ChatTextMaxRunes: document.ChatTextMaxRunes, MessageScanInitialBytes: document.MessageScanInitialBytes, MessageScanMaxBytes: document.MessageScanMaxBytes, MetadataTitleMaxRunes: document.MetadataTitleMaxRunes, MetadataSummaryMaxRunes: document.MetadataSummaryMaxRunes, RenameTitleMaxRunes: document.RenameTitleMaxRunes}
}

func filesystemPolicy(document config.FilesystemDocument) fsapi.Policy {
	return fsapi.Policy{MaxReadBytes: document.MaxReadBytes, MaxWriteBytes: document.MaxWriteBytes, MaxListItems: document.MaxListItems}
}
func gitPolicy(document config.GitDocument) gitapi.Policy {
	return gitapi.Policy{CommandTimeout: document.CommandTimeout.Duration, DiffTimeout: document.DiffTimeout.Duration, DirtyFileLimit: document.DirtyFileLimit, DiffRuneLimit: document.DiffRuneLimit, LogDefaultLimit: document.LogDefaultLimit, LogMaxLimit: document.LogMaxLimit, ContextFileReadBytes: document.ContextFileReadBytes, ContextPreviewRunes: document.ContextPreviewRunes, CommandOutputMaxBytes: document.CommandOutputMaxBytes}
}

func roomPolicy(document config.RoomDocument) room.Policy {
	return room.Policy{MessageRuneLimit: document.MessageRuneLimit, SpeakerRuneLimit: document.SpeakerRuneLimit, KindRuneLimit: document.KindRuneLimit, CompactionThreshold: document.CompactionThreshold, CompactionRetainMessages: document.CompactionRetainMessages, FeedDefaultLimit: document.FeedDefaultLimit, FeedMaxLimit: document.FeedMaxLimit, MemberWindow: document.MemberWindow.Duration, ScannerInitialBytes: document.ScannerInitialBytes, ScannerMaxBytes: document.ScannerMaxBytes}
}

func voicePolicy(document config.VoiceDocument) voice.Policy {
	return voice.Policy{RequestTimeout: document.RequestTimeout.Duration, TextMaxRunes: document.TextMaxRunes, TruncatedTextRunes: document.TruncatedTextRunes, SuccessBodyMaxBytes: document.SuccessBodyMaxBytes, ErrorBodyMaxBytes: document.ErrorBodyMaxBytes, ErrorBodyMaxRunes: document.ErrorBodyMaxRunes}
}

func skillsPolicy(document config.SkillsDocument) skills.Policy {
	return skills.Policy{MaxFileBytes: document.MaxFileBytes, DescriptionMaxRunes: document.DescriptionMaxRunes, MaxItems: document.MaxItems}
}
