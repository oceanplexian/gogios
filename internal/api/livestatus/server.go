package livestatus

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/oceanplexian/gogios/internal/api"
	"github.com/oceanplexian/gogios/internal/logging"
)

// Server is the Livestatus query server. It listens on a Unix domain socket
// and/or a TCP address and handles LQL queries.
type Server struct {
	socketPath    string
	tcpAddr       string
	provider      *api.StateProvider
	cmdSink       api.CommandSink
	batchCmdSink  api.BatchCommandSink
	listeners     []net.Listener
	wg            sync.WaitGroup
	quit          chan struct{}
}

// New creates a new Livestatus server.
func New(socketPath, tcpAddr string) *Server {
	return &Server{
		socketPath: socketPath,
		tcpAddr:    tcpAddr,
		quit:       make(chan struct{}),
	}
}

// SetBatchCommandSink sets an optional batch command sink for high-throughput
// command processing. When set, bulk commands on a single connection are
// dispatched in one batch (single lock acquisition) instead of individually.
func (s *Server) SetBatchCommandSink(sink api.BatchCommandSink) {
	s.batchCmdSink = sink
}

// Start begins listening for connections.
func (s *Server) Start(provider *api.StateProvider, cmdSink api.CommandSink) error {
	s.provider = provider
	s.cmdSink = cmdSink

	if s.socketPath != "" {
		// Remove stale socket
		os.Remove(s.socketPath)
		ln, err := net.Listen("unix", s.socketPath)
		if err != nil {
			return fmt.Errorf("unix listen %s: %w", s.socketPath, err)
		}
		os.Chmod(s.socketPath, 0660)
		s.listeners = append(s.listeners, ln)
		s.wg.Add(1)
		go s.acceptLoop(ln)
	}

	if s.tcpAddr != "" {
		ln, err := net.Listen("tcp", s.tcpAddr)
		if err != nil {
			return fmt.Errorf("tcp listen %s: %w", s.tcpAddr, err)
		}
		s.listeners = append(s.listeners, ln)
		s.wg.Add(1)
		go s.acceptLoop(ln)
	}

	return nil
}

// Stop shuts down the server.
func (s *Server) Stop() {
	close(s.quit)
	for _, ln := range s.listeners {
		ln.Close()
	}
	s.wg.Wait()
	if s.socketPath != "" {
		os.Remove(s.socketPath)
	}
}

func (s *Server) acceptLoop(ln net.Listener) {
	defer s.wg.Done()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				continue
			}
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Collect commands for batch dispatch. When a connection sends only
	// commands (typical for Thruk bulk operations), we read them all first
	// and dispatch in a single batch to avoid per-command lock overhead.
	var pendingCmds []api.CommandEntry

	reader := bufio.NewReader(conn)
	for {
		request, err := readRequest(reader)
		if err != nil {
			if err != io.EOF {
				if s.provider.Logger != nil {
					s.provider.Logger.Log("Livestatus read error: %v", err)
				}
			}
			// Connection done — flush any pending commands.
			s.flushCommands(pendingCmds, conn)
			return
		}
		if strings.TrimSpace(request) == "" {
			s.flushCommands(pendingCmds, conn)
			return
		}

		// Handle COMMAND before parsing as query — COMMANDs have no headers
		firstLine := strings.SplitN(strings.TrimSpace(request), "\n", 2)[0]
		if strings.HasPrefix(firstLine, "COMMAND ") {
			if s.provider.Logger != nil {
				s.provider.Logger.LogVerbose(logging.VerboseLivestatus, "LIVESTATUS: %s from %s", firstLine, conn.RemoteAddr())
			}
			// Queue the command for batch dispatch instead of executing immediately.
			entry := parseCommandEntry(firstLine)
			if entry != nil {
				pendingCmds = append(pendingCmds, *entry)
			}
			// Per spec: commands are fire-and-forget, no response.
			continue
		}

		// About to handle a query — flush any accumulated commands first.
		s.flushCommands(pendingCmds, conn)
		pendingCmds = nil

		q, err := ParseQuery(request)
		if err != nil {
			writeError(conn, q, fmt.Sprintf("Invalid query: %v", err))
			if q == nil || !q.KeepAlive {
				return
			}
			continue
		}

		if s.provider.Logger != nil {
			s.provider.Logger.LogVerbose(logging.VerboseLivestatus, "LIVESTATUS: GET %s (Columns: %d) (Filters: %d) from %s",
				q.Table, len(q.Columns), len(q.Filters), conn.RemoteAddr())
		}

		response := ExecuteQuery(q, s.provider)
		conn.Write([]byte(response))

		if !q.KeepAlive {
			return
		}
	}
}

// flushCommands dispatches accumulated commands. Uses batch dispatch when
// available (single lock), falls back to per-command dispatch otherwise.
func (s *Server) flushCommands(cmds []api.CommandEntry, conn net.Conn) {
	if len(cmds) == 0 {
		return
	}
	if s.batchCmdSink != nil {
		if s.provider.Logger != nil && len(cmds) > 1 {
			s.provider.Logger.LogVerbose(logging.VerboseLivestatus,
				"LIVESTATUS: batch-dispatching %d commands from %s", len(cmds), conn.RemoteAddr())
		}
		s.batchCmdSink(cmds)
		return
	}
	// Fallback: dispatch one at a time
	for _, c := range cmds {
		s.cmdSink(c.Name, c.Args)
	}
}

// parseCommandEntry extracts the command name and args from a COMMAND line
// without invoking the sink. Returns nil for unparseable input.
func parseCommandEntry(request string) *api.CommandEntry {
	line := strings.TrimPrefix(request, "COMMAND ")
	line = strings.TrimSpace(line)

	// Skip optional timestamp
	if strings.HasPrefix(line, "[") {
		idx := strings.Index(line, "]")
		if idx >= 0 {
			line = strings.TrimSpace(line[idx+1:])
		}
	}

	parts := strings.SplitN(line, ";", 2)
	name := parts[0]
	if name == "" {
		return nil
	}
	var args []string
	if len(parts) > 1 {
		args = strings.Split(parts[1], ";")
	}
	return &api.CommandEntry{Name: name, Args: args}
}

func readRequest(reader *bufio.Reader) (string, error) {
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if len(lines) > 0 && err == io.EOF {
				// Treat EOF after content as end of request
				lines = append(lines, line)
				return strings.Join(lines, "\n"), nil
			}
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

func writeError(conn net.Conn, q *Query, msg string) {
	if q != nil && q.ResponseHeader == "fixed16" {
		header := fmt.Sprintf("%3d %11d\n", 400, len(msg)+1)
		conn.Write([]byte(header))
	}
	conn.Write([]byte(msg + "\n"))
}
