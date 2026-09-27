package main

import (
	"encoding/json"
	"testing"
)

func TestParseChromeMetaStableWin64(t *testing.T) {
	raw := []byte(`{"channels":{"Stable":{"version":"152.0.7977.54","downloads":{"chrome":[{"platform":"win64","url":"https://example.com/chrome-win64.zip"},{"platform":"linux64","url":"https://example.com/chrome-linux64.zip"}]}}}}`)
	var meta chromeMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	ch := meta.Channels["Stable"]
	if ch.Version != "152.0.7977.54" {
		t.Fatalf("%s", ch.Version)
	}
	var win, linux string
	for _, item := range ch.Downloads.Chrome {
		if item.Platform == "win64" {
			win = item.URL
		}
		if item.Platform == "linux64" {
			linux = item.URL
		}
	}
	if win == "" || linux == "" {
		t.Fatalf("%+v", ch.Downloads.Chrome)
	}
}
