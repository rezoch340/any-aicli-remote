// Configuration document codec: bounded decoding, load, atomic save, and
// migration of superseded on-disk versions.

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/rezoch340/any-aicli-remote/backend/internal/atomicfile"
)

const (
	DocumentVersion = 1
	// BootstrapDocumentMaxBytes bounds configuration bootstrap input before runtime tuning is available.
	BootstrapDocumentMaxBytes int64 = 4 << 20
)

var DocumentTooLargeError = errors.New("configuration document exceeds bootstrap size limit")

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
