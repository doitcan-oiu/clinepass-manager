package browser

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseReleaseTag(t *testing.T) {
	ver, pro, ok := parseReleaseTag("chromium-v151.0.7922.108.2-pro")
	if !ok || !pro || ver != "151.0.7922.108.2" {
		t.Fatalf("%s %v %v", ver, pro, ok)
	}
	ver, pro, ok = parseReleaseTag("chromium-v146.0.7680.177.5")
	if !ok || pro || ver != "146.0.7680.177.5" {
		t.Fatalf("%s %v %v", ver, pro, ok)
	}
	if _, _, ok := parseReleaseTag("v0.5.8"); ok {
		t.Fatal("wrapper tag should be ignored")
	}
}

func TestCompareVersion(t *testing.T) {
	if CompareVersion("151.0.7922.108.2", "150.0.7871.114.6") <= 0 {
		t.Fatal("151 should be newer")
	}
	if CompareVersion("151.0.7922.108.2", "151.0.7922.108.2") != 0 {
		t.Fatal("same")
	}
	if CompareVersion("151.0.7922.108.1", "151.0.7922.108.2") >= 0 {
		t.Fatal("patch should lose")
	}
}

func TestPickLatestProVersion(t *testing.T) {
	got, ok := pickLatestProVersion([]githubRelease{
		{TagName: "chromium-v150.0.7871.114.6-pro"},
		{TagName: "chromium-v151.0.7922.108.2-pro"},
		{TagName: "chromium-v146.0.7680.177.5"},
		{TagName: "chromium-v151.0.7922.108.3-pro", Prerelease: true},
		{TagName: "v0.5.8"},
	})
	if !ok || got != "151.0.7922.108.2" {
		t.Fatalf("got %q", got)
	}
}

func TestLatestStableVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"tag_name":"chromium-v150.0.7871.114.6-pro","draft":false,"prerelease":false},
			{"tag_name":"chromium-v151.0.7922.108.2-pro","draft":false,"prerelease":false}
		]`))
	}))
	t.Cleanup(srv.Close)
	old := githubReleasesURL
	githubReleasesURL = srv.URL
	t.Cleanup(func() { githubReleasesURL = old })
	got, err := LatestStableVersion(srv.Client())
	if err != nil || got != "151.0.7922.108.2" {
		t.Fatalf("%q %v", got, err)
	}
}
