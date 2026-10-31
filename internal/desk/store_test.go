package desk

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	c, err := Init(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(dir, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return s, dir
}
func draft(kind string) Draft {
	return Draft{Kind: kind, Title: "Choose a deployment", Context: "Only deploy the selected task.", Options: nil}
}
func mustCreate(t *testing.T, s *Store, d Draft, key string) Issue {
	t.Helper()
	i, err := s.Create(d, key)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func TestCreateIdempotencyAndRevision(t *testing.T) {
	s, _ := testStore(t)
	d := draft("decision")
	d.Options = []Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}
	d.RecommendedOption = "b"
	i := mustCreate(t, s, d, "retry")
	again := mustCreate(t, s, d, "retry")
	if i.ID != again.ID {
		t.Fatal("retry created a second issue")
	}
	d.Title = "different"
	if _, err := s.Create(d, "retry"); !errors.Is(err, ErrConflict) {
		t.Fatalf("reused key with different payload: %v", err)
	}
	updated, err := s.Mutate(i.ID, "updated", "agent", Mutation{Revision: 1, Draft: &d})
	if err != nil || updated.Revision != 2 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err = s.Mutate(i.ID, "reply", "human:owner", Mutation{Revision: 1, OptionID: "b"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("old revision accepted: %v", err)
	}
	if _, err = s.Mutate(i.ID, "reply", "human:owner", Mutation{Revision: 2, OptionID: "unknown"}); err == nil {
		t.Fatal("unknown option accepted")
	}
	answer := Mutation{Revision: 2, Text: "My own choice: neither option"}
	answered, err := s.Mutate(i.ID, "reply", "human:owner", answer)
	if err != nil || answered.Reply.Text != answer.Text || answered.Reply.OptionID != "" {
		t.Fatalf("free answer changed: %+v %v", answered, err)
	}
	if _, err = s.Mutate(i.ID, "reply", "human:owner", answer); err != nil {
		t.Fatalf("identical reply retry: %v", err)
	}
	answer.Text = "different answer"
	if _, err = s.Mutate(i.ID, "reply", "human:owner", answer); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting reply: %v", err)
	}
	history, err := s.History(i.ID)
	if err != nil || len(history) != 3 {
		t.Fatalf("expected create, update, answer once: %+v %v", history, err)
	}
}

func TestAckDeferredWithdrawalAndNoAutomaticAnswer(t *testing.T) {
	s, _ := testStore(t)
	for _, kind := range []string{"action", "notice"} {
		i := mustCreate(t, s, draft(kind), "")
		deferred, err := s.Mutate(i.ID, "deferred", "human:owner", Mutation{Revision: 1, Deferred: true})
		if err != nil || deferred.Status != "pending" || deferred.Reply != nil {
			t.Fatal("deferral completed the issue")
		}
		if _, err = s.Mutate(i.ID, "reply", "human:owner", Mutation{Revision: 1}); err == nil {
			t.Fatal("implicit Ack accepted")
		}
		got, err := s.Mutate(i.ID, "reply", "human:owner", Mutation{Revision: 1, Ack: true, Text: "received, not completed"})
		if err != nil || got.Status != "acked" || got.Deferred {
			t.Fatalf("Ack: %+v %v", got, err)
		}
	}
	d := draft("decision")
	d.Options = []Option{{ID: "yes", Label: "Yes"}}
	d.RecommendedOption = "yes"
	i := mustCreate(t, s, d, "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if _, err := s.Wait(ctx, "coordinator", i.ID, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait timeout: %v", err)
	}
	got, _ := s.Get(i.ID)
	if got.Status != "pending" || got.Reply != nil {
		t.Fatal("timeout chose an answer")
	}
	got, err := s.Mutate(i.ID, "withdrawn", "agent", Mutation{Revision: 1})
	if err != nil || got.Status != "withdrawn" {
		t.Fatal(err)
	}
	if _, err = s.Mutate(i.ID, "reply", "human:owner", Mutation{Revision: 1, OptionID: "yes"}); !errors.Is(err, ErrConflict) {
		t.Fatal("withdrawn issue accepted answer")
	}
}

func TestPersistedInboxReceiptsAndLeases(t *testing.T) {
	s, dir := testStore(t)
	i := mustCreate(t, s, draft("decision"), "")
	if _, err := s.Mutate(i.ID, "reply", "human:owner", Mutation{Revision: 1, Text: "proceed"}); err != nil {
		t.Fatal(err)
	}
	events, err := s.Events("coordinator", 0, true)
	if err != nil || len(events) != 1 {
		t.Fatalf("inbox: %+v %v", events, err)
	}
	// Reading and process restart must not acknowledge delivery.
	s.DB.Close()
	reopened, err := OpenStore(dir, s.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	again, err := reopened.Events("coordinator", 0, true)
	if err != nil || len(again) != 1 || again[0].ID != events[0].ID {
		t.Fatal("event lost on restart")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err = reopened.Wait(ctx, "coordinator", "", 0); err != nil {
		t.Fatal("existing event did not wake wait")
	}
	if err = reopened.Receipt(events[0].ID, "coordinator", "session-a"); !errors.Is(err, ErrLease) {
		t.Fatal("receipt without lease accepted")
	}
	if _, err = reopened.Claim("coordinator", "session-a", time.Minute, false); err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.Claim("coordinator", "session-b", time.Minute, false); !errors.Is(err, ErrLease) {
		t.Fatal("overlapping coordinator accepted")
	}
	if err = reopened.Receipt(events[0].ID, "coordinator", "session-b"); !errors.Is(err, ErrLease) {
		t.Fatal("wrong holder accepted")
	}
	if err = reopened.Receipt(events[0].ID, "coordinator", "session-a"); err != nil {
		t.Fatal(err)
	}
	if err = reopened.Receipt(events[0].ID, "coordinator", "session-a"); err != nil {
		t.Fatal("receipt retry failed")
	}
	got, _ := reopened.Events("coordinator", 0, true)
	other, _ := reopened.Events("auditor", 0, true)
	if len(got) != 0 || len(other) != 1 {
		t.Fatal("receipt changed another consumer")
	}
	h, err := reopened.History(i.ID)
	if err != nil || len(h[1].Receipts) != 1 {
		t.Fatal("receipt absent from history")
	}
	reopened.DB.Exec("UPDATE leases SET expires=0 WHERE consumer='coordinator'")
	if _, err = reopened.Claim("coordinator", "session-b", time.Minute, false); err != nil {
		t.Fatal("expired lease did not permit recovery")
	}
}

func TestWaitWakesOnCommitAndCancellation(t *testing.T) {
	s, _ := testStore(t)
	i := mustCreate(t, s, draft("notice"), "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		v, err := s.Wait(ctx, "coordinator", "", 0)
		if err == nil && len(v.([]Event)) != 1 {
			err = errors.New("missing event")
		}
		done <- err
	}()
	if _, err := s.Mutate(i.ID, "reply", "human:owner", Mutation{Revision: 1, Ack: true}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := s.Wait(canceled, "other", "", 999); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestConcurrentClaimAndReply(t *testing.T) {
	s, _ := testStore(t)
	i := mustCreate(t, s, draft("decision"), "")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, text := range []string{"one", "two"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Mutate(i.ID, "reply", "human:owner", Mutation{Revision: 1, Text: text})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("conflicting answers both committed")
	}
}

func TestWorktreesShareProjectDiscovery(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	os.Mkdir(repo, 0700)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
		return string(out)
	}
	run("init")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial")
	worktree := filepath.Join(root, "worktree")
	run("worktree", "add", "-b", "other", worktree)
	t.Setenv("AGENTDESK_DATA_DIR", "")
	t.Chdir(repo)
	a, err := DataDir("")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(worktree)
	b, err := DataDir("")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("worktree split: %s %s", a, b)
	}
}

func TestOnlyOneServerPerProject(t *testing.T) {
	_, dir := testStore(t)
	unlock, err := LockServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if other, err := LockServer(dir); err == nil {
		other()
		t.Fatal("second server acquired the project")
	}
}
