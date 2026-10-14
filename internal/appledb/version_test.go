package appledb

import (
	"strings"
	"testing"
)

func TestVersionRanges(t *testing.T) {
	ranges := mustRanges(t, "15.0-18.7.1", "26.0-26.0.1")
	in := []string{"15.0", "15.0.1", "16.7.11", "18.7.1", "26.0", "26.0.1"}
	out := []string{"14.8.1", "18.7.2", "19.0", "26.0.2", "26.1", "26.0 beta 1"}
	for _, v := range in {
		if !VersionInRanges(v, ranges) {
			t.Fatalf("%s should match", v)
		}
	}
	for _, v := range out {
		if VersionInRanges(v, ranges) {
			t.Fatalf("%s should not match", v)
		}
	}

	through16 := mustRanges(t, "15-16")
	if !VersionInRanges("16.7.12", through16) || VersionInRanges("17.0", through16) {
		t.Fatal("major upper bound should include every 16.x and stop before 17")
	}
	patches := mustRanges(t, "16.7")
	if !VersionInRanges("16.7", patches) || !VersionInRanges("16.7.11", patches) || VersionInRanges("16.6.1", patches) || VersionInRanges("16.8", patches) {
		t.Fatal("minor bound should include every patch of that minor")
	}
	exact := mustRanges(t, "18.7.1")
	if !VersionInRanges("18.7.1", exact) || VersionInRanges("18.7.0", exact) || VersionInRanges("18.7.2", exact) {
		t.Fatal("full version should be exact")
	}
}

func TestSelectReleasesClipsToDevice(t *testing.T) {
	builds := []Build{
		{OS: "iOS", Version: "14.8.1", Build: "18H107"},
		{OS: "iOS", Version: "16.7.11", Build: "20H360"},
		{OS: "iOS", Version: "16.7.10", Build: "20H350"},
		{OS: "iOS", Version: "17.0", Build: "21A329"},
		{OS: "iOS", Version: "15.0 beta 1", Build: "19A5281h", Beta: true},
		{OS: "iPadOS", Version: "15.0", Build: "19A346"},
	}
	got := SelectReleases(builds, "iOS", mustRanges(t, "15.0-18.7.1"))
	if len(got) != 3 || got[0].Build != "20H350" || got[1].Build != "20H360" || got[2].Build != "21A329" {
		t.Fatalf("%+v", got)
	}
}

func TestResolveDevices(t *testing.T) {
	devs := []Device{
		{Name: "iPhone 8 (CDMA)", Identifier: "iPhone10,1", Boards: []string{"d20ap"}},
		{Name: "iPhone 8 Plus (CDMA)", Identifier: "iPhone10,2", Boards: []string{"d21ap"}},
		{Name: "iPhone 8 (GSM)", Identifier: "iPhone10,4", Boards: []string{"d201ap"}},
		{Name: "iPhone X (GSM)", Identifier: "iPhone10,6", Boards: []string{"d221ap"}},
		{Name: "iPhone XR", Identifier: "iPhone11,8", Boards: []string{"n841ap"}},
		{Name: "iPhone XS", Identifier: "iPhone11,2", Boards: []string{"d321ap"}},
		{Name: "iPhone XS Max", Identifier: "iPhone11,6", Boards: []string{"d331pap"}},
		{Name: "iPhone XS Max (China mainland)", Identifier: "iPhone11,4", Boards: []string{"d331ap"}},
		{Name: "iPhone SE (1st generation)", Identifier: "iPhone8,4", Boards: []string{"n69ap"}},
		{Name: "iPhone SE (2nd generation)", Identifier: "iPhone12,8", Boards: []string{"d79ap"}},
	}
	got, err := ResolveDevices(devs, []string{"iPhone 8", "iPhone X", "iPhone12,8"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, d := range got {
		ids = append(ids, d.Identifier)
	}
	want := "iPhone10,1,iPhone10,4,iPhone10,6,iPhone12,8"
	if strings.Join(ids, ",") != want {
		t.Fatalf("got %s", strings.Join(ids, ","))
	}
	xs, err := ResolveDevices(devs, []string{"iPhone XS"})
	if err != nil {
		t.Fatal(err)
	}
	if len(xs) != 1 || xs[0].Identifier != "iPhone11,2" {
		t.Fatalf("XS matched %+v", xs)
	}
	max, err := ResolveDevices(devs, []string{"iPhone XS Max"})
	if err != nil {
		t.Fatal(err)
	}
	if len(max) != 2 {
		t.Fatalf("XS Max matched %+v", max)
	}
	if _, err := ResolveDevices(devs, []string{"iPhone 9"}); err == nil {
		t.Fatal("expected unknown device")
	}
}

func mustRanges(t *testing.T, specs ...string) []VersionRange {
	t.Helper()
	var out []VersionRange
	for _, s := range specs {
		r, err := ParseVersionRange(s)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}
