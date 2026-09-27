package browser

import "testing"

func TestParseLddMissing(t *testing.T) {
	raw := `
	linux-vdso.so.1 (0x0000)
	libnss3.so => not found
	libgbm.so.1 => not found
	libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x0000)
`
	got := parseLddMissing(raw)
	if len(got) != 2 || got[0] != "libnss3.so" || got[1] != "libgbm.so.1" {
		t.Fatalf("%v", got)
	}
}
