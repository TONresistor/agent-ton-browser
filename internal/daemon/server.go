package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/TONresistor/agent-tonbrowser/internal/cdp"
	"github.com/TONresistor/agent-tonbrowser/internal/config"
)

// DefaultIdleTimeout is how long the daemon waits with no requests before shutting down.
const DefaultIdleTimeout = 5 * time.Minute

// Server holds the persistent CDP session and serves HTTP on a Unix socket.
type Server struct {
	session     *cdp.Session
	port        int
	currentTab  int
	mu          sync.Mutex
	lastActive  time.Time
	idleTimeout time.Duration
	httpSrv     *http.Server
}

// NewServer creates a new Server with the given idle timeout.
// If idleTimeout <= 0, DefaultIdleTimeout is used.
func NewServer(idleTimeout time.Duration) *Server {
	if idleTimeout <= 0 {
		idleTimeout = DefaultIdleTimeout
	}
	return &Server{
		currentTab:  -1,
		lastActive:  time.Now(),
		idleTimeout: idleTimeout,
	}
}

// Run starts the daemon server. Blocks until shutdown.
// It writes a PID file, opens a Unix socket, registers HTTP routes,
// starts an idle timer goroutine, and serves until shutdown or idle timeout.
func (s *Server) Run() error {
	if err := config.EnsureDirs(); err != nil {
		return fmt.Errorf("ensure dirs: %w", err)
	}

	pidPath, err := PIDPath()
	if err != nil {
		return fmt.Errorf("pid path: %w", err)
	}
	sockPath, err := SocketPath()
	if err != nil {
		return fmt.Errorf("socket path: %w", err)
	}

	// Check if daemon already running via PID file
	if data, err := os.ReadFile(pidPath); err == nil {
		var existingPID int
		if n, _ := fmt.Sscanf(string(data), "%d", &existingPID); n == 1 {
			if proc, err := os.FindProcess(existingPID); err == nil {
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					return fmt.Errorf("daemon already running with pid %d", existingPID)
				}
			}
		}
	}

	// Remove stale socket from a previous run
	os.Remove(sockPath)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", sockPath, err)
	}
	defer os.Remove(sockPath)
	defer ln.Close()

	// Write PID file after successful listen, with secure permissions
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0o600); err != nil {
		return fmt.Errorf("write pid: %w", err)
	}
	defer os.Remove(pidPath)

	mux := http.NewServeMux()
	s.registerRoutes(mux)
	s.httpSrv = &http.Server{Handler: mux}

	// Idle shutdown goroutine — checks every 30s
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.mu.Lock()
			idle := time.Since(s.lastActive)
			s.mu.Unlock()
			if idle > s.idleTimeout {
				log.Printf("daemon: idle timeout after %v, shutting down", idle)
				s.httpSrv.Close()
				return
			}
		}
	}()

	log.Printf("daemon: listening on %s (pid %d)", sockPath, os.Getpid())
	err = s.httpSrv.Serve(ln)
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// touch updates the last-active timestamp. Must be called with s.mu held.
func (s *Server) touch() {
	s.lastActive = time.Now()
}

// ensureSession returns an error if no session is connected.
// Must be called with s.mu held.
func (s *Server) ensureSession() error {
	if s.session == nil {
		return fmt.Errorf("not connected — call POST /connect first")
	}
	return nil
}

// maybeSwitch reconnects the session if the requested tab differs from currentTab.
// Must be called with s.mu held.
func (s *Server) maybeSwitch(tab int) error {
	if tab == s.currentTab {
		return nil
	}
	// Use a long-lived context — the new session must survive beyond this call.
	ctx := context.Background()

	// Clear session reference before disconnect to prevent stale pointer on failure.
	old := s.session
	s.session = nil
	old.Disconnect()

	var sess *cdp.Session
	var err error
	if tab >= 0 {
		sess, err = cdp.ConnectToTarget(ctx, s.port, tab)
	} else {
		sess, err = cdp.Connect(ctx, s.port)
	}
	if err != nil {
		return fmt.Errorf("switch to tab %d: %w", tab, err)
	}
	s.session = sess
	s.currentTab = tab
	return nil
}

// writeJSON encodes a standard Response to the response writer.
func writeJSON(w http.ResponseWriter, ok bool, data any, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	resp := Response{OK: ok}
	if errMsg != "" {
		resp.Error = errMsg
	}
	if data != nil {
		b, _ := json.Marshal(data)
		resp.Data = json.RawMessage(b)
	}
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// postHandler wraps a handler to enforce POST method and limit request body.
func postHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = io.NopCloser(io.LimitReader(r.Body, 1<<20))
		h(w, r)
	}
}

// prepareTab locks the mutex, touches activity, ensures session, and switches tab.
// Returns true if the handler should abort (error written to w).
// Caller must hold s.mu after this returns false.
func (s *Server) prepareTab(w http.ResponseWriter, tab int) bool {
	if err := s.ensureSession(); err != nil {
		writeJSON(w, false, nil, err.Error())
		return true
	}
	if err := s.maybeSwitch(tab); err != nil {
		writeJSON(w, false, nil, err.Error())
		return true
	}
	return false
}

// registerRoutes wires all HTTP handlers onto mux.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/connect", postHandler(s.handleConnect))
	mux.HandleFunc("/disconnect", postHandler(s.handleDisconnect))
	mux.HandleFunc("/shutdown", postHandler(s.handleShutdown))
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/eval", postHandler(s.handleEval))
	mux.HandleFunc("/click", postHandler(s.handleClick))
	mux.HandleFunc("/fill", postHandler(s.handleFill))
	mux.HandleFunc("/type", postHandler(s.handleType))
	mux.HandleFunc("/press", postHandler(s.handlePress))
	mux.HandleFunc("/scroll", postHandler(s.handleScroll))
	mux.HandleFunc("/wait", postHandler(s.handleWait))
	mux.HandleFunc("/screenshot", postHandler(s.handleScreenshot))
	mux.HandleFunc("/snapshot", postHandler(s.handleSnapshot))
	mux.HandleFunc("/navigate", postHandler(s.handleNavigate))
	mux.HandleFunc("/back", postHandler(s.handleBack))
	mux.HandleFunc("/forward", postHandler(s.handleForward))
	mux.HandleFunc("/reload", postHandler(s.handleReload))
	mux.HandleFunc("/get", s.handleGet)
	mux.HandleFunc("/tabs", s.handleTabs)
	mux.HandleFunc("/tab/switch", postHandler(s.handleTabSwitch))
}

// --- request body structs ---

type connectReq struct {
	Port int `json:"port"`
	Tab  int `json:"tab"`
}

type tabReq struct {
	Tab int `json:"tab"`
}

type selectorReq struct {
	Tab      int    `json:"tab"`
	Selector string `json:"selector"`
}

type fillReq struct {
	Tab      int    `json:"tab"`
	Selector string `json:"selector"`
	Text     string `json:"text"`
}

type pressReq struct {
	Tab int    `json:"tab"`
	Key string `json:"key"`
}

type scrollReq struct {
	Tab       int    `json:"tab"`
	Direction string `json:"direction"`
	Pixels    int    `json:"pixels"`
}

type waitReq struct {
	Tab       int    `json:"tab"`
	Condition string `json:"condition"`
}

type evalReq struct {
	Tab    int    `json:"tab"`
	Script string `json:"script"`
}

type screenshotReq struct {
	Tab  int    `json:"tab"`
	Path string `json:"path"`
	Full bool   `json:"full"`
}

type snapshotReq struct {
	Tab         int  `json:"tab"`
	Interactive bool `json:"interactive"`
	Compact     bool `json:"compact"`
	Depth       int  `json:"depth"`
}

type navigateReq struct {
	Tab int    `json:"tab"`
	URL string `json:"url"`
}

type tabSwitchReq struct {
	Index int `json:"index"`
}

// --- handlers ---

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	req := connectReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	// Use a long-lived context for the session — it must survive beyond this handler.
	// Do NOT use context.WithTimeout here: canceling the context invalidates the session.
	ctx := context.Background()

	var sess *cdp.Session
	var err error
	if req.Tab >= 0 {
		sess, err = cdp.ConnectToTarget(ctx, req.Port, req.Tab)
	} else {
		sess, err = cdp.Connect(ctx, req.Port)
	}
	if err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}

	if s.session != nil {
		old := s.session
		s.session = nil
		old.Disconnect()
	}
	s.session = sess
	s.port = req.Port
	s.currentTab = req.Tab
	writeJSON(w, true, nil, "")
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()
	if s.session != nil {
		s.session.Disconnect()
		s.session = nil
	}
	writeJSON(w, true, nil, "")
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, true, nil, "")
	// Shutdown after response is flushed
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpSrv.Shutdown(ctx) //nolint:errcheck
	}()
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	connected := s.session != nil
	port := s.port
	tab := s.currentTab
	s.mu.Unlock()

	writeJSON(w, true, StatusResponse{
		Connected: connected,
		Port:      port,
		Tab:       tab,
		PID:       os.Getpid(),
	}, "")
}

func (s *Server) handleEval(w http.ResponseWriter, r *http.Request) {
	req := evalReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if s.prepareTab(w, req.Tab) {
		return
	}

	result, err := cdp.Eval(s.session, req.Script)
	if err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, result, "")
}

func (s *Server) handleClick(w http.ResponseWriter, r *http.Request) {
	req := selectorReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if s.prepareTab(w, req.Tab) {
		return
	}
	if err := cdp.Click(s.session, req.Selector); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, nil, "")
}

func (s *Server) handleFill(w http.ResponseWriter, r *http.Request) {
	req := fillReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if s.prepareTab(w, req.Tab) {
		return
	}
	if err := cdp.Fill(s.session, req.Selector, req.Text); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, nil, "")
}

func (s *Server) handleType(w http.ResponseWriter, r *http.Request) {
	req := fillReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if s.prepareTab(w, req.Tab) {
		return
	}
	if err := cdp.Type(s.session, req.Selector, req.Text); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, nil, "")
}

func (s *Server) handlePress(w http.ResponseWriter, r *http.Request) {
	req := pressReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if s.prepareTab(w, req.Tab) {
		return
	}
	if err := cdp.Press(s.session, req.Key); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, nil, "")
}

func (s *Server) handleScroll(w http.ResponseWriter, r *http.Request) {
	req := scrollReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if s.prepareTab(w, req.Tab) {
		return
	}
	if err := cdp.Scroll(s.session, req.Direction, req.Pixels); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, nil, "")
}

func (s *Server) handleWait(w http.ResponseWriter, r *http.Request) {
	req := waitReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	s.touch()
	if s.prepareTab(w, req.Tab) {
		s.mu.Unlock()
		return
	}
	// Capture session then release lock: Wait can block for up to 30s.
	sess := s.session
	s.mu.Unlock()

	if err := cdp.Wait(sess, req.Condition); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, nil, "")
}

func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	req := screenshotReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	s.touch()
	if s.prepareTab(w, req.Tab) {
		s.mu.Unlock()
		return
	}
	sess := s.session
	s.mu.Unlock() // release before I/O

	var path string
	var err error
	if req.Full {
		path, err = cdp.FullScreenshot(sess, req.Path)
	} else {
		path, err = cdp.Screenshot(sess, req.Path)
	}
	if err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, path, "")
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	req := snapshotReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	s.touch()
	if s.prepareTab(w, req.Tab) {
		s.mu.Unlock()
		return
	}
	sess := s.session
	s.mu.Unlock() // release before I/O

	result, err := cdp.Snapshot(sess, cdp.SnapshotOptions{
		InteractiveOnly: req.Interactive,
		Compact:         req.Compact,
		MaxDepth:        req.Depth,
	})
	if err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, result.FormatText(), "")
}

func (s *Server) handleNavigate(w http.ResponseWriter, r *http.Request) {
	req := navigateReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if s.prepareTab(w, req.Tab) {
		return
	}
	if err := cdp.Navigate(s.session, req.URL); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, nil, "")
}

func (s *Server) handleBack(w http.ResponseWriter, r *http.Request) {
	req := tabReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if s.prepareTab(w, req.Tab) {
		return
	}
	if err := cdp.Back(s.session); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, nil, "")
}

func (s *Server) handleForward(w http.ResponseWriter, r *http.Request) {
	req := tabReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if s.prepareTab(w, req.Tab) {
		return
	}
	if err := cdp.Forward(s.session); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, nil, "")
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	req := tabReq{Tab: -1}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if s.prepareTab(w, req.Tab) {
		return
	}
	if err := cdp.Reload(s.session); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, nil, "")
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	what := r.URL.Query().Get("what")
	tab := -1
	if tabStr := r.URL.Query().Get("tab"); tabStr != "" {
		fmt.Sscanf(tabStr, "%d", &tab) //nolint:errcheck
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if s.prepareTab(w, tab) {
		return
	}

	var result string
	var err error
	switch what {
	case "url":
		result, err = cdp.GetURL(s.session)
	case "title":
		result, err = cdp.GetTitle(s.session)
	default:
		writeJSON(w, false, nil, fmt.Sprintf("unknown 'what' param: %q (use 'url' or 'title')", what))
		return
	}
	if err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, result, "")
}

func (s *Server) handleTabs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if err := s.ensureSession(); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	tabs, err := cdp.ListTabs(s.session)
	if err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	writeJSON(w, true, tabs, "")
}

func (s *Server) handleTabSwitch(w http.ResponseWriter, r *http.Request) {
	var req tabSwitchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, false, nil, fmt.Sprintf("decode: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.touch()

	if err := s.ensureSession(); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	if err := cdp.SwitchTab(s.session, req.Index); err != nil {
		writeJSON(w, false, nil, err.Error())
		return
	}
	s.currentTab = req.Index
	writeJSON(w, true, nil, "")
}
