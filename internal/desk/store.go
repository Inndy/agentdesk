package desk

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	DB              *sql.DB
	Config          Config
	TelegramEnabled bool
	mu              sync.Mutex
	changed         chan struct{}
}

func OpenStore(dir string, c Config) (*Store, error) {
	path, err := filepath.Abs(filepath.Join(dir, "agentdesk.db"))
	if err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	q := u.Query()
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, Config: c, changed: make(chan struct{})}
	_, err = db.Exec(`
 CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY,value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS issues (id TEXT PRIMARY KEY,body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS idempotency (key TEXT PRIMARY KEY,digest TEXT NOT NULL,issue_id TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS events (seq INTEGER PRIMARY KEY AUTOINCREMENT,id TEXT UNIQUE NOT NULL,issue_id TEXT NOT NULL,kind TEXT NOT NULL,body TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS receipts (event_id TEXT NOT NULL,consumer TEXT NOT NULL,holder TEXT NOT NULL,at TEXT NOT NULL,PRIMARY KEY(event_id,consumer));
 CREATE TABLE IF NOT EXISTS leases (consumer TEXT PRIMARY KEY,holder TEXT NOT NULL,expires INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS sessions (hash TEXT PRIMARY KEY,expires INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS login_links (hash TEXT PRIMARY KEY,issue_id TEXT NOT NULL,expires INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS telegram_notifications (issue_id TEXT PRIMARY KEY,state TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,next_attempt INTEGER NOT NULL DEFAULT 0,message_id INTEGER NOT NULL DEFAULT 0,last_error TEXT NOT NULL DEFAULT '');
 `)
	if err == nil {
		_, err = db.Exec("INSERT OR IGNORE INTO meta(key,value) VALUES('project_id',?)", c.ProjectID)
	}
	if err == nil {
		var id string
		err = db.QueryRow("SELECT value FROM meta WHERE key='project_id'").Scan(&id)
		if err == nil && id != c.ProjectID {
			err = errors.New("database belongs to another project")
		}
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Store) Subscribe() <-chan struct{} { s.mu.Lock(); defer s.mu.Unlock(); return s.changed }
func (s *Store) notify() {
	s.mu.Lock()
	defer s.mu.Unlock()
	close(s.changed)
	s.changed = make(chan struct{})
}
func (s *Store) Get(id string) (Issue, error) {
	var i Issue
	var b string
	err := s.DB.QueryRow("SELECT body FROM issues WHERE id=?", id).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return i, ErrNotFound
	}
	if err != nil {
		return i, err
	}
	err = json.Unmarshal([]byte(b), &i)
	return i, err
}
func (s *Store) List(pending bool) ([]Issue, error) {
	rows, err := s.DB.Query("SELECT body FROM issues ORDER BY rowid DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Issue{}
	for rows.Next() {
		var b string
		var i Issue
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(b), &i); err != nil {
			return nil, err
		}
		if !pending || i.Status == "pending" {
			out = append(out, i)
		}
	}
	return out, rows.Err()
}
func saveEvent(tx *sql.Tx, i Issue, kind, actor string) error {
	b, err := json.Marshal(i)
	if err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO issues(id,body) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", i.ID, string(b)); err != nil {
		return err
	}
	e := Event{ID: "E-" + RandomToken()[:24], IssueID: i.ID, Revision: i.Revision, Kind: kind, Actor: actor, At: now(), Issue: i}
	b, err = json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO events(id,issue_id,kind,body) VALUES(?,?,?,?)", e.ID, i.ID, kind, string(b))
	return err
}
func (s *Store) Create(d Draft, key string) (Issue, error) {
	var i Issue
	if err := d.Validate(); err != nil {
		return i, err
	}
	if len(key) > 200 {
		return i, errors.New("idempotency key too long")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return i, err
	}
	defer tx.Rollback()
	if key != "" {
		var oldDigest, id string
		err = tx.QueryRow("SELECT digest,issue_id FROM idempotency WHERE key=?", key).Scan(&oldDigest, &id)
		if err == nil {
			if oldDigest != digest(d) {
				return i, ErrConflict
			}
			var b string
			err = tx.QueryRow("SELECT body FROM issues WHERE id=?", id).Scan(&b)
			if err != nil {
				return i, err
			}
			err = json.Unmarshal([]byte(b), &i)
			return i, err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return i, err
		}
	}
	prefix := map[string]string{"decision": "D", "action": "A", "notice": "N"}[d.Kind]
	i = Issue{Draft: d, ProjectID: s.Config.ProjectID, ID: prefix + "-" + RandomToken()[:12], Revision: 1, Status: "pending", CreatedAt: now(), UpdatedAt: now()}
	i.WebURL = s.Config.PublicURL + "/?issue=" + i.ID
	if err = saveEvent(tx, i, "created", d.CreatedBy); err != nil {
		return i, err
	}
	if s.TelegramEnabled {
		if _, err = tx.Exec("INSERT INTO telegram_notifications(issue_id,state) VALUES(?,'pending')", i.ID); err != nil {
			return i, err
		}
	}
	if key != "" {
		if _, err = tx.Exec("INSERT INTO idempotency(key,digest,issue_id) VALUES(?,?,?)", key, digest(d), i.ID); err != nil {
			return i, err
		}
	}
	if err = tx.Commit(); err == nil {
		s.notify()
	}
	return i, err
}

type Mutation struct {
	Revision int    `json:"revision"`
	Draft    *Draft `json:"draft,omitempty"`
	OptionID string `json:"option_id,omitempty"`
	Text     string `json:"text,omitempty"`
	Ack      bool   `json:"ack,omitempty"`
	Deferred bool   `json:"deferred,omitempty"`
}

func (s *Store) Mutate(id, kind, actor string, m Mutation) (Issue, error) {
	var i Issue
	tx, err := s.DB.Begin()
	if err != nil {
		return i, err
	}
	defer tx.Rollback()
	var b string
	err = tx.QueryRow("SELECT body FROM issues WHERE id=?", id).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return i, ErrNotFound
	}
	if err != nil {
		return i, err
	}
	if err = json.Unmarshal([]byte(b), &i); err != nil {
		return i, err
	}
	if i.Revision != m.Revision {
		return i, ErrConflict
	}
	if kind == "deferred" && i.Status == "pending" && i.Deferred == m.Deferred {
		return i, nil
	}
	if kind == "reply" && i.Reply != nil && i.Reply.OptionID == m.OptionID && i.Reply.Text == m.Text && i.Reply.Ack == m.Ack {
		return i, nil
	}
	if i.Status != "pending" {
		return i, ErrConflict
	}
	switch kind {
	case "reply":
		if len(m.Text) > 100000 {
			return i, errors.New("reply too large")
		}
		if i.Kind == "decision" {
			if m.Ack || (m.OptionID == "" && lenTrim(m.Text) == 0) {
				return i, errors.New("choose an option or enter an answer")
			}
			if m.OptionID != "" {
				found := false
				for _, o := range i.Options {
					if o.ID == m.OptionID {
						found = true
					}
				}
				if !found {
					return i, errors.New("unknown option ID")
				}
			}
			i.Status = "answered"
		} else {
			if !m.Ack || m.OptionID != "" {
				return i, errors.New("explicit Ack is required")
			}
			i.Status = "acked"
		}
		i.Reply = &Reply{Revision: m.Revision, OptionID: m.OptionID, Text: m.Text, Ack: m.Ack, Actor: actor, At: now()}
		i.Deferred = false
		kind = i.Status
	case "updated":
		if m.Draft == nil {
			return i, errors.New("draft is required")
		}
		if err = m.Draft.Validate(); err != nil {
			return i, err
		}
		if m.Draft.Kind != i.Kind {
			return i, errors.New("issue kind cannot change")
		}
		i.Draft = *m.Draft
		i.Revision++
		i.Deferred = false
	case "withdrawn":
		i.Status = "withdrawn"
	case "deferred":
		i.Deferred = m.Deferred
	default:
		return i, errors.New("unknown mutation")
	}
	i.UpdatedAt = now()
	if err = saveEvent(tx, i, kind, actor); err != nil {
		return i, err
	}
	if err = tx.Commit(); err == nil {
		s.notify()
	}
	return i, err
}
func lenTrim(v string) int {
	n := 0
	for _, r := range v {
		if r != ' ' && r != '\t' && r != '\r' && r != '\n' {
			n++
		}
	}
	return n
}
func (s *Store) Events(consumer string, after int64, inbox bool) ([]Event, error) {
	query := "SELECT e.seq,e.body FROM events e WHERE e.seq>?"
	args := []any{after}
	if inbox {
		query += " AND e.kind IN ('answered','acked','updated','withdrawn') AND NOT EXISTS (SELECT 1 FROM receipts r WHERE r.event_id=e.id AND r.consumer=?)"
		args = append(args, consumer)
	}
	query += " ORDER BY e.seq LIMIT 200"
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var seq int64
		var b string
		var e Event
		if err = rows.Scan(&seq, &b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(b), &e); err != nil {
			return nil, err
		}
		e.Seq = seq
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *Store) Wait(ctx context.Context, consumer, id string, after int64) (any, error) {
	for {
		ch := s.Subscribe()
		if id != "" {
			i, err := s.Get(id)
			if err != nil {
				return nil, err
			}
			if i.Status != "pending" {
				return i, nil
			}
		} else {
			e, err := s.Events(consumer, after, true)
			if err != nil {
				return nil, err
			}
			if len(e) > 0 {
				return e, nil
			}
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
func (s *Store) History(id string) ([]History, error) {
	if _, err := s.Get(id); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query("SELECT seq,body FROM events WHERE issue_id=? ORDER BY seq", id)
	if err != nil {
		return nil, err
	}
	out := []History{}
	for rows.Next() {
		var h History
		var b string
		var seq int64
		if err = rows.Scan(&seq, &b); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal([]byte(b), &h.Event); err != nil {
			rows.Close()
			return nil, err
		}
		h.Seq = seq
		h.Receipts = []Receipt{}
		out = append(out, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for n := range out {
		r, err := s.DB.Query("SELECT consumer,holder,at FROM receipts WHERE event_id=? ORDER BY at", out[n].ID)
		if err != nil {
			return nil, err
		}
		for r.Next() {
			var receipt Receipt
			if err = r.Scan(&receipt.Consumer, &receipt.Holder, &receipt.At); err != nil {
				r.Close()
				return nil, err
			}
			out[n].Receipts = append(out[n].Receipts, receipt)
		}
		err = r.Err()
		r.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
func (s *Store) Claim(consumer, holder string, ttl time.Duration, release bool) (Lease, error) {
	var l Lease
	if consumer == "" || holder == "" || len(consumer) > 200 || len(holder) > 200 {
		return l, errors.New("consumer and holder are required (max 200 bytes)")
	}
	if ttl < time.Second || ttl > 10*time.Minute {
		return l, errors.New("lease ttl must be between 1s and 10m")
	}
	expires := time.Now().Add(ttl).UnixMilli()
	if release {
		_, err := s.DB.Exec("DELETE FROM leases WHERE consumer=? AND holder=?", consumer, holder)
		return Lease{Consumer: consumer, Holder: holder}, err
	}
	res, err := s.DB.Exec(`INSERT INTO leases(consumer,holder,expires) VALUES(?,?,?) ON CONFLICT(consumer) DO UPDATE SET holder=excluded.holder,expires=excluded.expires WHERE leases.expires<=? OR leases.holder=excluded.holder`, consumer, holder, expires, time.Now().UnixMilli())
	if err != nil {
		return l, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return l, err
	}
	if n == 0 {
		return l, ErrLease
	}
	return Lease{Consumer: consumer, Holder: holder, ExpiresAt: time.UnixMilli(expires).UTC().Format(time.RFC3339Nano)}, nil
}
func (s *Store) Receipt(id, consumer, holder string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var h string
	var expires int64
	err = tx.QueryRow("SELECT holder,expires FROM leases WHERE consumer=?", consumer).Scan(&h, &expires)
	if errors.Is(err, sql.ErrNoRows) || h != holder || expires <= time.Now().UnixMilli() {
		return ErrLease
	}
	if err != nil {
		return err
	}
	var exists int
	err = tx.QueryRow("SELECT 1 FROM events WHERE id=? AND kind IN ('answered','acked','updated','withdrawn')", id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT OR IGNORE INTO receipts(event_id,consumer,holder,at) VALUES(?,?,?,?)", id, consumer, holder, now())
	if err != nil {
		return err
	}
	if err = tx.Commit(); err == nil {
		s.notify()
	}
	return err
}
func (s *Store) NewSession() (string, error) {
	token := RandomToken()
	tx, err := s.DB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM sessions WHERE expires<=?", time.Now().Unix()); err != nil {
		return "", err
	}
	_, err = tx.Exec("INSERT INTO sessions(hash,expires) VALUES(?,?)", digest(token), time.Now().Add(7*24*time.Hour).Unix())
	if err != nil {
		return "", err
	}
	return token, tx.Commit()
}
func (s *Store) ValidSession(token string) bool {
	if len(token) != 64 {
		return false
	}
	var expires int64
	err := s.DB.QueryRow("SELECT expires FROM sessions WHERE hash=?", digest(token)).Scan(&expires)
	return err == nil && expires > time.Now().Unix()
}
func (s *Store) DeleteSession(token string) error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE hash=?", digest(token))
	return err
}
func (s *Store) RotateHuman(dir string) error {
	if _, err := s.DB.Exec("DELETE FROM sessions"); err != nil {
		return err
	}
	if _, err := s.DB.Exec("DELETE FROM login_links"); err != nil {
		return err
	}
	return WriteJSON(filepath.Join(dir, "human.json"), HumanCredential{Token: RandomToken(), ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour)})
}
func (s *Store) String() string { return fmt.Sprintf("project %s", s.Config.ProjectID) }
