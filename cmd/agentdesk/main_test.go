package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"agentdesk/internal/desk"
)

func capture(t *testing.T, fn func() (int, error)) (int, error, string) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()
	done := make(chan string, 1)
	go func() { b, _ := io.ReadAll(reader); reader.Close(); done <- string(b) }()
	code, runErr := fn()
	writer.Close()
	return code, runErr, <-done
}
func TestCLIWorkflowAndExitCodes(t *testing.T) {
	dir := t.TempDir()
	c, err := desk.Init(dir, "CLI test")
	if err != nil {
		t.Fatal(err)
	}
	s, err := desk.OpenStore(dir, c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	server := httptest.NewServer((&desk.Server{Store: s, Dir: dir, Assets: fstest.MapFS{"index.html": {Data: []byte("test")}}}).Handler())
	defer server.Close()
	c.Endpoint = server.URL
	c.PublicURL = server.URL
	if err = desk.WriteJSON(filepath.Join(dir, "project.json"), c); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "question.json")
	os.WriteFile(file, []byte(`{"kind":"decision","title":"CLI question","context":"test"}`), 0600)
	invoke := func(args ...string) (int, error, json.RawMessage) {
		t.Helper()
		args = append(args, "--data-dir", dir)
		code, err, out := capture(t, func() (int, error) { return run(context.Background(), args) })
		var e struct {
			SchemaVersion int             `json:"schema_version"`
			Data          json.RawMessage `json:"data"`
		}
		if out != "" {
			if parseErr := json.Unmarshal([]byte(out), &e); parseErr != nil {
				t.Fatalf("invalid machine output: %v", parseErr)
			}
			if e.SchemaVersion != 1 {
				t.Fatal("schema version missing")
			}
		}
		return code, err, e.Data
	}
	code, err, raw := invoke("create", "--file", file, "--key", "same", "--json")
	if code != 0 || err != nil {
		t.Fatal(code, err)
	}
	var i desk.Issue
	if err = json.Unmarshal(raw, &i); err != nil {
		t.Fatal(err)
	}
	_, _, retry := invoke("create", "--file", file, "--key", "same")
	var again desk.Issue
	json.Unmarshal(retry, &again)
	if again.ID != i.ID {
		t.Fatal("CLI retry duplicated issue")
	}
	if code, err, _ = invoke("wait", i.ID, "--timeout", "0s", "--json"); code != 3 || err != nil {
		t.Fatalf("wait timeout exit: %d %v", code, err)
	}
	if _, err = s.Mutate(i.ID, "reply", "human:owner", desk.Mutation{Revision: 1, Text: "go"}); err != nil {
		t.Fatal(err)
	}
	if code, err, _ = invoke("wait", i.ID, "--timeout", "0s"); code != 0 || err != nil {
		t.Fatal("existing answer not immediate")
	}
	code, err, events := invoke("inbox", "--consumer", "coordinator")
	var inbox []desk.Event
	json.Unmarshal(events, &inbox)
	if code != 0 || err != nil || len(inbox) != 1 {
		t.Fatal("CLI inbox failed")
	}
	if code, _, _ = invoke("receipt", inbox[0].ID, "--holder", "one"); code != 4 {
		t.Fatal("missing lease did not use conflict exit")
	}
	if code, err, _ = invoke("claim", "--holder", "one"); code != 0 || err != nil {
		t.Fatal(code, err)
	}
	if code, _, _ = invoke("claim", "--holder", "two"); code != 4 {
		t.Fatal("overlap did not conflict")
	}
	if code, err, _ = invoke("receipt", inbox[0].ID, "--holder", "one"); code != 0 || err != nil {
		t.Fatal(code, err)
	}
	if code, err, raw = invoke("inbox"); code != 0 || err != nil || string(raw) != "[]" {
		t.Fatal("receipt did not persist")
	}
	if code, _, _ = invoke("withdraw", i.ID, "--revision", "1"); code != 4 {
		t.Fatal("answered issue withdrew")
	}
	if code, _, _ = invoke("get"); code != 2 {
		t.Fatal("missing ID exit")
	}
	if code, _, _ = invoke("unknown"); code != 2 {
		t.Fatal("unknown command exit")
	}
}

func TestWatchReconnectDeduplicatesWithoutReceipt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	var badCursor atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n > 1 && r.URL.Query().Get("after") != "3" && r.URL.Query().Get("after") != "4" {
			badCursor.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1:
			json.NewEncoder(w).Encode(desk.Envelope{SchemaVersion: 1, Data: []desk.Event{{Seq: 3, ID: "E-first"}}})
		case 2:
			w.WriteHeader(503)
			json.NewEncoder(w).Encode(desk.Envelope{SchemaVersion: 1, Error: "temporary disconnect"})
		case 3:
			json.NewEncoder(w).Encode(desk.Envelope{SchemaVersion: 1, Data: []desk.Event{{Seq: 3, ID: "E-first"}, {Seq: 4, ID: "E-second"}}})
		default:
			cancel()
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	client := apiClient{config: desk.Config{Endpoint: server.URL}, http: server.Client()}
	code, err, out := capture(t, func() (int, error) { return client.watch(ctx, options{consumer: "coordinator"}) })
	if code != 0 || err != nil {
		t.Fatal(code, err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("watch lost/duplicated events: %d lines", len(lines))
	}
	if badCursor.Load() {
		t.Fatal("reconnect lost cursor")
	}
	for n, line := range lines {
		var e struct {
			Data desk.Event `json:"data"`
		}
		if err = json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatal(err)
		}
		if e.Data.Seq != int64(n+3) {
			t.Fatal("unexpected sequence")
		}
	}
}

func TestParseInterspersedFlags(t *testing.T) {
	command, o, args, err := parse([]string{"get", "D-123", "--json", "--data-dir", "some path"})
	if err != nil || command != "get" || len(args) != 1 || !o.json || o.dataDir != "some path" {
		t.Fatal("interspersed flags rejected")
	}
}

func TestServePIDLifecycle(t *testing.T) {
	dir := t.TempDir()
	c, err := desk.Init(dir, "PID lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "server.pid")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("init must not publish a server PID")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := serve(ctx, dir, c, options{listen: "127.0.0.1:0"}); done <- err }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		b, readErr := os.ReadFile(path)
		if readErr == nil && strings.TrimSpace(string(b)) == strconv.Itoa(os.Getpid()) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not publish its PID")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("server did not shut down")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("graceful shutdown retained the PID: %v", err)
	}
	unlock, err := desk.LockServer(dir)
	if err != nil {
		t.Fatal("shutdown retained the project lock", err)
	}
	unlock()
	// A startup error must clean up both PID and lock as well.
	if _, err := serve(context.Background(), dir, c, options{listen: "not-a-valid-listen-address"}); err == nil {
		t.Fatal("invalid listen address succeeded")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("failed startup retained the PID")
	}
}
