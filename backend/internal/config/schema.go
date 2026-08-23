// Canonical configuration schema. These types are the single serialized shape
// shared by the daemon, the command-line interface, and the macOS launcher.

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Duration struct{ time.Duration }

func (value Duration) MarshalJSON() ([]byte, error) { return json.Marshal(value.String()) }
func (value *Duration) UnmarshalJSON(data []byte) error {
	var text string
	if operationError := json.Unmarshal(data, &text); operationError != nil {
		return errors.New("duration must be a string")
	}
	parsed, operationError := time.ParseDuration(text)
	if operationError != nil || parsed <= 0 {
		return fmt.Errorf("invalid positive duration %q", text)
	}
	value.Duration = parsed
	return nil
}

type Document struct {
	Version  int              `json:"version"`
	Network  NetworkDocument  `json:"network"`
	Agent    AgentDocument    `json:"agent"`
	Storage  StorageDocument  `json:"storage"`
	Provider ProviderDocument `json:"provider"`
	Tuning   TuningDocument   `json:"tuning"`
}
type NetworkDocument struct {
	Bind       string `json:"bind"`
	Port       int    `json:"port"`
	PublicHost string `json:"public_host,omitempty"`
}
type AgentDocument struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Ensure     bool   `json:"ensure"`
	StopOnExit bool   `json:"stop_on_exit"`
}
type StorageDocument struct {
	DataDirectory    string `json:"data_directory"`
	RuntimeDirectory string `json:"runtime_directory"`
}
type ProviderDocument struct {
	ID             string            `json:"id"`
	ExecutablePath string            `json:"executable_path,omitempty"`
	Options        map[string]string `json:"options,omitempty"`
}
type TuningDocument struct {
	HTTP       HTTPDocument       `json:"http"`
	Hub        HubDocument        `json:"hub"`
	History    HistoryDocument    `json:"history"`
	Lifecycle  LifecycleDocument  `json:"lifecycle"`
	Loops      LoopsDocument      `json:"loops"`
	Room       RoomDocument       `json:"room"`
	Filesystem FilesystemDocument `json:"filesystem"`
	Voice      VoiceDocument      `json:"voice"`
	Skills     SkillsDocument     `json:"skills"`
	Git        GitDocument        `json:"git"`
}
type VoiceDocument struct {
	RequestTimeout      Duration `json:"request_timeout"`
	TextMaxRunes        int      `json:"text_max_runes"`
	TruncatedTextRunes  int      `json:"truncated_text_runes"`
	SuccessBodyMaxBytes int64    `json:"success_body_max_bytes"`
	ErrorBodyMaxBytes   int64    `json:"error_body_max_bytes"`
	ErrorBodyMaxRunes   int      `json:"error_body_max_runes"`
}
type RoomDocument struct {
	MessageRuneLimit         int      `json:"message_rune_limit"`
	SpeakerRuneLimit         int      `json:"speaker_rune_limit"`
	KindRuneLimit            int      `json:"kind_rune_limit"`
	CompactionThreshold      int      `json:"compaction_threshold"`
	CompactionRetainMessages int      `json:"compaction_retain_messages"`
	FeedDefaultLimit         int      `json:"feed_default_limit"`
	FeedMaxLimit             int      `json:"feed_max_limit"`
	MemberWindow             Duration `json:"member_window"`
	ScannerInitialBytes      int      `json:"scanner_initial_bytes"`
	ScannerMaxBytes          int      `json:"scanner_max_bytes"`
}
type FilesystemDocument struct {
	MaxReadBytes  int64 `json:"max_read_bytes"`
	MaxWriteBytes int64 `json:"max_write_bytes"`
	MaxListItems  int   `json:"max_list_items"`
}
type GitDocument struct {
	CommandTimeout        Duration `json:"command_timeout"`
	DiffTimeout           Duration `json:"diff_timeout"`
	DirtyFileLimit        int      `json:"dirty_file_limit"`
	DiffRuneLimit         int      `json:"diff_rune_limit"`
	LogDefaultLimit       int      `json:"log_default_limit"`
	LogMaxLimit           int      `json:"log_max_limit"`
	ContextFileReadBytes  int64    `json:"context_file_read_bytes"`
	ContextPreviewRunes   int      `json:"context_preview_runes"`
	CommandOutputMaxBytes int64    `json:"command_output_max_bytes"`
}
type SkillsDocument struct {
	MaxFileBytes        int64 `json:"max_file_bytes"`
	DescriptionMaxRunes int   `json:"description_max_runes"`
	MaxItems            int   `json:"max_items"`
}
type HTTPDocument struct {
	ReadHeaderTimeout             Duration `json:"read_header_timeout"`
	IdleTimeout                   Duration `json:"idle_timeout"`
	ShutdownTimeout               Duration `json:"shutdown_timeout"`
	StartupFailureShutdownTimeout Duration `json:"startup_failure_shutdown_timeout"`
	MaxHeaderBytes                int      `json:"max_header_bytes"`
	MaxRequestBodyBytes           int64    `json:"max_request_body_bytes"`
	AuthenticationCookieMaxAge    Duration `json:"authentication_cookie_max_age"`
	DeepHealthTimeout             Duration `json:"deep_health_timeout"`
	ExistingDaemonProbeTimeout    Duration `json:"existing_daemon_probe_timeout"`
	ProviderRequestTimeout        Duration `json:"provider_request_timeout"`
	ErrorResponseMaxRunes         int      `json:"error_response_max_runes"`
	DeepHealthDetailMaxRunes      int      `json:"deep_health_detail_max_runes"`
	HealthProbeMaxBytes           int64    `json:"health_probe_max_bytes"`
}
type HubDocument struct {
	ReadBufferBytes         int      `json:"read_buffer_bytes"`
	WriteBufferBytes        int      `json:"write_buffer_bytes"`
	MaxMessageBytes         int64    `json:"max_message_bytes"`
	Heartbeat               Duration `json:"heartbeat"`
	ClientReadTimeout       Duration `json:"client_read_timeout"`
	WatcherEnsureInterval   Duration `json:"watcher_ensure_interval"`
	StateBroadcastInterval  Duration `json:"state_broadcast_interval"`
	EnsureAttempt           Duration `json:"ensure_attempt"`
	ClientConnectEnsure     Duration `json:"client_connect_ensure"`
	DialAttempts            int      `json:"dial_attempts"`
	DialHandshake           Duration `json:"dial_handshake"`
	RetryDelay              Duration `json:"retry_delay"`
	WriteTimeout            Duration `json:"write_timeout"`
	ControlWriteTimeout     Duration `json:"control_write_timeout"`
	PendingLimit            int      `json:"pending_limit"`
	PendingClientLimit      int      `json:"pending_client_limit"`
	PendingTimeout          Duration `json:"pending_timeout"`
	NormalEnsure            Duration `json:"normal_ensure"`
	PatientEnsure           Duration `json:"patient_ensure"`
	NotificationEnsure      Duration `json:"notification_ensure"`
	ReverseOperationTimeout Duration `json:"reverse_operation_timeout"`
	ReverseReadBytes        int64    `json:"reverse_read_bytes"`
	TerminalOutputBytes     int64    `json:"terminal_output_bytes"`
}
type HistoryDocument struct {
	DefaultLimit            int   `json:"default_limit"`
	LiveLimit               int   `json:"live_limit"`
	MinLimit                int   `json:"min_limit"`
	MaxLimit                int   `json:"max_limit"`
	DefaultMaxBytes         int64 `json:"default_max_bytes"`
	LiveMaxBytes            int64 `json:"live_max_bytes"`
	BeforeMaxBytes          int64 `json:"before_max_bytes"`
	MinMaxBytes             int64 `json:"min_max_bytes"`
	MaxMaxBytes             int64 `json:"max_max_bytes"`
	ProviderEventLimit      int   `json:"provider_event_limit"`
	ProviderReadBytes       int64 `json:"provider_read_bytes"`
	TitleBatchLimit         int   `json:"title_batch_limit"`
	ChatTextMaxRunes        int   `json:"chat_text_max_runes"`
	MessageScanInitialBytes int   `json:"message_scan_initial_bytes"`
	MessageScanMaxBytes     int   `json:"message_scan_max_bytes"`
	MetadataTitleMaxRunes   int   `json:"metadata_title_max_runes"`
	MetadataSummaryMaxRunes int   `json:"metadata_summary_max_runes"`
	RenameTitleMaxRunes     int   `json:"rename_title_max_runes"`
}
type LifecycleDocument struct {
	KillGrace        Duration `json:"kill_grace"`
	RestartWait      Duration `json:"restart_wait"`
	RestartPoll      Duration `json:"restart_poll"`
	PostKillDelay    Duration `json:"post_kill_delay"`
	StopWait         Duration `json:"stop_wait"`
	StopPoll         Duration `json:"stop_poll"`
	BootAgentTimeout Duration `json:"boot_agent_timeout"`
	HubEnsureTimeout Duration `json:"hub_ensure_timeout"`
	ListenerPoll     Duration `json:"listener_poll"`
	DialTimeout      Duration `json:"dial_timeout"`
	StackSettle      Duration `json:"stack_settle"`
	StackWait        Duration `json:"stack_wait"`
	StartTimeout     Duration `json:"start_timeout"`
	RestartTimeout   Duration `json:"restart_timeout"`
}
type LoopsDocument struct {
	MinInterval     Duration `json:"min_interval"`
	MaxInterval     Duration `json:"max_interval"`
	DefaultInterval Duration `json:"default_interval"`
	Retention       Duration `json:"retention"`
	MaxJobs         int      `json:"max_jobs"`
	FireTimeout     Duration `json:"fire_timeout"`
	LastErrorRunes  int      `json:"last_error_runes"`
}
