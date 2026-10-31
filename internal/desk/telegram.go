package desk

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TelegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

func LoadTelegram(dir, path string) (TelegramConfig, error) {
	var c TelegramConfig
	if path == "" {
		path = filepath.Join(dir, "telegram.json")
	}
	b, err := os.ReadFile(path)
	if err == nil {
		if json.Unmarshal(b, &c) != nil {
			return c, errors.New("invalid Telegram config JSON")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return c, errors.New("cannot read Telegram config")
	}
	if token := os.Getenv("AGENTDESK_TELEGRAM_BOT_TOKEN"); token != "" {
		c.BotToken = token
	}
	if chat := os.Getenv("AGENTDESK_TELEGRAM_CHAT_ID"); chat != "" {
		c.ChatID = chat
	}
	c.BotToken = strings.TrimSpace(c.BotToken)
	c.ChatID = strings.TrimSpace(c.ChatID)
	if c.BotToken == "" || c.ChatID == "" {
		return c, errors.New("Telegram requires bot_token and chat_id in telegram.json or AGENTDESK_TELEGRAM_BOT_TOKEN / AGENTDESK_TELEGRAM_CHAT_ID")
	}
	if strings.ContainsAny(c.BotToken, "/ ?#\r\n\t") || !strings.Contains(c.BotToken, ":") {
		return TelegramConfig{}, errors.New("invalid Telegram bot token format")
	}
	return c, nil
}

type Notification struct {
	IssueID     string `json:"issue_id"`
	State       string `json:"state"`
	Attempts    int    `json:"attempts"`
	NextAttempt int64  `json:"next_attempt_unix_ms"`
	MessageID   int64  `json:"message_id,omitempty"`
	LastError   string `json:"last_error,omitempty"`
}

func (s *Store) Notifications() ([]Notification, error) {
	rows, err := s.DB.Query("SELECT issue_id,state,attempts,next_attempt,message_id,last_error FROM telegram_notifications ORDER BY rowid DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		if err = rows.Scan(&n.IssueID, &n.State, &n.Attempts, &n.NextAttempt, &n.MessageID, &n.LastError); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

type TelegramSender struct {
	Store  *Store
	Config TelegramConfig
	Client *http.Client
	// APIBase is injectable for local tests; production always uses Telegram HTTPS.
	APIBase string
}

type sendResult struct {
	state, reason string
	messageID     int64
	retryAfter    time.Duration
}

func (t *TelegramSender) send(ctx context.Context, i Issue) sendResult {
	base := t.APIBase
	if base == "" {
		base = "https://api.telegram.org"
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return sendResult{state: "failed", reason: "invalid Telegram API endpoint"}
	}
	token, err := t.Store.NewLoginLink(i.ID)
	if err != nil {
		return sendResult{state: "pending", reason: "cannot persist notification login link; retry scheduled", retryAfter: time.Second}
	}
	link := t.Store.Config.PublicURL + "/?issue=" + url.QueryEscape(i.ID) + "#token=" + url.QueryEscape(token)
	text := fmt.Sprintf("%s\n[%s] %s\n%s", truncate(t.Store.Config.Name, 200), i.Kind, truncate(i.Title, 300), link)
	body, _ := json.Marshal(map[string]any{"chat_id": t.Config.ChatID, "text": text, "link_preview_options": map[string]bool{"is_disabled": true}})
	request, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(base, "/")+"/bot"+t.Config.BotToken+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return sendResult{state: "failed", reason: "cannot construct Telegram request"}
	}
	request.Header.Set("Content-Type", "application/json")
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	// Never follow redirects: the token appears in the Bot API path.
	safeClient := *client
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := safeClient.Do(request)
	if err != nil {
		return sendResult{state: "unknown", reason: "Telegram transport failed; delivery may have occurred, not automatically resent"}
	}
	defer res.Body.Close()
	var reply struct {
		OK        bool `json:"ok"`
		ErrorCode int  `json:"error_code"`
		Result    struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
		Parameters struct {
			RetryAfter int64 `json:"retry_after"`
		} `json:"parameters"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&reply); err != nil {
		return sendResult{state: "unknown", reason: "unrecognized Telegram response; delivery not confirmed"}
	}
	if res.StatusCode == 200 && reply.OK && reply.Result.MessageID > 0 {
		return sendResult{state: "sent", messageID: reply.Result.MessageID}
	}
	if !reply.OK && reply.ErrorCode > 0 {
		if reply.ErrorCode == 429 || reply.ErrorCode >= 500 {
			delay := time.Duration(min(max(reply.Parameters.RetryAfter, 0), 86400)) * time.Second
			return sendResult{state: "pending", reason: fmt.Sprintf("Telegram rejected request (%d); retry scheduled", reply.ErrorCode), retryAfter: delay}
		}
		return sendResult{state: "failed", reason: fmt.Sprintf("Telegram rejected request (%d)", reply.ErrorCode)}
	}
	return sendResult{state: "unknown", reason: "Telegram did not confirm message delivery"}
}

func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
}

// ProcessNext sends at most one queued message. sending is persisted before the
// HTTP call, so a crash after remote delivery cannot silently cause a resend.
func (t *TelegramSender) ProcessNext(ctx context.Context) (bool, time.Duration, error) {
	var id string
	var attempts int
	err := t.Store.DB.QueryRow("SELECT issue_id,attempts FROM telegram_notifications WHERE state='pending' AND next_attempt<=? ORDER BY rowid LIMIT 1", time.Now().UnixMilli()).Scan(&id, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		var next sql.NullInt64
		err = t.Store.DB.QueryRow("SELECT MIN(next_attempt) FROM telegram_notifications WHERE state='pending'").Scan(&next)
		if err != nil {
			return false, 5 * time.Second, err
		}
		if next.Valid {
			return false, max(time.Until(time.UnixMilli(next.Int64)), time.Millisecond), nil
		}
		return false, 24 * time.Hour, nil
	}
	if err != nil {
		return false, 5 * time.Second, err
	}
	res, err := t.Store.DB.Exec("UPDATE telegram_notifications SET state='sending',attempts=attempts+1 WHERE issue_id=? AND state='pending'", id)
	if err != nil {
		return false, 5 * time.Second, err
	}
	count, err := res.RowsAffected()
	if err != nil || count == 0 {
		return false, time.Second, err
	}
	i, err := t.Store.Get(id)
	if err != nil {
		return true, 0, t.finish(id, sendResult{state: "failed", reason: "cannot load queued issue"}, 0)
	}
	if i.Status != "pending" {
		return true, 0, t.finish(id, sendResult{state: "skipped", reason: "issue no longer pending"}, 0)
	}
	outcome := t.send(ctx, i)
	var next int64
	if outcome.state == "pending" {
		delay := max(outcome.retryAfter, time.Duration(1<<min(attempts, 9))*time.Second)
		next = time.Now().Add(delay).UnixMilli()
	}
	return true, 0, t.finish(id, outcome, next)
}
func (t *TelegramSender) finish(id string, r sendResult, next int64) error {
	_, err := t.Store.DB.Exec("UPDATE telegram_notifications SET state=?,next_attempt=?,message_id=?,last_error=? WHERE issue_id=?", r.state, next, r.messageID, r.reason, id)
	return err
}

func (t *TelegramSender) Run(ctx context.Context) {
	_, err := t.Store.DB.Exec("UPDATE telegram_notifications SET state='unknown',last_error='service stopped during send; delivery uncertain, not automatically resent' WHERE state='sending'")
	if err != nil {
		log.Print("Telegram: cannot recover notification queue")
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		changed := t.Store.Subscribe()
		processed, delay, err := t.ProcessNext(ctx)
		if err != nil {
			log.Print("Telegram: cannot update notification queue; retrying")
			delay = 5 * time.Second
		} else if processed {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}
