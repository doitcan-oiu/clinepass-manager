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

func TestSuffixCanStart(t *testing.T) {
	if blocked, busy := SuffixCanStart(3, 0); !blocked || busy {
		t.Fatal("3 fails should block")
	}
	if blocked, busy := SuffixCanStart(2, 1); blocked || !busy {
		t.Fatal("2 fail + 1 running should be busy")
	}
	if blocked, busy := SuffixCanStart(1, 0); blocked || busy {
		t.Fatal("can start")
	}
}
