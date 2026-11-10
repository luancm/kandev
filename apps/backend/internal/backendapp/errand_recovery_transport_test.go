//go:build linux

package backendapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/processidentity"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type recoveryProviderPrompt struct {
	Text         string `json:"text"`
	SubmissionID string `json:"delivery_submission_id"`
	Generation   uint64 `json:"prompt_generation"`
}

type recoveryProvider struct {
	process *exec.Cmd

	server                       *httptest.Server
	journal                      *journal.Journal
	session, incarnation, stream string
	prompts                      chan recoveryProviderPrompt
}

func newRecoveryProvider(t *testing.T, session, incarnation string) *recoveryProvider {
	t.Helper()
	j, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "journal.bbolt")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, j.Close()) })
	p := &recoveryProvider{journal: j, session: session, incarnation: incarnation, stream: session + "-stream", prompts: make(chan recoveryProviderPrompt, 10)}
	p.server = httptest.NewServer(http.HandlerFunc(p.serveHTTP))
	t.Cleanup(p.server.Close)
	return p
}

func (p *recoveryProvider) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer fixture-secret" {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/api/v1/status":
		writeManagedGoCacheJSON(w, map[string]any{"agent_status": "running"})
	case r.URL.Path == "/api/v1/agent/stream":
		p.serveStream(w, r)
	case r.URL.Path == "/api/v1/agent/delivery":
		value, err := p.journal.RecoveryDescriptor(r.Context(), p.session, p.incarnation, 2, p.stream)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeManagedGoCacheJSON(w, value)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/submissions":
		value, err := p.journal.ListSubmissions(r.Context(), p.session)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeManagedGoCacheJSON(w, value)
	case strings.HasSuffix(r.URL.Path, "/retire"):
		_, err := p.journal.RetireSubmission(r.Context(), strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/agent/submissions/"), "/retire"), 2)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unexpected transport operation", http.StatusNotFound)
	}
}

func (p *recoveryProvider) serveStream(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	for {
		var request ws.Message
		if conn.ReadJSON(&request) != nil {
			return
		}
		payload := map[string]any{"success": true}
		switch request.Action {
		case "agent.initialize":
			payload["agent_info"] = map[string]string{"name": "fixture", "version": "1"}
			payload["durable_delivery"] = map[string]any{"version": journal.CurrentVersion, "durable": true}
		case "agent.session.new", "agent.session.load":
			payload["session_id"] = "native-" + p.session
		case "agent.prompt":
			var prompt recoveryProviderPrompt
			if json.Unmarshal(request.Payload, &prompt) != nil {
				return
			}
			raw, err := json.Marshal(struct {
				Text string `json:"text"`
			}{prompt.Text})
			if err != nil {
				return
			}
			_, err = p.journal.PutSubmission(r.Context(), journal.Submission{ID: prompt.SubmissionID, SessionID: p.session, IncarnationID: p.incarnation, HarnessGeneration: 2, StreamID: p.stream, Payload: raw, Hash: journal.SubmissionHash(raw), State: journal.SubmissionDispatching})
			if err != nil {
				return
			}
			payload["delivery_submission_id"] = prompt.SubmissionID
			p.prompts <- prompt
		}
		response, err := ws.NewResponse(request.ID, request.Action, payload)
		if err != nil || conn.WriteJSON(response) != nil {
			return
		}
	}
}

func (p *recoveryProvider) port(t *testing.T) int {
	t.Helper()
	u, err := url.Parse(p.server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return port
}

func (p *recoveryProvider) ownedClient(t *testing.T) *client.Client {
	t.Helper()
	process := exec.CommandContext(t.Context(), "/bin/sleep", "120")
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, process.Start())
	t.Cleanup(func() { _ = process.Process.Kill(); _ = process.Wait() })
	p.process = process
	birth, err := processidentity.Capture(process.Process.Pid)
	require.NoError(t, err)
	owner := client.NewRuntimeOwner(nil, logger.Default(), p.session)
	t.Cleanup(owner.Stop)
	binding, err := owner.PrepareBinding()
	require.NoError(t, err)
	require.NoError(t, binding.SetProcessIdentity(birth))
	require.NoError(t, binding.Configure("127.0.0.1", p.port(t), "fixture-secret", birth.PID, nil))
	require.NoError(t, binding.Commit())
	lease, err := owner.Acquire(t.Context())
	require.NoError(t, err)
	defer lease.Close()
	c := lease.NewBoundInstanceClient(p.port(t), logger.Default())
	t.Cleanup(c.Close)
	return c
}

func (p *recoveryProvider) interrupt(t *testing.T, id string) {
	t.Helper()
	require.NoError(t, p.process.Process.Kill())
	require.Error(t, p.process.Wait())
	_, err := p.journal.TransitionSubmission(t.Context(), id, journal.SubmissionInterruptedUnknown, time.Now().UTC())
	require.NoError(t, err)
}
