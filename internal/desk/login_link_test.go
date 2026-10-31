package desk

import (
	"errors"
	"sync"
	"testing"
)

func TestLoginLinksAuthenticateOnceExpireAndRotate(t *testing.T) {
	f := newFixture(t)
	i := mustCreate(t, f.store, draft("notice"), "")
	token, err := f.store.NewLoginLink(i.ID)
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]string{"token": token}
	status, _, cookies := f.request(t, "POST", "/api/v1/session", body, false, f.server.URL)
	if status != 200 || len(cookies) != 1 || !f.store.ValidSession(cookies[0].Value) {
		t.Fatal("one-use token did not produce a session")
	}
	if status, _, _ = f.request(t, "POST", "/api/v1/session", body, false, f.server.URL); status != 401 {
		t.Fatal("used token authenticated twice")
	}
	f.cookie = cookies[0]
	if status, _, _ = f.request(t, "POST", "/api/v1/session", body, false, f.server.URL); status != 200 {
		t.Fatal("valid session could not reopen its notification link")
	}
	f.cookie = nil
	expired, err := f.store.NewLoginLink(i.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.store.DB.Exec("UPDATE login_links SET expires=0")
	if status, _, _ = f.request(t, "POST", "/api/v1/session", map[string]string{"token": expired}, false, f.server.URL); status != 401 {
		t.Fatal("expired token authenticated")
	}
	revoked, err := f.store.NewLoginLink(i.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.RotateHuman(f.dir); err != nil {
		t.Fatal(err)
	}
	if status, _, _ = f.request(t, "POST", "/api/v1/session", map[string]string{"token": revoked}, false, f.server.URL); status != 401 {
		t.Fatal("rotation left old notification tokens valid")
	}
	f.login(t)
}

func TestConcurrentLoginLinkRedemption(t *testing.T) {
	s, _ := testStore(t)
	i := mustCreate(t, s, draft("notice"), "")
	token, err := s.NewLoginLink(i.ID)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.RedeemLoginLink(token); results <- err }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("concurrent token replay created multiple sessions")
	}
}
