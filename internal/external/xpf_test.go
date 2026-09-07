package external

import "testing"

func TestParseXPF(t *testing.T) {
	out := `Starting XPF with /tmp/kernelcache (Darwin Kernel Version 23.4.0)
Kernel base: 0xFFFFFFF007004000
Kernel entry: 0xFFFFFFF0070ABCDE
0x0000000fffffc000 <- kernelSymbol.gPhysBase
0x0000000000000019 <- kernelConstant.T1SZ_BOOT
XPF finished in 1.25 seconds
`
	res, err := ParseXPF(out)
	if err != nil {
		t.Fatal(err)
	}
	if res.KernelVersion != "Darwin Kernel Version 23.4.0" {
		t.Fatalf("version: %q", res.KernelVersion)
	}
	if res.KernelBase != "0xFFFFFFF007004000" {
		t.Fatalf("base: %q", res.KernelBase)
	}
	if res.Offsets["kernelSymbol.gPhysBase"] != "0x0000000fffffc000" {
		t.Fatalf("offsets: %#v", res.Offsets)
	}
	if res.ElapsedSec != "1.25" {
		t.Fatalf("elapsed: %q", res.ElapsedSec)
	}
}
