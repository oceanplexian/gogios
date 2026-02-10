package livestatus

import (
	"strings"

	"github.com/oceanplexian/gogios/internal/api"
)

// handleCommand processes a COMMAND request.
// Format: COMMAND [timestamp] command_name;arg1;arg2;...
func handleCommand(request string, sink api.CommandSink) {
	if sink == nil {
		return
	}
	// Strip "COMMAND " prefix
	line := strings.TrimPrefix(request, "COMMAND ")
	line = strings.TrimSpace(line)

	// Skip optional timestamp in brackets
	if strings.HasPrefix(line, "[") {
		idx := strings.Index(line, "]")
		if idx >= 0 {
			line = strings.TrimSpace(line[idx+1:])
		}
	}

	// Split command name from arguments
	parts := strings.SplitN(line, ";", 2)
	name := parts[0]
	var args []string
	if len(parts) > 1 {
		args = strings.Split(parts[1], ";")
	}

	sink(name, args)
}
