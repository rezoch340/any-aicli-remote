package server

import "path/filepath"

const (
	logsDirectoryName     = "logs"
	agentStateFileName    = "agent-state.json"
	loopsStoreFileName    = "loops.json"
	runtimeConfigFileName = "runtime-config.json"
)

func logsDirectory(dataDirectory string) string {
	return filepath.Join(dataDirectory, logsDirectoryName)
}
func agentStatePath(dataDirectory string) string {
	return filepath.Join(dataDirectory, agentStateFileName)
}
func loopsStorePath(dataDirectory string) string {
	return filepath.Join(dataDirectory, loopsStoreFileName)
}
func runtimeConfigPath(dataDirectory string) string {
	return filepath.Join(dataDirectory, runtimeConfigFileName)
}
