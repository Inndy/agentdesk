package desk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func notificationState(t *testing.T, s *Store, id string) Notification {
	t.Helper()
	all, err := s.Notifications()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range all {
		if n.IssueID == id {
			return n
		}
	}
	t.Fatal("notification missing")
	return Notification{}
}

func TestTelegramOneMessageNoSecretsAndIdempotentCreate(t *testing.T) {
	s, dir := testStore(t)
	human, err := LoadHuman(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.TelegramEnabled = true
	s.Config.PublicURL = "http://192.168.10.8:7331"
	d := draft("decision")
	d.Title = "Choose <A> or B"
	d.Context = "private context must not leave project"
	i := mustCreate(t, s, d, "same")
	again := mustCreate(t, s, d, "same")
	if i.ID != again.ID {
		t.Fatal("duplicate issue")
	}
	var calls atomic.Int32
	var linkToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.Path != "/bot123:test/sendMessage" {
			t.Error("unexpected Telegram method or API path")
		}
		var body struct {
			ChatID      string `json:"chat_id"`
			Text        string `json:"text"`
			LinkPreview struct {
				Disabled bool `json:"is_disabled"`
			} `json:"link_preview_options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.ChatID != "test-chat" || !body.LinkPreview.Disabled || !strings.Contains(body.Text, s.Config.PublicURL+"/?issue="+i.ID) {
			t.Error("notification omitted issue link")
		}
		lines := strings.Split(body.Text, "\n")
		link, err := url.Parse(lines[len(lines)-1])
		if err != nil {
			t.Error("invalid login link")
		} else {
			fragment, _ := url.ParseQuery(link.Fragment)
			linkToken = fragment.Get("token")
			if len(linkToken) != 64 {
				t.Error("missing one-use login token")
			}
		}
		for _, secret := range []string{s.Config.AgentToken, d.Context, human.Token} {
			if strings.Contains(body.Text, secret) {
				t.Error("notification leaked a secret or full context")
			}
		}
		io.WriteString(w, `{"ok":true,"result":{"message_id":1234}}`)
	}))
	defer server.Close()
	sender := TelegramSender{Store: s, Config: TelegramConfig{BotToken: "123:test", ChatID: "test-chat"}, APIBase: server.URL, Client: server.Client()}
	if _, _, err := sender.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := notificationState(t, s, i.ID); n.State != "sent" || n.MessageID != 1234 || n.Attempts != 1 {
		t.Fatalf("unexpected delivery: %+v", n)
	}
	if processed, _, err := sender.ProcessNext(context.Background()); err != nil || processed || calls.Load() != 1 {
		t.Fatal("message resent after success")
	}
	session, err := s.RedeemLoginLink(linkToken)
	if err != nil || !s.ValidSession(session) {
		t.Fatal("delivered login link did not authenticate")
	}
	if _, err = s.RedeemLoginLink(linkToken); !errors.Is(err, ErrNotFound) {
		t.Fatal("one-use link was reusable")
	}
	notifications, _ := s.Notifications()
	raw, _ := json.Marshal(notifications)
	if strings.Contains(string(raw), linkToken) {
		t.Fatal("agent-visible notification state leaked login token")
	}
}

func TestTelegramRetryAfterAndPermanentFailure(t *testing.T) {
	s, _ := testStore(t)
	s.TelegramEnabled = true
	i := mustCreate(t, s, draft("notice"), "")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(429)
			io.WriteString(w, `{"ok":false,"error_code":429,"parameters":{"retry_after":30},"description":"secret content must not be logged"}`)
		} else {
			w.WriteHeader(403)
			io.WriteString(w, `{"ok":false,"error_code":403,"description":"bot blocked by user"}`)
		}
	}))
	defer server.Close()
	sender := TelegramSender{Store: s, Config: TelegramConfig{BotToken: "123:test", ChatID: "test-chat"}, APIBase: server.URL, Client: server.Client()}
	sender.ProcessNext(context.Background())
	n := notificationState(t, s, i.ID)
	if n.State != "pending" || n.NextAttempt < time.Now().Add(28*time.Second).UnixMilli() {
		t.Fatalf("retry_after not respected: %+v", n)
	}
	if processed, _, _ := sender.ProcessNext(context.Background()); processed {
		t.Fatal("rate limit retried immediately")
	}
	s.DB.Exec("UPDATE telegram_notifications SET next_attempt=0")
	sender.ProcessNext(context.Background())
	n = notificationState(t, s, i.ID)
	if n.State != "failed" || n.Attempts != 2 || strings.Contains(n.LastError, "bot blocked") {
		t.Fatalf("permanent failure not recorded safely: %+v", n)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("secret transport detail")
}

func TestTelegramUncertainDeliveryAndCrashRecoveryDoNotResend(t *testing.T) {
	s, dir := testStore(t)
	s.TelegramEnabled = true
	i := mustCreate(t, s, draft("action"), "")
	sender := TelegramSender{Store: s, Config: TelegramConfig{BotToken: "123:secret", ChatID: "test-chat"}, Client: &http.Client{Transport: failingTransport{}}}
	sender.ProcessNext(context.Background())
	n := notificationState(t, s, i.ID)
	if n.State != "unknown" || strings.Contains(n.LastError, "secret") {
		t.Fatal("uncertain transport error was not redacted")
	}
	i2 := mustCreate(t, s, draft("notice"), "")
	s.DB.Exec("UPDATE telegram_notifications SET state='sending' WHERE issue_id=?", i2.ID)
	s.DB.Close()
	reopened, err := OpenStore(dir, s.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	sender.Store = reopened
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sender.Run(ctx)
	if n = notificationState(t, reopened, i2.ID); n.State != "unknown" {
		t.Fatal("interrupted sending row became resendable")
	}
	if processed, _, _ := sender.ProcessNext(context.Background()); processed {
		t.Fatal("uncertain message was resent")
	}
}

func TestTelegramQueuePersistsAndSkipsResolvedIssues(t *testing.T) {
	s, dir := testStore(t)
	without := mustCreate(t, s, draft("notice"), "")
	all, _ := s.Notifications()
	if len(all) != 0 {
		t.Fatal("Telegram was enabled without opt-in")
	}
	s.TelegramEnabled = true
	i := mustCreate(t, s, draft("notice"), "")
	s.Mutate(i.ID, "reply", "human:owner", Mutation{Revision: 1, Ack: true})
	s.DB.Close()
	reopened, err := OpenStore(dir, s.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	sender := TelegramSender{Store: reopened}
	if _, _, err = sender.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if notificationState(t, reopened, i.ID).State != "skipped" {
		t.Fatal("resolved issue sent")
	}
	if _, err = reopened.Get(without.ID); err != nil {
		t.Fatal("non-notified issue lost")
	}
}

func TestTelegramConfiguration(t *testing.T) {
	t.Setenv("AGENTDESK_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("AGENTDESK_TELEGRAM_CHAT_ID", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "telegram.json")
	if err := WriteJSON(path, TelegramConfig{BotToken: "123:local", ChatID: "local-chat"}); err != nil {
		t.Fatal(err)
	}
	c, err := LoadTelegram(dir, "")
	if err != nil || c.ChatID != "local-chat" {
		t.Fatal("local Telegram config failed")
	}
	t.Setenv("AGENTDESK_TELEGRAM_CHAT_ID", "override-chat")
	c, err = LoadTelegram(dir, "")
	if err != nil || c.ChatID != "override-chat" {
		t.Fatal("environment override failed")
	}
	os.WriteFile(path, []byte("invalid json"), 0600)
	if _, err = LoadTelegram(dir, ""); err == nil {
		t.Fatal("malformed config accepted")
	}
	os.Remove(path)
	if _, err = LoadTelegram(dir, ""); err == nil {
		t.Fatal("incomplete credentials accepted")
	}
}
