package export

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"opencode-go-manager/internal/model"
)

func TestPayLinksXLSXContainsAccountPasswordCookieAndURL(t *testing.T) {
	raw, err := PayLinksXLSX([]model.PayLink{
		{Email: "a@x.com", Password: "p&w<1>", Cookie: "auth=Fe26.2**abc; oc_locale=zh", URL: "https://pay.example/1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var sheet string
	for _, f := range zr.File {
		if f.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		buf := &bytes.Buffer{}
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		_ = rc.Close()
		sheet = buf.String()
	}
	if sheet == "" {
		t.Fatal("missing sheet")
	}
	for _, want := range []string{"账号", "密码", "Cookie", "支付链接", "a@x.com", "p&amp;w&lt;1&gt;", "auth=Fe26.2**abc; oc_locale=zh", "https://pay.example/1"} {
		if !strings.Contains(sheet, want) {
			t.Fatalf("sheet missing %q\n%s", want, sheet)
		}
	}
}
