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
	socketPath string
	tcpAddr    string
	provider   *api.StateProvider
	cmdSink    api.CommandSink
	listeners  []net.Listener
	wg         sync.WaitGroup
	quit       chan struct{}
}

// New creates a new Livestatus server.
func New(socketPath, tcpAddr string) *Server {
	return &Server{
		socketPath: socketPath,
		tcpAddr:    tcpAddr,
		quit:       make(chan struct{}),
	}
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

	reader := bufio.NewReader(conn)
	for {
		request, err := readRequest(reader)
		if err != nil {
			if err != io.EOF {
				if s.provider.Logger != nil {
					s.provider.Logger.Log("Livestatus read error: %v", err)
				}
			}
			return
		}
		if strings.TrimSpace(request) == "" {
			return
		}

		// Handle COMMAND before parsing as query — COMMANDs have no headers
		firstLine := strings.SplitN(strings.TrimSpace(request), "\n", 2)[0]
		if strings.HasPrefix(firstLine, "COMMAND ") {
			if s.provider.Logger != nil {
				s.provider.Logger.LogVerbose(logging.VerboseLivestatus, "LIVESTATUS: %s from %s", firstLine, conn.RemoteAddr())
			}
			handleCommand(firstLine, s.cmdSink)
			// Per spec: commands are fire-and-forget, no response.
			// Continue the loop to process additional commands on the
			// same connection (Thruk sends bulk commands this way).
			continue
		}

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
