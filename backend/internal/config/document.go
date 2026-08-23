package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/rezoch340/any-aicli-remote/backend/internal/atomicfile"
	providerfactory "github.com/rezoch340/any-aicli-remote/backend/internal/provider/factory"
)

const (
	DocumentVersion = 1
	// BootstrapDocumentMaxBytes bounds configuration bootstrap input before runtime tuning is available.
	BootstrapDocumentMaxBytes int64 = 4 << 20
)

var DocumentTooLargeError = errors.New("configuration document exceeds bootstrap size limit")

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
func DecodeDocument(data []byte) (Document, error) {
	home, operationError := os.UserHomeDir()
	if operationError != nil {
		return Document{}, operationError
	}
	return DecodeDocumentReader(bytes.NewReader(data), home)
}
func decodeDocument(data []byte, home string) (Document, error) {
	var envelope map[string]json.RawMessage
	if operationError := json.Unmarshal(data, &envelope); operationError != nil {
		return Document{}, operationError
	}
	versionData, present := envelope["version"]
	if !present {
		return migrateV0(data, home)
	}
	var versionValue int
	if operationError := json.Unmarshal(versionData, &versionValue); operationError != nil {
		return Document{}, errors.New("version must be an integer")
	}
	if versionValue == 0 {
		return migrateV0(data, home)
	}
	var document Document
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if operationError := decoder.Decode(&document); operationError != nil {
		return document, operationError
	}
	var trailing json.RawMessage
	if operationError := decoder.Decode(&trailing); operationError == nil {
		return document, errors.New("trailing JSON data")
	} else if !errors.Is(operationError, io.EOF) {
		return document, fmt.Errorf("invalid trailing JSON: %w", operationError)
	}
	if document.Version == 0 {
		return migrateV0(data, home)
	}
	if document.Version != DocumentVersion {
		return document, fmt.Errorf("unsupported config version %d", document.Version)
	}
	return document, ValidateDocument(document)
}

// DecodeDocumentReader decodes one bounded configuration document using the canonical schema path.
func DecodeDocumentReader(reader io.Reader, home string) (Document, error) {
	limited := io.LimitReader(reader, BootstrapDocumentMaxBytes+1)
	data, operationError := io.ReadAll(limited)
	if operationError != nil {
		return Document{}, operationError
	}
	if int64(len(data)) > BootstrapDocumentMaxBytes {
		return Document{}, DocumentTooLargeError
	}
	return decodeDocument(data, home)
}

// DecodeAndNormalizeDocumentReader decodes candidate configuration with the same bounded schema and normalization path.
func DecodeAndNormalizeDocumentReader(reader io.Reader) (Document, error) {
	home, operationError := os.UserHomeDir()
	if operationError != nil {
		return Document{}, operationError
	}
	document, operationError := DecodeDocumentReader(reader, home)
	if operationError != nil {
		return Document{}, operationError
	}
	document = NormalizeDocument(document, home)
	if operationError := ValidateDocument(document); operationError != nil {
		return Document{}, operationError
	}
	return document, nil
}

func LoadDocument(path string, home string) (Document, error) {
	file, operationError := os.Open(path)
	if os.IsNotExist(operationError) {
		return NormalizeDocument(DefaultDocument(home), home), nil
	}
	if operationError != nil {
		return Document{}, operationError
	}
	defer file.Close()
	document, operationError := DecodeDocumentReader(file, home)
	if operationError != nil {
		return Document{}, operationError
	}
	return NormalizeDocument(document, home), nil
}
func SaveDocument(path string, document Document) error {
	if operationError := ValidateDocument(document); operationError != nil {
		return operationError
	}
	data, operationError := json.MarshalIndent(document, "", "  ")
	if operationError != nil {
		return operationError
	}
	return atomicfile.WritePrivate(path, append(data, '\n'))
}

func ValidateDocument(document Document) error {
	if document.Version != DocumentVersion {
		return errors.New("invalid config version")
	}
	if strings.TrimSpace(document.Network.Bind) == "" || strings.TrimSpace(document.Agent.Host) == "" {
		return errors.New("bind and agent host are required")
	}
	if document.Network.Port < 1 || document.Network.Port > 65535 || document.Agent.Port < 1 || document.Agent.Port > 65535 || document.Network.Port == document.Agent.Port {
		return errors.New("ports must be distinct values between 1 and 65535")
	}
	if strings.TrimSpace(document.Storage.DataDirectory) == "" || strings.TrimSpace(document.Storage.RuntimeDirectory) == "" {
		return errors.New("storage directories are required")
	}
	if strings.TrimSpace(document.Provider.ID) == "" {
		return errors.New("provider id is required")
	}
	if operationError := validateOptions(document.Provider.Options); operationError != nil {
		return operationError
	}
	return validateTuning(document.Tuning)
}
func validateOptions(options map[string]string) error {
	for key := range options {
		lowered := strings.ToLower(key)
		for _, word := range []string{"secret", "token", "password", "credential", "api_key", "key"} {
			if strings.Contains(lowered, word) {
				return fmt.Errorf("provider option %q may contain secret material", key)
			}
		}
	}
	return nil
}
func validateTuning(tuning TuningDocument) error {
	if operationError := validatePositiveTuning(reflect.ValueOf(tuning)); operationError != nil {
		return operationError
	}
	if tuning.History.MinLimit > tuning.History.DefaultLimit || tuning.History.DefaultLimit > tuning.History.MaxLimit || tuning.History.MinLimit > tuning.History.LiveLimit || tuning.History.LiveLimit > tuning.History.MaxLimit {
		return errors.New("history limits are inconsistent")
	}
	if tuning.Room.CompactionRetainMessages > tuning.Room.CompactionThreshold || tuning.Room.FeedDefaultLimit > tuning.Room.FeedMaxLimit || tuning.Room.ScannerInitialBytes > tuning.Room.ScannerMaxBytes {
		return errors.New("room limits are inconsistent")
	}
	if tuning.Git.LogDefaultLimit > tuning.Git.LogMaxLimit {
		return errors.New("git log limits are inconsistent")
	}
	if tuning.Filesystem.MaxReadBytes >= math.MaxInt64 || tuning.Filesystem.MaxWriteBytes >= math.MaxInt64 || tuning.Filesystem.MaxListItems >= int(^uint(0)>>1) {
		return errors.New("filesystem limits must leave room for sentinels")
	}
	if tuning.Skills.MaxFileBytes >= math.MaxInt64 {
		return errors.New("skills file limit must leave room for sentinel")
	}
	if tuning.Git.ContextFileReadBytes >= math.MaxInt64 || tuning.Git.CommandOutputMaxBytes >= math.MaxInt64 {
		return errors.New("git context read limit overflows sentinel")
	}
	if tuning.Hub.PendingClientLimit > tuning.Hub.PendingLimit {
		return errors.New("pending client limit exceeds pending limit")
	}
	if tuning.History.MessageScanInitialBytes > tuning.History.MessageScanMaxBytes {
		return errors.New("history message scanner limits are inconsistent")
	}
	if tuning.Voice.TruncatedTextRunes >= tuning.Voice.TextMaxRunes || tuning.Voice.SuccessBodyMaxBytes >= math.MaxInt64 || tuning.Voice.ErrorBodyMaxBytes >= math.MaxInt64 {
		return errors.New("voice limits are inconsistent")
	}
	if tuning.History.MinMaxBytes > tuning.History.DefaultMaxBytes || tuning.History.DefaultMaxBytes > tuning.History.MaxMaxBytes || tuning.History.MinMaxBytes > tuning.History.LiveMaxBytes || tuning.History.LiveMaxBytes > tuning.History.MaxMaxBytes || tuning.History.MinMaxBytes > tuning.History.BeforeMaxBytes || tuning.History.BeforeMaxBytes > tuning.History.MaxMaxBytes {
		return errors.New("history byte limits are inconsistent")
	}
	if tuning.Loops.MinInterval.Duration > tuning.Loops.DefaultInterval.Duration || tuning.Loops.DefaultInterval.Duration > tuning.Loops.MaxInterval.Duration {
		return errors.New("loop intervals are inconsistent")
	}
	if tuning.Loops.MinInterval.Duration%time.Second != 0 || tuning.Loops.DefaultInterval.Duration%time.Second != 0 || tuning.Loops.MaxInterval.Duration%time.Second != 0 {
		return errors.New("loop intervals must be whole seconds")
	}
	cookieSeconds := int64(tuning.HTTP.AuthenticationCookieMaxAge.Duration / time.Second)
	maximumInt := int64(^uint(0) >> 1)
	if tuning.HTTP.AuthenticationCookieMaxAge.Duration%time.Second != 0 || cookieSeconds > maximumInt {
		return errors.New("authentication cookie max age must be whole seconds within platform int range")
	}
	return nil
}

func validatePositiveTuning(value reflect.Value) error {
	if value.Kind() == reflect.Struct {
		for fieldIndex := 0; fieldIndex < value.NumField(); fieldIndex++ {
			if operationError := validatePositiveTuning(value.Field(fieldIndex)); operationError != nil {
				return operationError
			}
		}
		return nil
	}
	if value.Kind() == reflect.Int || value.Kind() == reflect.Int64 {
		if value.Int() <= 0 {
			return errors.New("tuning values must be positive")
		}
	}
	return nil
}
func migrateV0(data []byte, home string) (Document, error) {
	var old struct {
		Bind             string            `json:"bind"`
		Port             int               `json:"port"`
		AgentHost        string            `json:"agentHost"`
		AgentPort        int               `json:"agentPort"`
		RuntimeDirectory string            `json:"runtimeDirectory"`
		PublicHost       string            `json:"publicHost"`
		ProviderID       string            `json:"providerID"`
		ProviderPath     string            `json:"providerPath"`
		DataDirectory    string            `json:"dataDirectory"`
		EnsureAgent      *bool             `json:"ensureAgent"`
		StopAgentOnExit  *bool             `json:"stopAgentOnExit"`
		ProviderOptions  map[string]string `json:"providerOptions"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if operationError := decoder.Decode(&old); operationError != nil {
		return Document{}, operationError
	}
	doc := DefaultDocument(home)
	if old.Bind != "" {
		doc.Network.Bind = old.Bind
	}
	if old.Port != 0 {
		doc.Network.Port = old.Port
	}
	if old.PublicHost != "" {
		doc.Network.PublicHost = old.PublicHost
	}
	if old.AgentHost != "" {
		doc.Agent.Host = old.AgentHost
	}
	if old.AgentPort != 0 {
		doc.Agent.Port = old.AgentPort
	}
	if old.EnsureAgent != nil {
		doc.Agent.Ensure = *old.EnsureAgent
	}
	if old.StopAgentOnExit != nil {
		doc.Agent.StopOnExit = *old.StopAgentOnExit
	}
	if old.RuntimeDirectory != "" {
		doc.Storage.RuntimeDirectory = old.RuntimeDirectory
	}
	if old.DataDirectory != "" {
		doc.Storage.DataDirectory = old.DataDirectory
	}
	if old.ProviderID != "" {
		doc.Provider.ID = old.ProviderID
	}
	if old.ProviderPath != "" {
		doc.Provider.ExecutablePath = old.ProviderPath
	}
	if old.ProviderOptions != nil {
		doc.Provider.Options = old.ProviderOptions
	}
	return doc, ValidateDocument(doc)
}
