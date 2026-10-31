package desk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

type fixture struct {
	store  *Store
	dir    string
	server *httptest.Server
	cookie *http.Cookie
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	s, dir := testStore(t)
	f := &fixture{store: s, dir: dir}
	handler := (&Server{Store: s, Dir: dir, Assets: fstest.MapFS{"index.html": {Data: []byte("ui")}}}).Handler()
	f.server = httptest.NewServer(handler)
	t.Cleanup(f.server.Close)
	return f
}
func (f *fixture) request(t *testing.T, method, path string, body any, agent bool, origin string) (int, Envelope, []*http.Cookie) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, f.server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if agent {
		req.Header.Set("Authorization", "Bearer "+f.store.Config.AgentToken)
		req.Header.Set("X-Project-ID", f.store.Config.ProjectID)
	}
	if f.cookie != nil {
		req.AddCookie(f.cookie)
	}
	res, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var env Envelope
	if err = json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, env, res.Cookies()
}
func (f *fixture) login(t *testing.T) {
	t.Helper()
	h, err := LoadHuman(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	status, _, cookies := f.request(t, "POST", "/api/v1/session", map[string]string{"token": h.Token}, false, f.server.URL)
	if status != 200 || len(cookies) != 1 {
		t.Fatalf("login status %d", status)
	}
	f.cookie = cookies[0]
	if !f.cookie.HttpOnly || f.cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("cookie missing security properties")
	}
}

func TestHTTPHumanAgentSeparationAndCrossProject(t *testing.T) {
	f := newFixture(t)
	i := mustCreate(t, f.store, draft("decision"), "")
	path := "/api/v1/issues/" + i.ID + "/reply"
	answer := map[string]any{"revision": 1, "text": "yes"}
	if status, _, _ := f.request(t, "POST", path, answer, true, f.server.URL); status != 401 {
		t.Fatalf("agent answered: %d", status)
	}
	if status, _, _ := f.request(t, "POST", "/api/v1/session", map[string]string{"token": f.store.Config.AgentToken}, true, f.server.URL); status != 401 {
		t.Fatal("agent logged in as human")
	}
	other := newFixture(t)
	req, _ := http.NewRequest("GET", other.server.URL+"/api/v1/agent/issues", nil)
	req.Header.Set("Authorization", "Bearer "+f.store.Config.AgentToken)
	req.Header.Set("X-Project-ID", f.store.Config.ProjectID)
	res, err := other.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("project A accessed project B")
	}
	f.login(t)
	if status, _, _ := f.request(t, "POST", path, answer, false, "https://untrusted.invalid"); status != 403 {
		t.Fatal("cross-site reply accepted")
	}
	if status, _, _ := f.request(t, "POST", path, answer, false, ""); status != 403 {
		t.Fatal("missing origin accepted")
	}
	spoof := map[string]any{"revision": 1, "text": "yes", "actor": "human:someone-else"}
	if status, _, _ := f.request(t, "POST", path, spoof, false, f.server.URL); status != 400 {
		t.Fatal("actor field accepted")
	}
	if status, _, _ := f.request(t, "POST", path, answer, false, f.server.URL); status != 200 {
		t.Fatalf("human reply failed: %d", status)
	}
	got, _ := f.store.Get(i.ID)
	if got.Reply.Actor != "human:owner" {
		t.Fatal("actor not server-derived")
	}
	if status, _, _ := f.request(t, "POST", path, answer, false, f.server.URL); status != 200 {
		t.Fatal("reply retry failed")
	}
	answer["text"] = "conflict"
	if status, _, _ := f.request(t, "POST", path, answer, false, f.server.URL); status != 409 {
		t.Fatal("reply conflict not reported")
	}
}

func TestSessionPersistsAndRotationRevokes(t *testing.T) {
	f := newFixture(t)
	f.login(t)
	token := f.cookie.Value
	if !f.store.ValidSession(token) {
		t.Fatal("session missing")
	}
	if status, _, _ := f.request(t, "GET", "/api/v1/session", nil, false, ""); status != 200 {
		t.Fatal("session rejected")
	}
	if err := f.store.RotateHuman(f.dir); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := f.request(t, "GET", "/api/v1/session", nil, false, ""); status != 401 {
		t.Fatal("rotation did not revoke session")
	}
	f.login(t)
	if status, _, _ := f.request(t, "DELETE", "/api/v1/session", nil, false, f.server.URL); status != 200 {
		t.Fatal("logout failed")
	}
	if f.store.ValidSession(f.cookie.Value) {
		t.Fatal("logout left session usable")
	}
}

func TestHTTPWaitAndSecurityHeaders(t *testing.T) {
	f := newFixture(t)
	i := mustCreate(t, f.store, draft("notice"), "")
	status, env, _ := f.request(t, "GET", "/api/v1/agent/wait?issue="+i.ID+"&timeout=1ms", nil, true, "")
	if status != 200 || env.Data.(map[string]any)["timed_out"] != true {
		t.Fatalf("timeout result: %+v", env)
	}
	if status, _, _ := f.request(t, "GET", "/api/v1/agent/wait?after=bad", nil, true, ""); status != 400 {
		t.Fatal("invalid cursor accepted")
	}
	req, _ := http.NewRequest("GET", f.server.URL+"/", nil)
	res, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	for _, header := range []string{"Content-Security-Policy", "Referrer-Policy", "X-Content-Type-Options", "Cache-Control"} {
		if res.Header.Get(header) == "" {
			t.Errorf("missing %s", header)
		}
	}
	if strings.Contains(fmt.Sprint(res.Header), f.store.Config.AgentToken) {
		t.Fatal("credential in response headers")
	}
}
