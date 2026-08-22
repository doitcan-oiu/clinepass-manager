package backup

import (
	"encoding/json"
	"testing"

	"opencode-go-manager/internal/model"
)

func TestParseMonitorBackup(t *testing.T) {
	raw := []byte(`{
  "version": 1,
  "accounts": [
    {
      "account": "A@X.com",
      "password": "pw",
      "auxEmail": "b@x.com",
      "workspaceID": "wrk_1",
      "auth": "auth=Fe26.2**abc; oc_locale=zh",
      "apiKey": "sk-test"
    },
    {
      "account": "only@x.com",
      "auth": "Fe26.2**cookie-only"
    },
    {
      "account": "bad",
      "auth": "auth=x"
    }
  ]
}`)
	got, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items=%d skipped=%v", len(got.Items), got.Skipped)
	}
	if got.Items[0].Email != "a@x.com" || got.Items[0].CookieHeader != "auth=Fe26.2**abc; oc_locale=zh" {
		t.Fatalf("%+v", got.Items[0])
	}
	if got.Items[1].Email != "only@x.com" || got.Items[1].CookieHeader != "auth=Fe26.2**cookie-only" {
		t.Fatalf("%+v", got.Items[1])
	}
	if got.Items[1].APIKey != "" || got.Items[1].WorkspaceID != "" {
		t.Fatalf("cookie-only should leave key/ws empty")
	}
}

func TestParseArrayAndExportRoundtrip(t *testing.T) {
	got, err := Parse([]byte(`[{"account":"c@x.com","auth":"auth=abc"}]`))
	if err != nil || len(got.Items) != 1 || got.Items[0].Email != "c@x.com" {
		t.Fatalf("%+v %v", got, err)
	}
	bin, err := json.Marshal(Export([]model.Account{{
		Email: "c@x.com", Password: "p", CookieHeader: "auth=abc", APIKey: "sk-1", WorkspaceID: "wrk_1",
	}}))
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(bin)
	if err != nil || again.Items[0].Password != "p" || again.Items[0].APIKey != "sk-1" {
		t.Fatalf("%+v %v", again, err)
	}
	if again.Items[0].LoginProvider != model.LoginGoogle {
		t.Fatalf("default login=%q", again.Items[0].LoginProvider)
	}
}

func TestParseExportLoginType(t *testing.T) {
	got, err := Parse([]byte(`[{"account":"ms@x.com","auth":"auth=abc","loginType":"microsoft"}]`))
	if err != nil || len(got.Items) != 1 || got.Items[0].LoginProvider != model.LoginMicrosoft {
		t.Fatalf("%+v %v", got, err)
	}
	legacy, err := Parse([]byte(`[{"account":"g@x.com","auth":"auth=abc","login_provider":"google"}]`))
	if err != nil || legacy.Items[0].LoginProvider != model.LoginGoogle {
		t.Fatalf("%+v %v", legacy, err)
	}
	bin, err := json.Marshal(Export([]model.Account{{
		Email: "ms@x.com", CookieHeader: "auth=abc", LoginProvider: "outlook",
	}}))
	if err != nil {
		t.Fatal(err)
	}
	var file File
	if err := json.Unmarshal(bin, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Accounts) != 1 || file.Accounts[0].LoginType != model.LoginMicrosoft {
		t.Fatalf("%+v", file.Accounts)
	}
	round, err := Parse(bin)
	if err != nil || round.Items[0].LoginProvider != model.LoginMicrosoft {
		t.Fatalf("%+v %v", round, err)
	}
}
