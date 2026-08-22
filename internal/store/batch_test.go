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
	if list[0].LoginProvider != model.LoginGoogle {
		t.Fatalf("default provider %q", list[0].LoginProvider)
	}
}

func TestCreateBatchMicrosoftProvider(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	b, errors, err := s.CreateBatch(model.CreateBatchInput{
		Name:          "微软批次",
		Text:          "a@outlook.com----pw\nb@outlook.com----pw2\n",
		LoginProvider: "microsoft",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(errors) != 0 {
		t.Fatalf("unexpected errors: %v", errors)
	}
	if b.Total != 2 {
		t.Fatalf("summary %+v", b)
	}
	list, err := s.ListByBatch(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d accounts", len(list))
	}
	for _, a := range list {
		if a.LoginProvider != model.LoginMicrosoft {
			t.Fatalf("%s provider=%q", a.Email, a.LoginProvider)
		}
		if a.RecoveryEmail != "" {
			t.Fatalf("%s should have no recovery, got %q", a.Email, a.RecoveryEmail)
		}
	}
}

func TestDeleteRadarDenied(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	b, _, err := s.CreateBatch(model.CreateBatchInput{
		Name: "雷达批次",
		Text: "radar@x.com----pw\nbanned@x.com----pw\nok@x.com----pw\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListByBatch(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	byEmail := map[string]string{}
	for _, a := range list {
		byEmail[a.Email] = a.ID
	}
	if err := s.UpdateStatus(byEmail["radar@x.com"], "failed", "AuthKit Radar 拦截（policy_denied），已跳过"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateStatus(byEmail["banned@x.com"], "failed", "账号已被封禁，已跳过"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateStatus(byEmail["ok@x.com"], "ready", ""); err != nil {
		t.Fatal(err)
	}

	n, err := s.DeleteRadarDenied(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted %d", n)
	}
	left, err := s.ListByBatch(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 2 {
		t.Fatalf("left %d", len(left))
	}
	for _, a := range left {
		if a.Email == "radar@x.com" {
			t.Fatal("radar account should be gone")
		}
	}
	if _, err := s.DeleteRadarDenied("missing"); err == nil {
		t.Fatal("expected missing batch error")
	}
}
