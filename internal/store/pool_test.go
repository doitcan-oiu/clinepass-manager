package store

import (
	"path/filepath"
	"testing"

	"opencode-go-manager/internal/model"
)

func TestMarkPaidAndManualAccount(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	b, _, err := s.CreateBatch(model.CreateBatchInput{
		Name: "批次-付款",
		Text: "pay@x.com----pw----b@x.com\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkExported(b.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPaid(b.ID); err != nil {
		t.Fatal(err)
	}
	n, err := s.CountPoolAccounts("")
	if err != nil || n != 0 {
		t.Fatalf("batch paid should not auto-add unpaid accounts, got %d %v", n, err)
	}
	list, err := s.ListByBatch(b.ID)
	if err != nil || len(list) != 1 {
		t.Fatal(err)
	}
	if err := s.SetAccountPaid(list[0].ID, true); err != nil {
		t.Fatal(err)
	}
	n, err = s.CountPoolAccounts("")
	if err != nil || n != 1 {
		t.Fatalf("pool count %d %v", n, err)
	}
	sum, err := s.GetBatchSummary(b.ID)
	if err != nil || sum.PaidCount != 1 {
		t.Fatalf("paid_count %+v %v", sum, err)
	}

	a, err := s.CreatePaidAccount(model.CreatePaidAccountInput{
		Email:        "manual@x.com",
		APIKey:       "sk-test",
		WorkspaceID:  "wrk_TEST",
		CookieHeader: "auth=abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.PaidAt == 0 || a.BatchName != "手动账号" {
		t.Fatalf("%+v", a)
	}
	if a.LoginProvider != model.LoginGoogle {
		t.Fatalf("default paid provider %q", a.LoginProvider)
	}
	ms, err := s.CreatePaidAccount(model.CreatePaidAccountInput{
		Email:         "ms@x.com",
		CookieHeader:  "auth=ms",
		LoginProvider: "microsoft",
	})
	if err != nil || ms.LoginProvider != model.LoginMicrosoft {
		t.Fatalf("ms paid %+v %v", ms, err)
	}

	cookieOnly, err := s.CreatePaidAccount(model.CreatePaidAccountInput{
		Email:        "cookie@x.com",
		CookieHeader: "auth=only",
	})
	if err != nil || cookieOnly.APIKey != "" || cookieOnly.PaidAt == 0 {
		t.Fatalf("cookie-only %+v %v", cookieOnly, err)
	}
	again, updated, err := s.UpsertPaidAccount(model.CreatePaidAccountInput{
		Email:        "cookie@x.com",
		CookieHeader: "auth=new",
		APIKey:       "sk-later",
		WorkspaceID:  "wrk_later",
	})
	if err != nil || !updated || again.CookieHeader != "auth=new" || again.APIKey != "sk-later" {
		t.Fatalf("upsert %+v updated=%v %v", again, updated, err)
	}
	n, err = s.CountBatches()
	if err != nil || n != 1 {
		t.Fatalf("extract page should hide 手动账号, count=%d %v", n, err)
	}
	page, err := s.ListBatchesPage(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range page {
		if b.Name == ManualBatchName {
			t.Fatal("手动账号 should not appear in extract list")
		}
	}
	n, err = s.CountPoolAccounts("")
	if err != nil || n != 4 {
		t.Fatalf("pool count after manual %d %v", n, err)
	}
	if err := s.SaveAccountUsage(a.ID, model.AccountUsage{
		Rolling:  model.UsageWindow{Status: "ok", UsagePercent: 12},
		SyncedAt: 1,
		Models:   []model.ModelSpend{{Model: "glm-5.3", USD: 1.5, LimitUSD: 15}},
	}); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetPoolAccount(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Usage.Rolling.UsagePercent != 12 || len(p.Usage.Models) != 1 {
		t.Fatalf("usage %+v", p.Usage)
	}

	now := int64(1_700_000_000)
	if err := s.SaveAccountUsage(a.ID, model.AccountUsage{
		Monthly:  model.UsageWindow{Status: "ok", ResetInSec: 10, UsagePercent: 82},
		SyncedAt: now - 20,
	}); err != nil {
		t.Fatal(err)
	}
	n, err = s.DeleteExpiredAccounts(now)
	if err != nil || n != 1 {
		t.Fatalf("deleted %d %v", n, err)
	}
	if _, err := s.GetPoolAccount(a.ID); err == nil {
		t.Fatal("expired account should be deleted")
	}

	list[0].PaymentURL = "https://pay.example/1"
	if err := s.SaveLoginResult(list[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccountPaid(list[0].ID, true); err != nil {
		t.Fatal(err)
	}
	links, err := s.UniquePaymentLinks(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("paid accounts should be skipped, got %d", len(links))
	}
	if err := s.SetAccountPaid(list[0].ID, false); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	got.PaymentURL = "https://pay.example/1"
	if err := s.SaveLoginResult(got); err != nil {
		t.Fatal(err)
	}
	links, err = s.UniquePaymentLinks(b.ID)
	if err != nil || len(links) != 1 {
		t.Fatalf("unpaid links %d %v", len(links), err)
	}
}
