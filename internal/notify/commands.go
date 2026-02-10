package notify

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// CommandExecutor runs notification commands.
type CommandExecutor struct {
	Timeout time.Duration
}

// NewCommandExecutor creates a new executor with the given timeout.
func NewCommandExecutor(timeout time.Duration) *CommandExecutor {
	return &CommandExecutor{Timeout: timeout}
}

// Execute runs a notification command asynchronously and returns immediately.
// The command is run via /bin/sh -c.
func (e *CommandExecutor) Execute(cmdLine string) {
	go e.run(cmdLine)
}

// ExecuteSync runs a notification command synchronously. Used for testing.
func (e *CommandExecutor) ExecuteSync(cmdLine string) error {
	return e.run(cmdLine)
}

func (e *CommandExecutor) run(cmdLine string) error {
	timeout := e.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", cmdLine)
	return cmd.Run()
}

// ExpandMacros does simple macro substitution in a command line.
// The macros map provides $MACRO$ -> value mappings (without the $ delimiters).
func ExpandMacros(cmdLine string, macros map[string]string) string {
	result := cmdLine
	for k, v := range macros {
		result = strings.ReplaceAll(result, "$"+k+"$", v)
	}
	return result
}
