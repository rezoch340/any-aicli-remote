// Configuration defaults and normalization. Every deployment-tunable value has
// its default here rather than at a call site.

package config

import (
	"path/filepath"
	"strings"
	"time"

	providerfactory "github.com/rezoch340/any-aicli-remote/backend/internal/provider/factory"
)

func duration(text string) Duration { parsed, _ := time.ParseDuration(text); return Duration{parsed} }
func DefaultDocument(home string) Document {
	data := filepath.Join(home, ".any-aicli-remote")
	return Document{Version: DocumentVersion, Network: NetworkDocument{Bind: "0.0.0.0", Port: DefaultPort}, Agent: AgentDocument{Host: "127.0.0.1", Port: DefaultAgentPort, Ensure: true}, Storage: StorageDocument{DataDirectory: data, RuntimeDirectory: filepath.Join(data, "run")}, Provider: ProviderDocument{ID: providerfactory.DefaultProviderID}, Tuning: defaultTuning()}
}
func NormalizeDocument(document Document, home string) Document {
	defaults := DefaultDocument(home)
	if strings.TrimSpace(document.Storage.DataDirectory) == "" {
		document.Storage.DataDirectory = defaults.Storage.DataDirectory
	}
	document.Storage.DataDirectory = filepath.Clean(expandHome(document.Storage.DataDirectory, home))
	if absolutePath, operationError := filepath.Abs(document.Storage.DataDirectory); operationError == nil {
		document.Storage.DataDirectory = absolutePath
	}
	if strings.TrimSpace(document.Storage.RuntimeDirectory) == "" {
		document.Storage.RuntimeDirectory = filepath.Join(document.Storage.DataDirectory, "run")
	}
	document.Storage.RuntimeDirectory = filepath.Clean(expandHome(document.Storage.RuntimeDirectory, home))
	if absolutePath, operationError := filepath.Abs(document.Storage.RuntimeDirectory); operationError == nil {
		document.Storage.RuntimeDirectory = absolutePath
	}
	if strings.TrimSpace(document.Provider.ExecutablePath) != "" {
		document.Provider.ExecutablePath = filepath.Clean(expandHome(document.Provider.ExecutablePath, home))
	}
	for optionName, optionValue := range document.Provider.Options {
		document.Provider.Options[optionName] = expandHome(optionValue, home)
	}
	return document
}
func defaultTuning() TuningDocument {
	return TuningDocument{
		HTTP:       HTTPDocument{ReadHeaderTimeout: duration("10s"), IdleTimeout: duration("75s"), ShutdownTimeout: duration("8s"), StartupFailureShutdownTimeout: duration("2s"), MaxHeaderBytes: 1 << 20, MaxRequestBodyBytes: 8 << 20, AuthenticationCookieMaxAge: duration("720h"), DeepHealthTimeout: duration("6s"), ExistingDaemonProbeTimeout: duration("2s"), ProviderRequestTimeout: duration("30s"), ErrorResponseMaxRunes: 300, DeepHealthDetailMaxRunes: 200, HealthProbeMaxBytes: 512},
		Hub:        HubDocument{ReadBufferBytes: 64 << 10, WriteBufferBytes: 64 << 10, MaxMessageBytes: 16 << 20, Heartbeat: duration("20s"), ClientReadTimeout: duration("60s"), WatcherEnsureInterval: duration("5s"), StateBroadcastInterval: duration("15s"), EnsureAttempt: duration("12s"), ClientConnectEnsure: duration("15s"), DialAttempts: 3, DialHandshake: duration("8s"), RetryDelay: duration("250ms"), WriteTimeout: duration("20s"), ControlWriteTimeout: duration("5s"), PendingLimit: 256, PendingClientLimit: 32, PendingTimeout: duration("30m"), NormalEnsure: duration("5s"), PatientEnsure: duration("18s"), NotificationEnsure: duration("3s"), ReverseOperationTimeout: duration("2m"), ReverseReadBytes: 2_000_000, TerminalOutputBytes: 1 << 20},
		History:    HistoryDocument{DefaultLimit: 100, LiveLimit: 400, MinLimit: 20, MaxLimit: 4000, DefaultMaxBytes: 400000, LiveMaxBytes: 512000, BeforeMaxBytes: 1200000, MinMaxBytes: 64000, MaxMaxBytes: 12000000, ProviderEventLimit: 1600, ProviderReadBytes: 8000000, TitleBatchLimit: 250, ChatTextMaxRunes: 120000, MessageScanInitialBytes: 64 * 1024, MessageScanMaxBytes: 8 * 1024 * 1024, MetadataTitleMaxRunes: 80, MetadataSummaryMaxRunes: 160, RenameTitleMaxRunes: 160},
		Lifecycle:  LifecycleDocument{KillGrace: duration("4s"), RestartWait: duration("3s"), RestartPoll: duration("100ms"), PostKillDelay: duration("100ms"), StopWait: duration("2s"), StopPoll: duration("50ms"), BootAgentTimeout: duration("18s"), HubEnsureTimeout: duration("18s"), ListenerPoll: duration("250ms"), DialTimeout: duration("500ms"), StackSettle: duration("350ms"), StackWait: duration("24s"), StartTimeout: duration("18s"), RestartTimeout: duration("18s")},
		Loops:      LoopsDocument{MinInterval: duration("60s"), MaxInterval: duration("168h"), DefaultInterval: duration("5m"), Retention: duration("168h"), MaxJobs: 50, FireTimeout: duration("10m"), LastErrorRunes: 200},
		Room:       RoomDocument{MessageRuneLimit: 240, SpeakerRuneLimit: 32, KindRuneLimit: 12, CompactionThreshold: 2000, CompactionRetainMessages: 1000, FeedDefaultLimit: 200, FeedMaxLimit: 500, MemberWindow: duration("15m"), ScannerInitialBytes: 64 * 1024, ScannerMaxBytes: 4 * 1024 * 1024},
		Filesystem: FilesystemDocument{MaxReadBytes: 2 * 1024 * 1024, MaxWriteBytes: 4 * 1024 * 1024, MaxListItems: 10_000},
		Skills:     SkillsDocument{MaxFileBytes: 1 << 20, DescriptionMaxRunes: 240, MaxItems: 2000},
		Voice:      VoiceDocument{RequestTimeout: duration("60s"), TextMaxRunes: 15000, TruncatedTextRunes: 14990, SuccessBodyMaxBytes: 64 * 1024 * 1024, ErrorBodyMaxBytes: 16 * 1024, ErrorBodyMaxRunes: 400},
		Git:        GitDocument{CommandTimeout: duration("12s"), DiffTimeout: duration("20s"), DirtyFileLimit: 80, DiffRuneLimit: 200000, LogDefaultLimit: 12, LogMaxLimit: 30, ContextFileReadBytes: 16000, ContextPreviewRunes: 4000, CommandOutputMaxBytes: 16 << 20},
	}
}
