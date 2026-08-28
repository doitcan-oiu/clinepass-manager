package job

import (
	"path/filepath"
	"testing"
	"time"

	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

func TestShouldRunCookieKeep(t *testing.T) {
	now := time.Date(2026, 8, 27, 16, 0, 0, 0, time.Local)
	if ShouldRunCookieKeep(now, false, 4, "") {
		t.Fatal("disabled")
	}
	if ShouldRunCookieKeep(time.Date(2026, 8, 27, 3, 0, 0, 0, time.Local), true, 4, "") {
		t.Fatal("before hour")
	}
	if !ShouldRunCookieKeep(now, true, 4, "") {
		t.Fatal("should catch up after hour")
	}
	if ShouldRunCookieKeep(now, true, 4, "2026-08-27") {
		t.Fatal("already ran today")
	}
	if !ShouldRunCookieKeep(now, true, 4, "2026-08-26") {
		t.Fatal("new day")
	}
}

func TestEnqueueCookieSharesQueue(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	paid, err := st.CreatePaidAccount(model.CreatePaidAccountInput{
		Email:        "keep@x.com",
		CookieHeader: "cline_session_id=abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	m := New(config.Config{MaxConcurrent: 1}, st)
	m.HoldPump()
	if _, err := m.Enqueue(paid.ID, false); err == nil {
		t.Fatal("paid login should skip")
	}
	j, err := m.EnqueueCookie(paid.ID)
	if err != nil || j.Kind != KindCookie {
		t.Fatalf("%+v %v", j, err)
	}
	if _, err := m.EnqueueCookie(paid.ID); err == nil {
		t.Fatal("duplicate cookie job")
	}
	jobs, err := m.EnqueueCookieKeep()
	if err != nil || len(jobs) != 0 {
		t.Fatalf("busy keep %d %v", len(jobs), err)
	}
}
