package usage

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

func TestStartAllRejectsOverlap(t *testing.T) {
	st := openStore(t)
	mustPaid(t, st, "a@x.com")
	s := NewSyncer(st)
	started := make(chan struct{})
	s.fetch = func(model.Account, string) (model.AccountUsage, bool, error) {
		close(started)
		time.Sleep(80 * time.Millisecond)
		return okUsage(10), true, nil
	}
	if err := s.StartAll(); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := s.StartAll(); err == nil {
		t.Fatal("overlap should be rejected")
	}
}

func TestRefreshCoalescesConcurrentCalls(t *testing.T) {
	st := openStore(t)
	a := mustPaid(t, st, "a@x.com")
	s := NewSyncer(st)
	var n atomic.Int32
	s.fetch = func(model.Account, string) (model.AccountUsage, bool, error) {
		n.Add(1)
		time.Sleep(50 * time.Millisecond)
		return okUsage(40), true, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Refresh(a.ID)
		}()
	}
	wg.Wait()
	if n.Load() != 1 {
		t.Fatalf("fetches=%d want 1", n.Load())
	}
	u, err := st.GetAccountUsage(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if u.Rolling.UsagePercent != 40 {
		t.Fatalf("rolling=%v", u.Rolling)
	}
}

func TestFetchErrorKeepsPreviousUsage(t *testing.T) {
	st := openStore(t)
	a := mustPaid(t, st, "a@x.com")
	if err := st.SaveAccountUsage(a.ID, okUsage(33)); err != nil {
		t.Fatal(err)
	}
	s := NewSyncer(st)
	s.fetch = func(model.Account, string) (model.AccountUsage, bool, error) {
		return model.AccountUsage{}, false, fmt.Errorf("network down")
	}
	_, err := s.One(a.ID)
	if err == nil {
		t.Fatal("expected fetch error")
	}
	u, err := st.GetAccountUsage(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if u.Rolling.UsagePercent != 33 {
		t.Fatalf("previous usage wiped: %+v", u.Rolling)
	}
	if u.Error != "network down" {
		t.Fatalf("error=%q", u.Error)
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func mustPaid(t *testing.T, st *store.Store, email string) model.Account {
	t.Helper()
	a, err := st.CreatePaidAccount(model.CreatePaidAccountInput{
		Email:        email,
		APIKey:       "sk",
		WorkspaceID:  "ws",
		UserID:       "usr-1",
		CookieHeader: "auth=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func okUsage(rolling float64) model.AccountUsage {
	return model.AccountUsage{
		SyncedAt: time.Now().Unix(),
		Rolling:  model.UsageWindow{Status: "ok", UsagePercent: rolling, ResetInSec: 300},
		Weekly:   model.UsageWindow{Status: "ok", UsagePercent: 10},
		Monthly:  model.UsageWindow{Status: "ok", UsagePercent: 10},
	}
}
