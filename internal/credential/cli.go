package credential

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	invalidArgumentsMessage = "VoxInk credential: invalid arguments"
	unsupportedMessage      = "VoxInk credential: unsupported"
	storageFailedMessage    = "VoxInk credential: storage failed"
	invalidInputMessage     = "VoxInk credential: invalid input"
)

// ValueReader obtains one credential value without putting it in arguments.
type ValueReader func() ([]byte, error)

// Execute runs the fixed credential CLI and returns its process exit code.
func Execute(args []string, stdout, stderr io.Writer, store Store, readValue ValueReader) int {
	command, name, jsonOutput, ok := parseArguments(args)
	if !ok {
		fmt.Fprintln(stderr, invalidArgumentsMessage)
		return 2
	}
	if store == nil {
		fmt.Fprintln(stderr, unsupportedMessage)
		return 1
	}

	switch command {
	case "set":
		if readValue == nil {
			fmt.Fprintln(stderr, invalidInputMessage)
			return 1
		}
		value, err := readValue()
		if err != nil || len(value) == 0 || len(value) > MaximumValueBytes {
			clear(value)
			fmt.Fprintln(stderr, invalidInputMessage)
			return 1
		}
		defer clear(value)
		if err := store.Write(name, value); err != nil {
			fmt.Fprintln(stderr, storageFailedMessage)
			return 1
		}
		fmt.Fprintln(stdout, "VoxInk credential: configured")
		return 0
	case "delete":
		if err := store.Delete(name); err != nil && !errors.Is(err, ErrNotFound) {
			fmt.Fprintln(stderr, storageFailedMessage)
			return 1
		}
		fmt.Fprintln(stdout, "VoxInk credential: deleted")
		return 0
	case "list":
		statuses, err := list(store)
		if err != nil {
			fmt.Fprintln(stderr, storageFailedMessage)
			return 1
		}
		if jsonOutput {
			if err := json.NewEncoder(stdout).Encode(statuses); err != nil {
				fmt.Fprintln(stderr, storageFailedMessage)
				return 1
			}
			return 0
		}
		for _, status := range statuses {
			fmt.Fprintf(stdout, "%s configured=%t\n", status.Name, status.Configured)
		}
		return 0
	default:
		panic("validated credential command was not handled")
	}
}

func parseArguments(args []string) (command string, name Name, jsonOutput bool, ok bool) {
	if len(args) == 2 && args[0] == "list" && args[1] == "--json" {
		return "list", "", true, true
	}
	if len(args) == 1 && args[0] == "list" {
		return "list", "", false, true
	}
	if len(args) != 2 || (args[0] != "set" && args[0] != "delete") {
		return "", "", false, false
	}
	name, ok = ParseName(args[1])
	return args[0], name, false, ok
}

func list(store Store) ([]Status, error) {
	statuses := make([]Status, 0, len(allNames))
	for _, name := range allNames {
		value, err := store.Read(name)
		clear(value)
		switch {
		case err == nil:
			statuses = append(statuses, Status{Name: name, Configured: true})
		case errors.Is(err, ErrNotFound):
			statuses = append(statuses, Status{Name: name})
		default:
			return nil, ErrStorage
		}
	}
	return statuses, nil
}
