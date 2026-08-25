package main

import "testing"

func TestParseCookieHeader(t *testing.T) {
	got := parseCookieHeader("auth=Fe26.2**abc; oc_locale=zh")
	if len(got) != 2 || got[0].Name != "auth" || got[0].Value != "Fe26.2**abc" || got[1].Name != "oc_locale" {
		t.Fatalf("%+v", got)
	}
}

func TestSkipMicrosoftHostCookie(t *testing.T) {
	if !skipCookie("__Host-MSAAUTH") {
		t.Fatal("microsoft host cookie must be skipped")
	}
	if !skipCookie("ESTSAUTH") {
		t.Fatal("microsoft session cookie must be skipped")
	}
	if skipCookie("auth") || skipCookie("oc_locale") {
		t.Fatal("cline cookies must be kept")
	}
}

func TestCookiePrefix(t *testing.T) {
	if cookiePrefix("__Host-MSAAUTH") != "host" {
		t.Fatal(cookiePrefix("__Host-MSAAUTH"))
	}
	if cookiePrefix("auth") != "" {
		t.Fatal("plain")
	}
}
