package store

import (
	"path/filepath"
	"testing"

	"opencode-go-manager/internal/model"
)

func TestCreateBatchAndList(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if _, _, err := s.CreateBatch(model.CreateBatchInput{Text: ""}); err == nil {
		t.Fatal("expected empty text error")
	}

	b, errors, err := s.CreateBatch(model.CreateBatchInput{
		Name: "批次-测试",
		Text: "a@x.com----pw----b@x.com\nb@x.com----pw2----c@x.com\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(errors) != 0 {
		t.Fatalf("unexpected errors: %v", errors)
	}
	if b.Total != 2 || b.Pending != 2 {
		t.Fatalf("summary %+v", b)
	}
	page, err := s.ListBatchesPage(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 {
		t.Fatalf("page %d", len(page))
	}
	n, err := s.CountBatches()
	if err != nil || n != 1 {
		t.Fatalf("count %d %v", n, err)
	}
	list, err := s.ListByBatch(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d accounts", len(list))
	}
	if list[0].BatchName != "批次-测试" {
		t.Fatalf("batch name %q", list[0].BatchName)
	}
	links, err := s.UniquePaymentLinks(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("expected no pay links, got %d", len(links))
	}
}

func TestCreateBatchRejectsBlacklistedSuffix(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	st, err := s.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	st.EmailSuffixBlacklist = []string{"foxcroftp.us"}
	if err := s.SaveSettings(st); err != nil {
		t.Fatal(err)
	}
	_, blocked, err := s.CreateBatch(model.CreateBatchInput{
		Text: "ok@jasperway.us----pw----b@x.com\nbad@foxcroftp.us----pw----c@x.com\n",
	})
	if err == nil {
		t.Fatal("expected blacklist error")
	}
	if len(blocked) != 1 || blocked[0] != "bad@foxcroftp.us" {
		t.Fatalf("blocked=%v err=%v", blocked, err)
	}
}
