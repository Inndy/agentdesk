package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	webassets "agentdesk/agentdesk-ui"
	"agentdesk/internal/desk"
)

const usage = `Agentdesk — local human issue inbox

  agentdesk init [--name NAME] [--data-dir PATH]
  agentdesk start [--telegram] [--telegram-config PATH]   Init/reuse detached server
  agentdesk status --json               Verify project service identity
  agentdesk serve [--listen 127.0.0.1:7331] [--public-url URL] [--open]
  agentdesk serve --telegram [--telegram-config PATH]   Auto-detect and bind NIC
  agentdesk network --json              Show default-route interface address
  agentdesk notifications --json        Inspect Telegram delivery state
  agentdesk login [--open] [--rotate]     Human-only: emits a secret login URL
  agentdesk create --file question.json --key stable-key --json
  agentdesk get ISSUE-ID --json
  agentdesk list [--pending] --json
  agentdesk history ISSUE-ID --json
  agentdesk update ISSUE-ID --file question.json --revision N --json
  agentdesk withdraw ISSUE-ID --revision N --json
  agentdesk inbox [--consumer coordinator] --json
  agentdesk wait [ISSUE-ID] [--consumer coordinator] [--timeout 50s] --json
  agentdesk watch [--consumer coordinator] [--after SEQ] --jsonl
  agentdesk claim --holder SESSION-ID [--ttl 60s] [--release] --json
  agentdesk receipt EVENT-ID --holder SESSION-ID [--consumer coordinator] --json

Global: --data-dir PATH (or AGENTDESK_DATA_DIR). Agent commands always emit
schema_version=1 JSON. stdin: --file -. Exit codes: 0 success, 1 error,
2 invalid arguments, 3 wait timeout, 4 conflict/lease conflict. Ctrl+C cancels.
`

type options struct {
	publicURLExplicit, telegramExplicit, telegramConfigExplicit        bool
	dataDir, name, listen, publicURL, file, key, consumer, holder, ttl string
	telegramConfig                                                     string
	telegram, listenExplicit                                           bool
	timeout                                                            time.Duration
	revision                                                           int
	after                                                              int64
	pending, open, rotate, release, json, jsonl                        bool
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code, err := run(ctx, os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentdesk:", err)
	}
	os.Exit(code)
}
func parse(args []string) (string, options, []string, error) {
	var o options
	if len(args) == 0 {
		return "help", o, nil, nil
	}
	command := args[0]
	if command == "--help" || command == "-h" {
		command = "help"
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.dataDir, "data-dir", "", "state directory")
	f.StringVar(&o.name, "name", "", "project name")
	f.StringVar(&o.listen, "listen", "127.0.0.1:7331", "listen address")
	f.StringVar(&o.publicURL, "public-url", "", "browser URL")
	f.StringVar(&o.telegramConfig, "telegram-config", "", "private Telegram config path")
	f.BoolVar(&o.telegram, "telegram", false, "send issue-link notifications")
	f.StringVar(&o.file, "file", "-", "JSON file or -")
	f.StringVar(&o.key, "key", "", "idempotency key")
	f.StringVar(&o.consumer, "consumer", "coordinator", "consumer")
	f.StringVar(&o.holder, "holder", "", "coordinator session ID")
	f.StringVar(&o.ttl, "ttl", "60s", "lease duration")
	f.DurationVar(&o.timeout, "timeout", 50*time.Second, "wait timeout")
	f.IntVar(&o.revision, "revision", 0, "expected revision")
	f.Int64Var(&o.after, "after", 0, "watch cursor")
	f.BoolVar(&o.pending, "pending", false, "")
	f.BoolVar(&o.open, "open", false, "")
	f.BoolVar(&o.rotate, "rotate", false, "")
	f.BoolVar(&o.release, "release", false, "")
	f.BoolVar(&o.json, "json", false, "")
	f.BoolVar(&o.jsonl, "jsonl", false, "")
	// Permit flags after positional IDs, matching the documented CLI.
	flags, pos := []string{}, []string{}
	for n := 1; n < len(args); n++ {
		a := args[n]
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			key := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
			fl := f.Lookup(key)
			if fl != nil && !strings.Contains(a, "=") {
				if b, ok := fl.Value.(interface{ IsBoolFlag() bool }); !ok || !b.IsBoolFlag() {
					n++
					if n >= len(args) {
						return command, o, nil, fmt.Errorf("missing value for %s", a)
					}
					flags = append(flags, args[n])
				}
			}
		} else {
			pos = append(pos, a)
		}
	}
	err := f.Parse(flags)
	f.Visit(func(value *flag.Flag) {
		if value.Name == "public-url" {
			o.publicURLExplicit = true
		}
		if value.Name == "telegram" {
			o.telegramExplicit = true
		}
		if value.Name == "telegram-config" {
			o.telegramConfigExplicit = true
		}
		if value.Name == "listen" {
			o.listenExplicit = true
		}
	})
	return command, o, pos, err
}
func run(ctx context.Context, args []string) (int, error) {
	command, o, pos, err := parse(args)
	if err != nil {
		return 2, err
	}
	if command == "help" {
		fmt.Print(usage)
		return 0, nil
	}
	if command == "network" {
		nic, err := desk.DetectPublicNIC()
		if err != nil {
			return 1, err
		}
		return output(nic)
	}
	dir, err := desk.DataDir(o.dataDir)
	if err != nil {
		return 1, err
	}
	if command == "init" {
		c, err := desk.Init(dir, o.name)
		if err != nil {
			return 1, err
		}
		return output(map[string]any{"project_id": c.ProjectID, "name": c.Name, "data_dir": dir})
	}
	c, err := desk.LoadConfig(dir)
	if command == "start" && errors.Is(err, os.ErrNotExist) {
		c, err = desk.Init(dir, o.name)
	}
	if err != nil {
		return 1, err
	}
	switch command {
	case "start":
		return startServer(ctx, dir, c, o)
	case "serve":
		return serve(ctx, dir, c, o)
	case "login":
		if o.rotate {
			s, err := desk.OpenStore(dir, c)
			if err != nil {
				return 1, err
			}
			err = s.RotateHuman(dir)
			s.DB.Close()
			if err != nil {
				return 1, err
			}
		}
		h, err := desk.LoadHuman(dir)
		if err != nil {
			return 1, err
		}
		if time.Now().After(h.ExpiresAt) {
			return 1, errors.New("login token expired; run login --rotate")
		}
		link := c.PublicURL + "/#token=" + url.QueryEscape(h.Token)
		if o.open {
			return 0, openBrowser(link)
		}
		fmt.Println(link)
		return 0, nil
	}
	client := &apiClient{config: c, http: &http.Client{Timeout: 65 * time.Second}}
	id := ""
	if len(pos) > 0 {
		id = pos[0]
	}
	if len(pos) > 1 {
		return 2, errors.New("too many positional arguments")
	}
	needID := func() error {
		if id == "" {
			return errors.New("issue/event ID is required")
		}
		return nil
	}
	var v json.RawMessage
	switch command {
	case "create", "update":
		var d desk.Draft
		var b []byte
		if o.file == "-" {
			b, err = io.ReadAll(io.LimitReader(os.Stdin, 1<<20+1))
		} else {
			b, err = os.ReadFile(o.file)
		}
		if err != nil {
			return 1, err
		}
		if len(b) > 1<<20 {
			return 2, errors.New("input too large")
		}
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&d); err != nil {
			return 2, err
		}
		if decoder.Decode(new(any)) != io.EOF {
			return 2, errors.New("expected one JSON value")
		}
		if command == "create" {
			v, err = client.request(ctx, "POST", "/issues", d, o.key)
		} else {
			if err = needID(); err != nil {
				return 2, err
			}
			v, err = client.request(ctx, "POST", "/issues/"+url.PathEscape(id)+"/update", map[string]any{"revision": o.revision, "draft": d}, "")
		}
	case "get", "history", "withdraw":
		if err = needID(); err != nil {
			return 2, err
		}
		path := "/issues/" + url.PathEscape(id)
		method := "GET"
		var body any
		if command == "history" {
			path += "/history"
		}
		if command == "withdraw" {
			path += "/withdraw"
			method = "POST"
			body = map[string]int{"revision": o.revision}
		}
		v, err = client.request(ctx, method, path, body, "")
	case "list":
		v, err = client.request(ctx, "GET", "/issues?pending="+strconv.FormatBool(o.pending), nil, "")
	case "inbox":
		v, err = client.request(ctx, "GET", "/inbox?consumer="+url.QueryEscape(o.consumer), nil, "")
	case "status":
		v, err = client.request(ctx, "GET", "/status", nil, "")
	case "notifications":
		v, err = client.request(ctx, "GET", "/notifications", nil, "")
	case "wait":
		if o.timeout < 0 || o.timeout > 24*time.Hour {
			return 2, errors.New("timeout must be between 0s and 24h")
		}
		v, err = client.wait(ctx, id, o.consumer, 0, o.timeout)
		if err == nil && timedOut(v) {
			outputRaw(v)
			return 3, nil
		}
	case "watch":
		return client.watch(ctx, o)
	case "claim":
		v, err = client.request(ctx, "POST", "/claim", map[string]any{"consumer": o.consumer, "holder": o.holder, "ttl": o.ttl, "release": o.release}, "")
	case "receipt":
		if err = needID(); err != nil {
			return 2, err
		}
		v, err = client.request(ctx, "POST", "/receipt/"+url.PathEscape(id), map[string]string{"consumer": o.consumer, "holder": o.holder}, "")
	default:
		return 2, fmt.Errorf("unknown command %q; run agentdesk help", command)
	}
	if err != nil {
		var ae *apiError
		if errors.As(err, &ae) {
			if ae.status == 409 {
				return 4, err
			}
		}
		return 1, err
	}
	return outputRaw(v)
}
func output(v any) (int, error) {
	err := json.NewEncoder(os.Stdout).Encode(desk.Envelope{SchemaVersion: 1, Data: v})
	if err != nil {
		return 1, err
	}
	return 0, nil
}
func outputRaw(v json.RawMessage) (int, error) { return output(v) }

type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string { return e.message }

type apiClient struct {
	config desk.Config
	http   *http.Client
}

func (c *apiClient) request(ctx context.Context, method, path string, body any, key string) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.config.Endpoint+"/api/v1/agent"+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.config.AgentToken)
	req.Header.Set("X-Project-ID", c.config.ProjectID)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach project service at %s; start agentdesk serve", c.config.Endpoint)
	}
	defer res.Body.Close()
	var env struct {
		SchemaVersion int             `json:"schema_version"`
		Data          json.RawMessage `json:"data"`
		Error         string          `json:"error"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(&env); err != nil {
		return nil, errors.New("invalid response from project service")
	}
	if env.SchemaVersion != 1 {
		return nil, errors.New("unsupported response schema")
	}
	if res.StatusCode >= 400 {
		return nil, &apiError{res.StatusCode, env.Error}
	}
	return env.Data, nil
}
func timedOut(v json.RawMessage) bool {
	var t struct {
		TimedOut bool `json:"timed_out"`
	}
	json.Unmarshal(v, &t)
	return t.TimedOut
}
func (c *apiClient) wait(ctx context.Context, id, consumer string, after int64, timeout time.Duration) (json.RawMessage, error) {
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining < 0 {
			remaining = 0
		}
		if remaining > 50*time.Second {
			remaining = 50 * time.Second
		}
		q := url.Values{"consumer": {consumer}, "issue": {id}, "after": {strconv.FormatInt(after, 10)}, "timeout": {remaining.String()}}
		v, err := c.request(ctx, "GET", "/wait?"+q.Encode(), nil, "")
		if err != nil {
			return nil, err
		}
		if !timedOut(v) || time.Now().After(deadline) {
			return v, nil
		}
	}
}
func (c *apiClient) watch(ctx context.Context, o options) (int, error) {
	after := o.after
	backoff := time.Second
	for {
		v, err := c.wait(ctx, "", o.consumer, after, 50*time.Second)
		if ctx.Err() != nil {
			return 0, nil
		}
		if err != nil {
			var ae *apiError
			if errors.As(err, &ae) && ae.status < 500 {
				return 1, err
			}
			fmt.Fprintln(os.Stderr, "watch: disconnected; retrying")
			select {
			case <-ctx.Done():
				return 0, nil
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 15*time.Second)
			continue
		}
		backoff = time.Second
		if timedOut(v) {
			continue
		}
		var events []desk.Event
		if err = json.Unmarshal(v, &events); err != nil {
			return 1, err
		}
		for _, e := range events {
			if e.Seq <= after {
				continue
			}
			if _, err := output(e); err != nil {
				return 1, err
			}
			after = e.Seq
		}
	}
}
func serve(ctx context.Context, dir string, c desk.Config, o options) (int, error) {
	o = serverOptions(c, o)
	var telegram desk.TelegramConfig
	var err error
	if o.telegram {
		telegram, err = desk.LoadTelegram(dir, o.telegramConfig)
		if err != nil {
			return 2, err
		}
	} else if o.telegramConfigExplicit {
		return 2, errors.New("--telegram-config requires --telegram")
	}
	listen, advertise, err := desk.ResolveNetwork(o.listen, o.publicURL, o.listenExplicit, o.telegram, desk.DetectPublicNIC)
	if err != nil {
		return 2, err
	}
	unlock, err := desk.LockServer(dir)
	if err != nil {
		return 1, err
	}
	defer unlock()
	removePID, err := desk.WriteServerPID(dir)
	if err != nil {
		return 1, err
	}
	defer removePID()
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return 1, err
	}
	defer ln.Close()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	if host == "" || net.ParseIP(host).IsUnspecified() {
		host = "127.0.0.1"
	}
	c.Endpoint = "http://" + net.JoinHostPort(host, port)
	c.Server = desk.ServerDefaults{Telegram: o.telegram, TelegramConfig: o.telegramConfig, PublicURL: o.publicURL}
	if o.listenExplicit {
		c.Server.Listen = o.listen
	}
	if o.publicURL != "" {
		if err = desk.ValidateURL(o.publicURL); err != nil {
			return 2, err
		}
		c.PublicURL = strings.TrimRight(o.publicURL, "/")
	} else {
		c.PublicURL = c.Endpoint
		if advertise != "" {
			c.PublicURL = "http://" + net.JoinHostPort(advertise, port)
		}
	}
	s, err := desk.OpenStore(dir, c)
	if err != nil {
		return 1, err
	}
	defer s.DB.Close()
	s.TelegramEnabled = o.telegram
	assets, err := fs.Sub(webassets.Files, "dist")
	if err != nil {
		return 1, err
	}
	if err = desk.WriteJSON(filepath.Join(dir, "project.json"), c); err != nil {
		return 1, err
	}
	if o.telegram {
		workerCtx, stopWorker := context.WithCancel(ctx)
		workerDone := make(chan struct{})
		go func() { defer close(workerDone); (&desk.TelegramSender{Store: s, Config: telegram}).Run(workerCtx) }()
		defer func() { stopWorker(); <-workerDone }()
	}
	server := &http.Server{Handler: (&desk.Server{Store: s, Dir: dir, Assets: assets}).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 90 * time.Second, WriteTimeout: 65 * time.Second}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(ln) }()
	fmt.Fprintf(os.Stderr, "Agentdesk %s listening at %s\nRun agentdesk login --open to sign in.\n", c.Name, c.PublicURL)
	if o.open {
		h, err := desk.LoadHuman(dir)
		if err == nil && time.Now().Before(h.ExpiresAt) {
			if err = openBrowser(c.PublicURL + "/#token=" + url.QueryEscape(h.Token)); err != nil {
				fmt.Fprintln(os.Stderr, "cannot open browser; use agentdesk login")
			}
		}
	}
	select {
	case err = <-finished:
		if errors.Is(err, http.ErrServerClosed) {
			return 0, nil
		}
		return 1, err
	case <-ctx.Done():
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return 0, server.Shutdown(stop)
	}
}
func openBrowser(link string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", link)
	case "darwin":
		cmd = exec.Command("open", link)
	default:
		cmd = exec.Command("xdg-open", link)
	}
	return cmd.Run()
}
