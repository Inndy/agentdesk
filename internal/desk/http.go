package desk

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	Store  *Store
	Dir    string
	Assets fs.FS
}
type Envelope struct {
	SchemaVersion int    `json:"schema_version"`
	Data          any    `json:"data,omitempty"`
	Error         string `json:"error,omitempty"`
	Code          string `json:"code,omitempty"`
}

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Envelope{SchemaVersion: 1, Data: v})
}
func fail(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Envelope{SchemaVersion: 1, Error: msg, Code: code})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if media != "application/json" {
		fail(w, 415, "content_type", "application/json required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, "invalid_json", "invalid JSON request")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		fail(w, 400, "invalid_json", "expected one JSON value")
		return false
	}
	return true
}
func result(w http.ResponseWriter, v any, err error) {
	if err == nil {
		respond(w, 200, v)
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		fail(w, 404, "not_found", err.Error())
	case errors.Is(err, ErrConflict):
		fail(w, 409, "conflict", err.Error())
	case errors.Is(err, ErrLease):
		fail(w, 409, "lease_conflict", err.Error())
	default:
		fail(w, 400, "invalid_request", err.Error())
	}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/session", s.login)
	mux.HandleFunc("GET /api/v1/session", s.human(s.session))
	mux.HandleFunc("DELETE /api/v1/session", s.human(s.logout))
	mux.HandleFunc("GET /api/v1/issues", s.human(s.list))
	mux.HandleFunc("GET /api/v1/issues/{id}", s.human(s.get))
	mux.HandleFunc("GET /api/v1/issues/{id}/history", s.human(s.history))
	mux.HandleFunc("POST /api/v1/issues/{id}/reply", s.human(s.reply))
	mux.HandleFunc("POST /api/v1/issues/{id}/defer", s.human(s.deferIssue))
	mux.HandleFunc("GET /api/v1/agent/issues", s.agent(s.list))
	mux.HandleFunc("POST /api/v1/agent/issues", s.agent(s.create))
	mux.HandleFunc("GET /api/v1/agent/issues/{id}", s.agent(s.get))
	mux.HandleFunc("GET /api/v1/agent/issues/{id}/history", s.agent(s.history))
	mux.HandleFunc("POST /api/v1/agent/issues/{id}/update", s.agent(s.update))
	mux.HandleFunc("POST /api/v1/agent/issues/{id}/withdraw", s.agent(s.withdraw))
	mux.HandleFunc("GET /api/v1/agent/inbox", s.agent(s.inbox))
	mux.HandleFunc("GET /api/v1/agent/wait", s.agent(s.wait))
	mux.HandleFunc("POST /api/v1/agent/receipt/{id}", s.agent(s.receipt))
	mux.HandleFunc("POST /api/v1/agent/claim", s.agent(s.claim))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "not_found", "unknown API endpoint") })
	static := http.FileServerFS(s.Assets)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			fail(w, 405, "method", "method not allowed")
			return
		}
		static.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" && !strings.HasPrefix(r.URL.Path, "/api/v1/agent/") {
			origin := r.Header.Get("Origin")
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			if origin == "" || (origin != scheme+"://"+r.Host && origin != s.Store.Config.PublicURL) {
				fail(w, 403, "origin", "same-origin request required")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func (s *Server) agent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.Store.Config.AgentToken)) != 1 || r.Header.Get("X-Project-ID") != s.Store.Config.ProjectID {
			fail(w, 401, "unauthorized", "invalid project credentials")
			return
		}
		next(w, r)
	}
}
func (s *Server) human(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("agentdesk_session")
		if err != nil || !s.Store.ValidSession(c.Value) {
			fail(w, 401, "unauthorized", "login required")
			return
		}
		next(w, r)
	}
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	h, err := LoadHuman(s.Dir)
	if err != nil || len(body.Token) != 64 || time.Now().After(h.ExpiresAt) || subtle.ConstantTimeCompare([]byte(body.Token), []byte(h.Token)) != 1 {
		fail(w, 401, "unauthorized", "invalid or expired login token")
		return
	}
	token, err := s.Store.NewSession()
	if err != nil {
		fail(w, 500, "storage", "cannot persist session")
		return
	}
	s.cookie(w, r, token, 7*24*3600)
	s.session(w, r)
}
func (s *Server) cookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	secure := r.TLS != nil || strings.HasPrefix(s.Store.Config.PublicURL, "https://")
	http.SetCookie(w, &http.Cookie{Name: "agentdesk_session", Value: token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	respond(w, 200, map[string]any{"project_id": s.Store.Config.ProjectID, "name": s.Store.Config.Name, "actor": "human:owner"})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("agentdesk_session")
	if err := s.Store.DeleteSession(c.Value); err != nil {
		fail(w, 500, "storage", "cannot revoke session")
		return
	}
	s.cookie(w, r, "", -1)
	respond(w, 200, map[string]bool{"logged_out": true})
}
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.List(r.URL.Query().Get("pending") == "true")
	result(w, v, err)
}
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.Get(r.PathValue("id"))
	result(w, v, err)
}
func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.History(r.PathValue("id"))
	result(w, v, err)
}
func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var d Draft
	if !decode(w, r, &d) {
		return
	}
	v, err := s.Store.Create(d, r.Header.Get("Idempotency-Key"))
	result(w, v, err)
}
func (s *Server) reply(w http.ResponseWriter, r *http.Request) {
	var m struct {
		Revision int    `json:"revision"`
		OptionID string `json:"option_id"`
		Text     string `json:"text"`
		Ack      bool   `json:"ack"`
	}
	if !decode(w, r, &m) {
		return
	}
	v, err := s.Store.Mutate(r.PathValue("id"), "reply", "human:owner", Mutation{Revision: m.Revision, OptionID: m.OptionID, Text: m.Text, Ack: m.Ack})
	result(w, v, err)
}
func (s *Server) deferIssue(w http.ResponseWriter, r *http.Request) {
	var m struct {
		Revision int  `json:"revision"`
		Deferred bool `json:"deferred"`
	}
	if !decode(w, r, &m) {
		return
	}
	v, err := s.Store.Mutate(r.PathValue("id"), "deferred", "human:owner", Mutation{Revision: m.Revision, Deferred: m.Deferred})
	result(w, v, err)
}
func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	var m struct {
		Revision int   `json:"revision"`
		Draft    Draft `json:"draft"`
	}
	if !decode(w, r, &m) {
		return
	}
	v, err := s.Store.Mutate(r.PathValue("id"), "updated", "agent", Mutation{Revision: m.Revision, Draft: &m.Draft})
	result(w, v, err)
}
func (s *Server) withdraw(w http.ResponseWriter, r *http.Request) {
	var m struct {
		Revision int `json:"revision"`
	}
	if !decode(w, r, &m) {
		return
	}
	v, err := s.Store.Mutate(r.PathValue("id"), "withdrawn", "agent", Mutation{Revision: m.Revision})
	result(w, v, err)
}
func consumer(r *http.Request) string {
	c := r.URL.Query().Get("consumer")
	if c == "" {
		c = "coordinator"
	}
	return c
}
func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.Events(consumer(r), 0, true)
	result(w, v, err)
}
func (s *Server) wait(w http.ResponseWriter, r *http.Request) {
	timeout := 50 * time.Second
	if raw := r.URL.Query().Get("timeout"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < 0 || d > 55*time.Second {
			fail(w, 400, "timeout", "timeout must be between 0s and 55s")
			return
		}
		timeout = d
	}
	after, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if r.URL.Query().Get("after") == "" {
		after = 0
		err = nil
	}
	if err != nil || after < 0 {
		fail(w, 400, "cursor", "invalid after cursor")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	v, err := s.Store.Wait(ctx, consumer(r), r.URL.Query().Get("issue"), after)
	if errors.Is(err, context.DeadlineExceeded) {
		respond(w, 200, map[string]bool{"timed_out": true})
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	result(w, v, err)
}
func (s *Server) claim(w http.ResponseWriter, r *http.Request) {
	var m struct {
		Consumer string `json:"consumer"`
		Holder   string `json:"holder"`
		TTL      string `json:"ttl"`
		Release  bool   `json:"release"`
	}
	if !decode(w, r, &m) {
		return
	}
	if m.Consumer == "" {
		m.Consumer = "coordinator"
	}
	if m.TTL == "" {
		m.TTL = "60s"
	}
	ttl, err := time.ParseDuration(m.TTL)
	if err != nil {
		fail(w, 400, "ttl", "invalid ttl")
		return
	}
	v, err := s.Store.Claim(m.Consumer, m.Holder, ttl, m.Release)
	result(w, v, err)
}
func (s *Server) receipt(w http.ResponseWriter, r *http.Request) {
	var m struct {
		Consumer string `json:"consumer"`
		Holder   string `json:"holder"`
	}
	if !decode(w, r, &m) {
		return
	}
	if m.Consumer == "" {
		m.Consumer = "coordinator"
	}
	err := s.Store.Receipt(r.PathValue("id"), m.Consumer, m.Holder)
	result(w, map[string]bool{"received": err == nil}, err)
}
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("URL must be an http(s) origin without credentials, path, query, or fragment")
	}
	return nil
}
