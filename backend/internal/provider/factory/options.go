package factory

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/rezoch340/any-aicli-remote/backend/internal/compat"
)

// OptionParser owns command-line and environment parsing that is specific to
// the currently supported provider. Config stores only the resulting opaque
// option map.
type OptionParser struct {
	sessionsDirectory string
	alwaysApprove     bool
	leader            bool
	unknown           map[string]string
	parseError        error
}

func NewOptionParserWithValues(values map[string]string) *OptionParser {
	parser := &OptionParser{unknown: map[string]string{}}
	for optionName, optionValue := range values {
		parser.unknown[optionName] = optionValue
	}
	parser.sessionsDirectory = values[SessionsDirectoryOption]
	if value, present := values[AlwaysApproveOption]; present {
		parser.alwaysApprove, parser.parseError = strconv.ParseBool(value)
	}
	if value, present := values[LeaderOption]; present {
		parsedLeader, leaderError := strconv.ParseBool(value)
		if parser.parseError == nil {
			parser.leader, parser.parseError = parsedLeader, leaderError
		}
	}
	return parser
}

func NewOptionParser() *OptionParser {
	parser := &OptionParser{unknown: map[string]string{}}
	_ = parser.ApplyEnvironment()
	return parser
}

func (parser *OptionParser) ApplyEnvironment() error {
	if parser.parseError != nil {
		return fmt.Errorf("invalid provider option: %w", parser.parseError)
	}
	parser.sessionsDirectory = compat.Environment("ANY_AI_CLI_REMOTE_PROVIDER_SESSIONS_DIR", compat.Environment("ANY_AI_CLI_REMOTE_GROK_SESSIONS_DIR", parser.sessionsDirectory))
	if operationError := applyBooleanEnvironment("ANY_AI_CLI_REMOTE_PROVIDER_ALWAYS_APPROVE", "ANY_AI_CLI_REMOTE_GROK_ALWAYS_APPROVE", &parser.alwaysApprove); operationError != nil {
		return operationError
	}
	if operationError := applyBooleanEnvironment("ANY_AI_CLI_REMOTE_PROVIDER_LEADER", "ANY_AI_CLI_REMOTE_GROK_LEADER", &parser.leader); operationError != nil {
		return operationError
	}
	return nil
}

func (parser *OptionParser) Validate() error {
	if parser.parseError != nil {
		return fmt.Errorf("invalid provider option: %w", parser.parseError)
	}
	return nil
}

func applyBooleanEnvironment(currentKey string, legacyKey string, target *bool) error {
	value := compat.Environment(currentKey, compat.Environment(legacyKey, ""))
	if value == "" {
		return nil
	}
	parsed, operationError := strconv.ParseBool(value)
	if operationError != nil {
		switch strings.ToLower(value) {
		case "yes", "on":
			parsed = true
		case "no", "off":
			parsed = false
		default:
			return fmt.Errorf("%s must be a boolean", currentKey)
		}
	}
	*target = parsed
	return nil
}

func (parser *OptionParser) BindFlags(flagSet *flag.FlagSet, executablePath *string) {
	flagSet.StringVar(&parser.sessionsDirectory, "provider-sessions-dir", parser.sessionsDirectory, "provider sessions directory")
	flagSet.BoolVar(&parser.alwaysApprove, "provider-always-approve", parser.alwaysApprove, "launch the provider in automatic approval mode")
	flagSet.BoolVar(&parser.leader, "provider-leader", parser.leader, "launch the provider in leader mode")

	// Provider-specific names remain read-only compatibility aliases. New
	// documentation and writes use the generic flags above.
	flagSet.StringVar(executablePath, "grok", *executablePath, "deprecated alias for --provider-path")
	flagSet.StringVar(&parser.sessionsDirectory, "grok-sessions-dir", parser.sessionsDirectory, "deprecated alias for --provider-sessions-dir")
	flagSet.BoolVar(&parser.alwaysApprove, "grok-always-approve", parser.alwaysApprove, "deprecated alias for --provider-always-approve")
	flagSet.BoolVar(&parser.leader, "grok-leader", parser.leader, "deprecated alias for --provider-leader")
}

func (parser *OptionParser) Values() map[string]string {
	options := map[string]string{}
	for optionName, optionValue := range parser.unknown {
		options[optionName] = optionValue
	}
	options[AlwaysApproveOption] = strconv.FormatBool(parser.alwaysApprove)
	options[LeaderOption] = strconv.FormatBool(parser.leader)
	if sessionsDirectory := strings.TrimSpace(parser.sessionsDirectory); sessionsDirectory != "" {
		options[SessionsDirectoryOption] = sessionsDirectory
	}
	return options
}
