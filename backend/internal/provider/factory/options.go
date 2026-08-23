package factory

import (
	"flag"
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
}

func NewOptionParser() *OptionParser {
	return &OptionParser{
		sessionsDirectory: compat.Environment("ANY_AI_CLI_REMOTE_PROVIDER_SESSIONS_DIR", compat.Environment("ANY_AI_CLI_REMOTE_GROK_SESSIONS_DIR", "")),
		alwaysApprove: compat.BooleanEnvironment(
			"ANY_AI_CLI_REMOTE_PROVIDER_ALWAYS_APPROVE",
			compat.BooleanEnvironment("ANY_AI_CLI_REMOTE_GROK_ALWAYS_APPROVE", false),
		),
		leader: compat.BooleanEnvironment(
			"ANY_AI_CLI_REMOTE_PROVIDER_LEADER",
			compat.BooleanEnvironment("ANY_AI_CLI_REMOTE_GROK_LEADER", false),
		),
	}
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
	options := map[string]string{
		AlwaysApproveOption: strconv.FormatBool(parser.alwaysApprove),
		LeaderOption:        strconv.FormatBool(parser.leader),
	}
	if sessionsDirectory := strings.TrimSpace(parser.sessionsDirectory); sessionsDirectory != "" {
		options[SessionsDirectoryOption] = sessionsDirectory
	}
	return options
}
