package settings

import (
	"encoding/json"
	"fmt"
	"io"
)

const (
	invalidArgumentsMessage = "VoxInk settings: invalid arguments"
	invalidValueMessage     = "VoxInk settings: invalid value"
	storageFailedMessage    = "VoxInk settings: storage failed"
)

// Execute runs the fixed non-sensitive settings CLI and returns its process exit code.
func Execute(args []string, stdout, stderr io.Writer, repository Repository, getenv func(string) string) int {
	command, key, value, jsonOutput, ok := parseArguments(args)
	if !ok {
		fmt.Fprintln(stderr, invalidArgumentsMessage)
		return 2
	}
	if repository == nil {
		fmt.Fprintln(stderr, storageFailedMessage)
		return 1
	}
	document, err := repository.Load()
	if err != nil {
		fmt.Fprintln(stderr, storageFailedMessage)
		return 1
	}

	switch command {
	case "set":
		if err := Set(&document, key, value); err != nil {
			fmt.Fprintln(stderr, invalidValueMessage)
			return 1
		}
		if err := repository.Save(document); err != nil {
			fmt.Fprintln(stderr, storageFailedMessage)
			return 1
		}
		fmt.Fprintln(stdout, "VoxInk settings: configured")
		return 0
	case "delete":
		Delete(&document, key)
		if err := repository.Save(document); err != nil {
			fmt.Fprintln(stderr, storageFailedMessage)
			return 1
		}
		fmt.Fprintln(stdout, "VoxInk settings: deleted")
		return 0
	case "list":
		effective, err := Resolve(document, getenv)
		if err != nil {
			fmt.Fprintln(stderr, storageFailedMessage)
			return 1
		}
		values := listValues(effective)
		if jsonOutput {
			if err := json.NewEncoder(stdout).Encode(values); err != nil {
				fmt.Fprintln(stderr, storageFailedMessage)
				return 1
			}
			return 0
		}
		for _, key := range allKeys {
			fmt.Fprintf(stdout, "%s=%v\n", key, values[string(key)])
		}
		return 0
	default:
		panic("validated settings command was not handled")
	}
}

func parseArguments(args []string) (command string, key Key, value string, jsonOutput bool, ok bool) {
	if len(args) == 1 && args[0] == "list" {
		return "list", "", "", false, true
	}
	if len(args) == 2 && args[0] == "list" && args[1] == "--json" {
		return "list", "", "", true, true
	}
	if len(args) == 3 && args[0] == "set" {
		key, ok = ParseKey(args[1])
		return "set", key, args[2], false, ok
	}
	if len(args) == 2 && args[0] == "delete" {
		key, ok = ParseKey(args[1])
		return "delete", key, "", false, ok
	}
	return "", "", "", false, false
}

func listValues(effective Effective) map[string]any {
	return map[string]any{
		string(HotkeyKey):             effective.Hotkey.String(),
		string(VolcEndpointKey):       effective.VolcEndpoint,
		string(VolcReadLimitBytesKey): effective.VolcReadLimitBytes,
		string(MiMoEndpointKey):       effective.MiMoEndpoint,
		string(MiMoAuthModeKey):       string(effective.MiMoAuthMode),
	}
}
