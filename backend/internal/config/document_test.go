package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestDocumentRoundTripAndNormalize(testingContext *testing.T) {
	home := testingContext.TempDir()
	document := DefaultDocument(home)
	document.Storage.DataDirectory = "~/data"
	document.Storage.RuntimeDirectory = ""
	document = NormalizeDocument(document, home)
	if document.Storage.DataDirectory != filepath.Join(home, "data") || !filepath.IsAbs(document.Storage.RuntimeDirectory) {
		testingContext.Fatal(document.Storage)
	}
	target := filepath.Join(home, "nested", "config.json")
	if operationError := os.MkdirAll(filepath.Dir(target), 0755); operationError != nil {
		testingContext.Fatal(operationError)
	}
	if operationError := SaveDocument(target, document); operationError != nil {
		testingContext.Fatal(operationError)
	}
	information, operationError := os.Stat(target)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if information.Mode().Perm() != 0600 {
		testingContext.Fatalf("mode %o", information.Mode().Perm())
	}
	if parentInformation, parentError := os.Stat(filepath.Dir(target)); parentError != nil {
		testingContext.Fatalf("parent stat: %v", parentError)
	} else if parentInformation.Mode().Perm() != 0755 {
		testingContext.Fatalf("parent mode: %o", parentInformation.Mode().Perm())
	}
	loaded, operationError := LoadDocument(target, home)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if loaded.Tuning.History.LiveLimit != 400 || loaded.Tuning.Filesystem.MaxListItems != 10_000 || loaded.Tuning.Git.CommandOutputMaxBytes != 16<<20 {
		testingContext.Fatal(loaded.Tuning)
	}
}
func TestDocumentRejectsMalformed(testingContext *testing.T) {
	cases := []string{`{"version":1,"tuning":{"skills":{"unknown":1}}}`, `{"version":1,"unknown":1}`, `{"version":2}`, `{"version":1}{}`, `{"version":1,"provider":{"options":{"api_key":"x"}}}`}
	for _, value := range cases {
		if _, operationError := DecodeDocument([]byte(value)); operationError == nil {
			testingContext.Fatalf("accepted %s", value)
		}
	}
}
func TestDocumentRejectsInvalidTuning(testingContext *testing.T) {
	document := DefaultDocument(testingContext.TempDir())
	document.Tuning.Loops.DefaultInterval = duration("1s")
	document.Tuning.Loops.MinInterval = duration("2s")
	if operationError := ValidateDocument(document); operationError == nil {
		testingContext.Fatal("accepted loop ordering")
	}
	document = DefaultDocument(testingContext.TempDir())
	document.Tuning.Loops.MinInterval = duration("1s")
	document.Tuning.Loops.DefaultInterval = duration("1500ms")
	document.Tuning.Loops.MaxInterval = duration("2s")
	if operationError := ValidateDocument(document); operationError == nil {
		testingContext.Fatal("accepted sub-second loop interval")
	}
	document = DefaultDocument(testingContext.TempDir())
	document.Tuning.Hub.PendingClientLimit = 300
	if operationError := ValidateDocument(document); operationError == nil {
		testingContext.Fatal("accepted pending ordering")
	}
	document = DefaultDocument(testingContext.TempDir())
	document.Tuning.History.MinMaxBytes = document.Tuning.History.MaxMaxBytes + 1
	if operationError := ValidateDocument(document); operationError == nil {
		testingContext.Fatal("accepted history bytes")
	}
}
func TestMigrateV0PreservesDefaults(testingContext *testing.T) {
	document, operationError := DecodeDocument([]byte(`{"port":2500,"ensureAgent":false}`))
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if document.Network.Port != 2500 || document.Agent.Ensure || document.Agent.Port != DefaultAgentPort || document.Tuning.Skills.MaxFileBytes != 1<<20 || document.Tuning.HTTP.HealthProbeMaxBytes != 512 || document.Tuning.Filesystem.MaxListItems != 10_000 || document.Tuning.Git.CommandOutputMaxBytes != 16<<20 {
		testingContext.Fatal(document)
	}
	document, operationError = DecodeDocument([]byte(`{}`))
	if operationError != nil || !document.Agent.Ensure {
		testingContext.Fatal(operationError, document.Agent.Ensure)
	}
}

func TestLoadDocumentV0UsesProvidedHome(testingContext *testing.T) {
	providedHome := testingContext.TempDir()
	configurationPath := filepath.Join(providedHome, "legacy.json")
	if operationError := os.WriteFile(configurationPath, []byte(`{}`), 0600); operationError != nil {
		testingContext.Fatal(operationError)
	}
	document, operationError := LoadDocument(configurationPath, providedHome)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if document.Storage.DataDirectory != filepath.Join(providedHome, ".any-aicli-remote") {
		testingContext.Fatalf("data directory: %s", document.Storage.DataDirectory)
	}
	if document.Storage.RuntimeDirectory != filepath.Join(providedHome, ".any-aicli-remote", "run") {
		testingContext.Fatalf("runtime directory: %s", document.Storage.RuntimeDirectory)
	}
}

func TestHubDocumentIncludesNotificationAndReverseReadDefaults(testingContext *testing.T) {
	document := DefaultDocument(testingContext.TempDir())
	if document.Tuning.Hub.NotificationEnsure.Duration.String() != "3s" || document.Tuning.Hub.ReverseReadBytes != 2_000_000 || document.Tuning.Hub.AgentMaxMessageBytes != 64<<20 {
		testingContext.Fatalf("hub defaults = %#v", document.Tuning.Hub)
	}
	encoded, operationError := json.Marshal(document)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	decoded, operationError := DecodeDocument(encoded)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if decoded.Tuning.Hub.NotificationEnsure != document.Tuning.Hub.NotificationEnsure || decoded.Tuning.Hub.ReverseReadBytes != document.Tuning.Hub.ReverseReadBytes {
		testingContext.Fatalf("hub round trip = %#v", decoded.Tuning.Hub)
	}
}

func TestDecodeDocumentAddsAgentMessageLimitToExistingConfig(testingContext *testing.T) {
	home := testingContext.TempDir()
	document := DefaultDocument(home)
	document.Tuning.Hub.AgentMaxMessageBytes = 0
	encoded, operationError := json.Marshal(document)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}

	normalized, operationError := DecodeDocumentReader(bytes.NewReader(encoded), home)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	if normalized.Tuning.Hub.AgentMaxMessageBytes != 64<<20 {
		testingContext.Fatalf("agent max message bytes = %d", normalized.Tuning.Hub.AgentMaxMessageBytes)
	}
}

func TestDocumentRejectsBeforeHistoryBytesOutsideBounds(testContext *testing.T) {
	document := DefaultDocument(testContext.TempDir())
	document.Tuning.History.BeforeMaxBytes = document.Tuning.History.MaxMaxBytes + 1
	if operationError := ValidateDocument(document); operationError == nil {
		testContext.Fatal("expected invalid before history bytes")
	}
}

func TestDocumentRejectsInvalidAuthenticationCookieMaxAge(testingContext *testing.T) {
	document := DefaultDocument(testingContext.TempDir())
	document.Tuning.HTTP.AuthenticationCookieMaxAge = duration("1500ms")
	if operationError := ValidateDocument(document); operationError == nil {
		testingContext.Fatal("accepted subsecond cookie max age")
	}

	document = DefaultDocument(testingContext.TempDir())
	document.Tuning.HTTP.AuthenticationCookieMaxAge = Duration{Duration: time.Duration(math.MaxInt64)}
	if strconv.IntSize == 32 && ValidateDocument(document) == nil {
		testingContext.Fatal("accepted overflowing 32-bit cookie max age")
	}
}

func TestDocumentRejectsFilesystemSentinelOverflow(testContext *testing.T) {
	document := DefaultDocument(testContext.TempDir())
	document.Tuning.Filesystem.MaxReadBytes = math.MaxInt64
	if operationError := ValidateDocument(document); operationError == nil {
		testContext.Fatal("accepted filesystem read sentinel overflow")
	}
}

func TestRoomAndLoopTuningDefaultsRoundTripAndRelations(testContext *testing.T) {
	document := DefaultDocument(testContext.TempDir())
	room := document.Tuning.Room
	if room.MessageRuneLimit != 240 || room.SpeakerRuneLimit != 32 || room.KindRuneLimit != 12 || room.CompactionThreshold != 2000 || room.CompactionRetainMessages != 1000 || room.FeedDefaultLimit != 200 || room.FeedMaxLimit != 500 || room.MemberWindow.Duration != 15*time.Minute || room.ScannerInitialBytes != 64*1024 || room.ScannerMaxBytes != 4*1024*1024 || document.Tuning.Loops.LastErrorRunes != 200 {
		testContext.Fatalf("unexpected tuning: %#v %#v", room, document.Tuning.Loops)
	}
	encoded, operationError := json.Marshal(document)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	decoded, operationError := DecodeDocument(encoded)
	if operationError != nil {
		testContext.Fatal(operationError)
	}
	if decoded.Tuning.Room != room || decoded.Tuning.Loops.LastErrorRunes != 200 {
		testContext.Fatalf("round trip tuning = %#v %#v", decoded.Tuning.Room, decoded.Tuning.Loops)
	}
	for _, mutate := range []func(*Document){func(value *Document) {
		value.Tuning.Room.CompactionRetainMessages = value.Tuning.Room.CompactionThreshold + 1
	}, func(value *Document) { value.Tuning.Room.FeedDefaultLimit = value.Tuning.Room.FeedMaxLimit + 1 }, func(value *Document) { value.Tuning.Room.ScannerInitialBytes = value.Tuning.Room.ScannerMaxBytes + 1 }, func(value *Document) { value.Tuning.Loops.LastErrorRunes = 0 }} {
		invalid := DefaultDocument(testContext.TempDir())
		mutate(&invalid)
		if ValidateDocument(invalid) == nil {
			testContext.Fatal("accepted invalid tuning relation")
		}
	}
}

func TestHistoryAndVoiceTuningDefaultsRoundTripAndValidation(testContext *testing.T) {
	document := DefaultDocument(testContext.TempDir())
	if document.Tuning.History.ChatTextMaxRunes != 120000 || document.Tuning.History.MessageScanInitialBytes != 64*1024 || document.Tuning.Voice.TextMaxRunes != 15000 || document.Tuning.Voice.TruncatedTextRunes != 14990 {
		testContext.Fatalf("defaults=%#v %#v", document.Tuning.History, document.Tuning.Voice)
	}
	data, errorValue := json.Marshal(document)
	if errorValue != nil {
		testContext.Fatal(errorValue)
	}
	decoded, errorValue := decodeDocument(data, testContext.TempDir())
	if errorValue != nil || decoded.Tuning.Voice != document.Tuning.Voice || decoded.Tuning.History.ChatTextMaxRunes != document.Tuning.History.ChatTextMaxRunes {
		testContext.Fatalf("roundtrip=%#v err=%v", decoded, errorValue)
	}
	document.Tuning.History.MessageScanInitialBytes = document.Tuning.History.MessageScanMaxBytes + 1
	if ValidateDocument(document) == nil {
		testContext.Fatal("scanner relation accepted")
	}
	document = DefaultDocument(testContext.TempDir())
	document.Tuning.Voice.TruncatedTextRunes = document.Tuning.Voice.TextMaxRunes
	if ValidateDocument(document) == nil {
		testContext.Fatal("voice relation accepted")
	}
	document = DefaultDocument(testContext.TempDir())
	document.Tuning.Voice.SuccessBodyMaxBytes = math.MaxInt64
	if ValidateDocument(document) == nil {
		testContext.Fatal("voice sentinel accepted")
	}
}

func TestSkillsAndHTTPPolicyDefaultsRoundTripAndValidation(testingContext *testing.T) {
	document := NormalizeDocument(DefaultDocument(testingContext.TempDir()), testingContext.TempDir())
	if document.Tuning.Skills.MaxFileBytes != 1<<20 || document.Tuning.Skills.DescriptionMaxRunes != 240 || document.Tuning.Skills.MaxItems != 2000 || document.Tuning.HTTP.ErrorResponseMaxRunes != 300 || document.Tuning.HTTP.DeepHealthDetailMaxRunes != 200 || document.Tuning.HTTP.HealthProbeMaxBytes != 512 || document.Tuning.Filesystem.MaxListItems != 10_000 || document.Tuning.Git.CommandOutputMaxBytes != 16<<20 {
		testingContext.Fatalf("unexpected defaults: %#v %#v", document.Tuning.Skills, document.Tuning.HTTP)
	}
	encoded, operationError := json.Marshal(document)
	if operationError != nil {
		testingContext.Fatal(operationError)
	}
	decoded, operationError := decodeDocument(encoded, testingContext.TempDir())
	if operationError != nil || decoded.Tuning.Skills != document.Tuning.Skills || decoded.Tuning.HTTP.ErrorResponseMaxRunes != 300 {
		testingContext.Fatalf("round trip = %#v, %v", decoded, operationError)
	}
	document.Tuning.Skills.MaxItems = 0
	if ValidateDocument(document) == nil {
		testingContext.Fatal("accepted zero skills maximum")
	}
	document = DefaultDocument(testingContext.TempDir())
	document.Tuning.Skills.MaxFileBytes = math.MaxInt64
	if ValidateDocument(document) == nil {
		testingContext.Fatal("accepted skills sentinel overflow")
	}
	document = DefaultDocument(testingContext.TempDir())
	document.Tuning.HTTP.HealthProbeMaxBytes = 0
	if ValidateDocument(document) == nil {
		testingContext.Fatal("accepted zero health probe maximum")
	}
}

func TestDocumentReaderRejectsOversize(testingContext *testing.T) {
	home := testingContext.TempDir()
	oversize := bytes.Repeat([]byte("x"), int(BootstrapDocumentMaxBytes+1))
	if _, errorValue := DecodeDocument(oversize); !errors.Is(errorValue, DocumentTooLargeError) {
		testingContext.Fatalf("DecodeDocument error = %v", errorValue)
	}
	path := filepath.Join(home, "oversize.json")
	if errorValue := os.WriteFile(path, oversize, 0600); errorValue != nil {
		testingContext.Fatal(errorValue)
	}
	if _, errorValue := LoadDocument(path, home); !errors.Is(errorValue, DocumentTooLargeError) {
		testingContext.Fatalf("LoadDocument error = %v", errorValue)
	}
}

func TestDocumentRejectsInvalidPublicWorkspaceBounds(testContext *testing.T) {
	document := DefaultDocument(testContext.TempDir())
	document.Tuning.Filesystem.MaxListItems = 0
	if operationError := ValidateDocument(document); operationError == nil {
		testContext.Fatal("accepted zero filesystem list limit")
	}
	document = DefaultDocument(testContext.TempDir())
	document.Tuning.Filesystem.MaxListItems = int(^uint(0) >> 1)
	if operationError := ValidateDocument(document); operationError == nil {
		testContext.Fatal("accepted overflow filesystem list limit")
	}
	document = DefaultDocument(testContext.TempDir())
	document.Tuning.Git.CommandOutputMaxBytes = 0
	if operationError := ValidateDocument(document); operationError == nil {
		testContext.Fatal("accepted zero git output limit")
	}
}
