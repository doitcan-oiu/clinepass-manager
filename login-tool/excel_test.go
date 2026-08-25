package main

import (
	"os"
	"path/filepath"
	"testing"

	"opencode-go-manager/internal/export"
	"opencode-go-manager/internal/model"
)

func TestLoadPayRowsFromManagerExport(t *testing.T) {
	raw, err := export.PayLinksXLSX([]model.PayLink{
		{Email: "a@x.com", Password: "p1", Cookie: "auth=1; oc_locale=zh", URL: "https://pay.example/1"},
		{Email: "b@x.com", Password: "p2", Cookie: "auth=2", URL: "https://pay.example/2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "支付链接.xlsx")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := findExcel(dir)
	if err != nil || found != path {
		t.Fatalf("find=%s err=%v", found, err)
	}
	rows, err := loadPayRows(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Email != "a@x.com" || rows[0].Cookie != "auth=1; oc_locale=zh" || rows[1].URL != "https://pay.example/2" {
		t.Fatalf("%+v", rows)
	}
	if n := len(readyRows(rows)); n != 2 {
		t.Fatalf("ready=%d", n)
	}
}
