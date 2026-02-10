package nrdp

import (
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/oceanplexian/gogios/internal/logging"
	"github.com/oceanplexian/gogios/internal/objects"

	"golang.org/x/crypto/bcrypt"
)

// Config holds the NRDP server configuration.
type Config struct {
	Listen         string // e.g. ":5668"
	Path           string // URL path, e.g. "/nrdp/"
	TokenHash      string // bcrypt hash of accepted token
	DynamicEnabled bool   // auto-register unknown hosts/services
	DynamicTTL     time.Duration
	DynamicPrune   time.Duration
	SSLCert        string
	SSLKey         string
}

// Server is the NRDP HTTP relay endpoint.
type Server struct {
	cfg      Config
	store    *objects.ObjectStore
	resultCh chan<- *objects.CheckResult
	logger   *logging.Logger
	tracker  *DynamicTracker
	server   *http.Server
}

// New creates a new NRDP server.
func New(cfg Config, store *objects.ObjectStore, resultCh chan<- *objects.CheckResult, logger *logging.Logger) *Server {
	s := &Server{
		cfg:      cfg,
		store:    store,
		resultCh: resultCh,
		logger:   logger,
	}
	if cfg.DynamicEnabled {
		s.tracker = NewDynamicTracker(store, cfg.DynamicTTL, cfg.DynamicPrune)
		s.tracker.SetLogger(func(format string, args ...interface{}) {
			logger.Log(format, args...)
		})
	}
	return s
}

// Start begins listening for NRDP requests.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	path := s.cfg.Path
	if path == "" {
		path = "/nrdp/"
	}
	mux.HandleFunc(path, s.handleNRDP)

	s.server = &http.Server{
		Addr:         s.cfg.Listen,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if s.tracker != nil {
		s.tracker.StartPruner()
	}

	ln, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		return fmt.Errorf("nrdp: listen %s: %w", s.cfg.Listen, err)
	}

	go func() {
		var serveErr error
		if s.cfg.SSLCert != "" && s.cfg.SSLKey != "" {
			serveErr = s.server.ServeTLS(ln, s.cfg.SSLCert, s.cfg.SSLKey)
		} else {
			serveErr = s.server.Serve(ln)
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			s.logger.Log("NRDP server error: %v", serveErr)
		}
	}()
	return nil
}

// Stop gracefully shuts down the NRDP server.
func (s *Server) Stop() {
	if s.tracker != nil {
		s.tracker.Stop()
	}
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}
}

// handleNRDP is the main request handler for POST /nrdp/.
func (s *Server) handleNRDP(w http.ResponseWriter, r *http.Request) {
	reqID := GenerateRequestID()

	// Method check
	if r.Method != http.MethodPost {
		body, ct := FormatResponse(FormatRawJSON, reqID, 405, "Method Not Allowed")
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(405)
		w.Write(body)
		return
	}

	// Authentication
	if !s.authenticate(r) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(401)
		w.Write([]byte("authorization failed\n"))
		return
	}

	// Read body for raw content types
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, FormatRawJSON, reqID, 500, "failed to read request body")
		return
	}
	defer r.Body.Close()

	// Parse form data if applicable
	r.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	r.ParseForm()

	// Detect format
	format := DetectFormat(r.Header.Get("Content-Type"), r.Form)
	if format == FormatUnknown {
		s.writeError(w, FormatRawJSON, reqID, 500, "unsupported content type")
		return
	}

	// Parse payload
	results, err := ParsePayload(format, bodyBytes, r.Form)
	if err != nil {
		s.writeError(w, format, reqID, 500, fmt.Sprintf("payload decode failure: %v", err))
		return
	}

	// Process results
	source := BuildSource(format, r.RemoteAddr)
	processed := 0

	for _, result := range results {
		if result.Hostname == "" {
			continue
		}

		result.Source = source

		// Dynamic registration if enabled
		if s.tracker != nil && s.cfg.DynamicEnabled {
			s.store.Mu.Lock()
			if result.Servicename != "" {
				s.tracker.EnsureService(result.Hostname, result.Servicename)
			} else {
				s.tracker.EnsureHost(result.Hostname)
			}
			s.store.Mu.Unlock()

			s.tracker.Touch(result.Hostname, result.Servicename)
		}

		// Build check result and inject into pipeline
		now := time.Now()
		cr := &objects.CheckResult{
			HostName:           result.Hostname,
			ServiceDescription: result.Servicename,
			CheckType:          objects.CheckTypePassive,
			ReturnCode:         result.Status,
			Output:             result.Output,
			StartTime:          result.Timestamp,
			FinishTime:         now,
			ExitedOK:           true,
		}

		select {
		case s.resultCh <- cr:
			processed++
		default:
			s.logger.Log("NRDP [%s] result channel full, dropping result for %s/%s",
				reqID, result.Hostname, result.Servicename)
		}
	}

	msg := fmt.Sprintf("Processing %d Results", processed)
	s.logger.Log("NRDP [%s] %s from %s (%s)", reqID, msg, r.RemoteAddr, format)

	body, ct := FormatResponse(format, reqID, 200, msg)
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(200)
	w.Write(body)
}

// authenticate checks the request token against the configured bcrypt hash.
// Localhost requests bypass authentication.
func (s *Server) authenticate(r *http.Request) bool {
	// Localhost bypass
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host == "127.0.0.1" || host == "::1" {
		return true
	}

	// Both token hash and token must be non-empty
	if s.cfg.TokenHash == "" {
		return false
	}

	token := r.FormValue("token")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		return false
	}

	err = bcrypt.CompareHashAndPassword([]byte(s.cfg.TokenHash), []byte(token))
	if err != nil {
		return false
	}

	// Constant-time success (bcrypt already does this, but belt & suspenders)
	_ = subtle.ConstantTimeEq(1, 1)
	return true
}

// writeError sends an error response in the appropriate format.
func (s *Server) writeError(w http.ResponseWriter, format, reqID string, status int, message string) {
	body, ct := FormatResponse(format, reqID, status, message)
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(status)
	w.Write(body)
}
