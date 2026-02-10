package api

import (
	"github.com/oceanplexian/gogios/internal/downtime"
	"github.com/oceanplexian/gogios/internal/logging"
	"github.com/oceanplexian/gogios/internal/objects"
)

// StateProvider gives the livestatus API access to all runtime state.
type StateProvider struct {
	Store     *objects.ObjectStore
	Global    *objects.GlobalState
	Comments  *downtime.CommentManager
	Downtimes *downtime.DowntimeManager
	Logger    *logging.Logger
	LogFile   string
}

// CommandSink is a callback for executing external commands from the API.
type CommandSink func(name string, args []string)
