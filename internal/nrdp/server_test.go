package nrdp

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/logging"
	"github.com/oceanplexian/gogios/internal/objects"

	"golang.org/x/crypto/bcrypt"
)

func testLogger(t *testing.T) *logging.Logger {
	t.Helper()
	dir := t.TempDir()
	l, err := logging.NewLogger(filepath.Join(dir, "test.log"), dir, 0, false, &objects.GlobalState{})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func testServer(t *testing.T, tokenHash string, dynamic bool) (*Server, *objects.ObjectStore, chan *objects.CheckResult) {
	t.Helper()
	store := objects.NewObjectStore()
	resultCh := make(chan *objects.CheckResult, 100)
	logger := testLogger(t)
	cfg := Config{
		Listen:         ":0",
		Path:           "/nrdp/",
		TokenHash:      tokenHash,
		DynamicEnabled: dynamic,
		DynamicTTL:     10 * time.Minute,
		DynamicPrune:   1 * time.Minute,
	}
	s := New(cfg, store, resultCh, logger)
	return s, store, resultCh
}

func hashToken(t *testing.T, token string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(token), 4)
	if err != nil {
		t.Fatal(err)
	}
	return string(h)
}

func TestMethodNotAllowed(t *testing.T) {
	s, _, _ := testServer(t, "", false)
	req := httptest.NewRequest(http.MethodGet, "/nrdp/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)
	if w.Code != 405 {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestAuthLocalhostBypass(t *testing.T) {
	s, _, _ := testServer(t, "", false)
	body := strings.NewReader(`{"checkresults":[{"type":"service","hostname":"h","servicename":"s","status":0,"output":"ok"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/nrdp/", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAuthValidToken(t *testing.T) {
	hash := hashToken(t, "testtoken")
	s, _, _ := testServer(t, hash, false)
	formData := url.Values{
		"XMLDATA": {`<checkresults><checkresult type="service" checktype="1"><hostname>h</hostname><servicename>s</servicename><state>0</state><output>ok</output></checkresult></checkresults>`},
		"token":   {"testtoken"},
		"cmd":     {"submitcheck"},
	}
	req := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAuthInvalidToken(t *testing.T) {
	hash := hashToken(t, "testtoken")
	s, _, _ := testServer(t, hash, false)
	formData := url.Values{
		"XMLDATA": {`<checkresults><checkresult type="service" checktype="1"><hostname>h</hostname><servicename>s</servicename><state>0</state><output>ok</output></checkresult></checkresults>`},
		"token":   {"wrongtoken"},
	}
	req := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)
	if w.Code != 401 {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuthMissingToken(t *testing.T) {
	hash := hashToken(t, "testtoken")
	s, _, _ := testServer(t, hash, false)
	req := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(`{"checkresults":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)
	if w.Code != 401 {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuthEmptyHash(t *testing.T) {
	s, _, _ := testServer(t, "", false)
	req := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(`{"checkresults":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)
	if w.Code != 401 {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestXMLFormPost(t *testing.T) {
	hash := hashToken(t, "test")
	s, _, resultCh := testServer(t, hash, false)

	xmlData := `<checkresults><checkresult type="service" checktype="1"><hostname>web01</hostname><servicename>HTTP</servicename><state>0</state><output>OK</output></checkresult></checkresults>`
	formData := url.Values{
		"XMLDATA": {xmlData},
		"token":   {"test"},
		"cmd":     {"submitcheck"},
	}
	req := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	select {
	case cr := <-resultCh:
		if cr.HostName != "web01" {
			t.Errorf("hostname = %q, want web01", cr.HostName)
		}
		if cr.ServiceDescription != "HTTP" {
			t.Errorf("service = %q, want HTTP", cr.ServiceDescription)
		}
		if cr.ReturnCode != 0 {
			t.Errorf("returnCode = %d, want 0", cr.ReturnCode)
		}
	case <-time.After(time.Second):
		t.Fatal("no result received on channel")
	}
}

func TestJSONPost(t *testing.T) {
	s, _, resultCh := testServer(t, "", false)

	jsonBody := `{"checkresults":[{"type":"service","hostname":"app01","servicename":"CPU","status":1,"output":"WARNING - 90%"}]}`
	req := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	select {
	case cr := <-resultCh:
		if cr.HostName != "app01" || cr.ServiceDescription != "CPU" || cr.ReturnCode != 1 {
			t.Errorf("result = %+v", cr)
		}
	case <-time.After(time.Second):
		t.Fatal("no result")
	}
}

func TestBatchResults(t *testing.T) {
	s, _, resultCh := testServer(t, "", false)

	jsonBody := `{"checkresults":[
		{"type":"service","hostname":"h1","servicename":"s1","status":0,"output":"ok"},
		{"type":"service","hostname":"h2","servicename":"s2","status":1,"output":"warn"},
		{"type":"service","hostname":"h3","servicename":"s3","status":2,"output":"crit"}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	// Check response message
	bodyBytes, _ := io.ReadAll(w.Result().Body)
	var resp ResponseJSON
	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !strings.Contains(resp.Message, "3 Results") {
		t.Errorf("message = %q, want 'Processing 3 Results'", resp.Message)
	}

	// Drain channel
	count := 0
	for i := 0; i < 3; i++ {
		select {
		case <-resultCh:
			count++
		case <-time.After(time.Second):
			break
		}
	}
	if count != 3 {
		t.Errorf("received %d results, want 3", count)
	}
}

func TestStatusClamping(t *testing.T) {
	s, _, resultCh := testServer(t, "", false)

	jsonBody := `{"checkresults":[{"type":"service","hostname":"h","servicename":"s","status":5,"output":"bad"}]}`
	req := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	select {
	case cr := <-resultCh:
		if cr.ReturnCode != 3 {
			t.Errorf("returnCode = %d, want 3 (clamped from 5)", cr.ReturnCode)
		}
	case <-time.After(time.Second):
		t.Fatal("no result")
	}
}

func TestDynamicRegistration(t *testing.T) {
	s, _, resultCh := testServer(t, "", true)

	jsonBody := `{"checkresults":[{"type":"service","hostname":"dynamic-host","servicename":"dynamic-svc","status":0,"output":"ok"}]}`
	req := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	// The handler no longer creates hosts/services itself — it sets
	// DynamicRegister on the CheckResult so the scheduler callback
	// can create them under its existing store lock.
	select {
	case cr := <-resultCh:
		if !cr.DynamicRegister {
			t.Error("DynamicRegister = false, want true")
		}
		if cr.HostName != "dynamic-host" {
			t.Errorf("HostName = %q, want dynamic-host", cr.HostName)
		}
		if cr.ServiceDescription != "dynamic-svc" {
			t.Errorf("ServiceDescription = %q, want dynamic-svc", cr.ServiceDescription)
		}
	case <-time.After(time.Second):
		t.Fatal("no result")
	}
}

func TestDynamicRegistrationDisabled(t *testing.T) {
	s, _, resultCh := testServer(t, "", false)

	jsonBody := `{"checkresults":[{"type":"service","hostname":"h","servicename":"s","status":0,"output":"ok"}]}`
	req := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}

	select {
	case cr := <-resultCh:
		if cr.DynamicRegister {
			t.Error("DynamicRegister = true, want false when dynamic disabled")
		}
	case <-time.After(time.Second):
		t.Fatal("no result")
	}
}

// BenchmarkHandleNRDP measures raw handler throughput with dynamic enabled.
// After removing per-request store.Mu locks, the handler no longer contends
// with concurrent readers (e.g. livestatus queries).
func BenchmarkHandleNRDP(b *testing.B) {
	store := objects.NewObjectStore()
	resultCh := make(chan *objects.CheckResult, 65536)
	dir := b.TempDir()
	logger, err := logging.NewLogger(dir+"/test.log", dir, 0, false, &objects.GlobalState{})
	if err != nil {
		b.Fatal(err)
	}
	cfg := Config{
		Listen:         ":0",
		Path:           "/nrdp/",
		DynamicEnabled: true,
		DynamicTTL:     10 * time.Minute,
		DynamicPrune:   1 * time.Minute,
	}
	s := New(cfg, store, resultCh, logger)

	jsonBody := `{"checkresults":[{"type":"service","hostname":"h1","servicename":"s1","status":0,"output":"ok"}]}`

	// Drain channel in background
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-resultCh:
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "127.0.0.1:12345"
			w := httptest.NewRecorder()
			s.handleNRDP(w, req)
		}
	})
}

func TestResponseMirrorsFormat(t *testing.T) {
	s, _, _ := testServer(t, "", false)

	// XML request -> XML response
	xmlData := `<checkresults><checkresult type="service" checktype="1"><hostname>h</hostname><servicename>s</servicename><state>0</state><output>ok</output></checkresult></checkresults>`
	req := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(xmlData))
	req.Header.Set("Content-Type", "text/xml")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.handleNRDP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "text/xml" {
		t.Errorf("XML request: response Content-Type = %q, want text/xml", ct)
	}
	// Verify it's valid XML
	var xmlResp ResponseXML
	if err := xml.Unmarshal(w.Body.Bytes(), &xmlResp); err != nil {
		t.Errorf("XML response not valid XML: %v", err)
	}

	// JSON request -> JSON response
	jsonBody := `{"checkresults":[{"type":"service","hostname":"h","servicename":"s","status":0,"output":"ok"}]}`
	req2 := httptest.NewRequest(http.MethodPost, "/nrdp/", strings.NewReader(jsonBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "127.0.0.1:12345"
	w2 := httptest.NewRecorder()
	s.handleNRDP(w2, req2)

	if ct := w2.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("JSON request: response Content-Type = %q, want application/json", ct)
	}
	var jsonResp ResponseJSON
	if err := json.Unmarshal(w2.Body.Bytes(), &jsonResp); err != nil {
		t.Errorf("JSON response not valid JSON: %v", err)
	}
}
