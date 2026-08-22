package store

import "testing"

func TestEmailSuffix(t *testing.T) {
	if EmailSuffix("b4o3keyhc@foxcroftp.us") != "foxcroftp.us" {
		t.Fatal(EmailSuffix("b4o3keyhc@foxcroftp.us"))
	}
	if EmailSuffix("  ZV@JasperWay.US ") != "jasperway.us" {
		t.Fatal(EmailSuffix("  ZV@JasperWay.US "))
	}
	if EmailSuffix("no-at") != "" {
		t.Fatal("empty")
	}
}

func TestSuffixBlacklistRoundtrip(t *testing.T) {
	raw := EncodeSuffixList([]string{"@Foxcroftp.US", "foxcroftp.us", " mail.com "})
	got := DecodeSuffixList(raw)
	if len(got) != 2 || got[0] != "foxcroftp.us" || got[1] != "mail.com" {
		t.Fatalf("%q -> %#v", raw, got)
	}
	if !SuffixBlacklisted(got, "foxcroftp.us") || SuffixBlacklisted(got, "jasperway.us") {
		t.Fatal(got)
	}
}

func TestDecodeSuffixListPlain(t *testing.T) {
	got := DecodeSuffixList("foxcroftp.us\nmail.com, qq.com")
	if len(got) != 3 {
		t.Fatal(got)
	}
}
