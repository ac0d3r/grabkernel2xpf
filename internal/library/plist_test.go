package library

import (
	"strings"
	"testing"
)

func TestWriteOffsetsPlist(t *testing.T) {
	got := OffsetsPlist([]Manifest{
		{
			Identifier: "iPhone12,1",
			Build:      "19A346",
			Board:      "n104ap",
			Offsets:    map[string]string{"b": "0x2", "a": "0x1"},
		},
		{
			Identifier: "iPhone12,1",
			Build:      "19A346",
			Board:      "other",
			Offsets:    map[string]string{"a": "ignored"},
		},
	})
	if !strings.Contains(got, "<key>a</key>\n\t\t\t<string>0x1</string>") {
		t.Fatalf("%s", got)
	}
	if strings.Contains(got, "ignored") {
		t.Fatal("duplicate build should keep the first manifest")
	}
	if strings.Count(got, "<key>19A346</key>") != 1 {
		t.Fatalf("%s", got)
	}
}
