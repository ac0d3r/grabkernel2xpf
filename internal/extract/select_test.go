package extract

import (
	"strings"
	"testing"

	"github.com/ac0d3r/grabkernel2xpf/internal/appledb"
)

func TestSelectClipsToDeviceReleases(t *testing.T) {
	ranges := []appledb.VersionRange{
		mustRange(t, "15.0-18.7.1"),
		mustRange(t, "26.0-26.0.1"),
	}
	devices := []appledb.Device{
		{Name: "iPhone 8 (CDMA)", Identifier: "iPhone10,1", Boards: []string{"d20ap", "d201ap"}},
		{Name: "iPhone 8 (GSM)", Identifier: "iPhone10,4", Boards: []string{"d20ap"}},
		{Name: "iPhone XS", Identifier: "iPhone11,2", Boards: []string{"d321ap"}},
		{Name: "iPhone 15 Pro", Identifier: "iPhone16,1", Boards: []string{"d84ap"}},
	}
	firmware := map[string][]appledb.Build{
		"iPhone10,1": {
			{OS: "iOS", Version: "14.8.1", Build: "18H107"},
			{OS: "iOS", Version: "15.0", Build: "19A346"},
			{OS: "iOS", Version: "16.7.11", Build: "20H360"},
			{OS: "iOS", Version: "16.7 beta 1", Build: "20G5026e", Beta: true},
		},
		"iPhone10,4": {
			{OS: "iOS", Version: "16.7.10", Build: "20H350"},
		},
		"iPhone11,2": {
			{OS: "iOS", Version: "15.0", Build: "19A346"},
			{OS: "iOS", Version: "18.7.1", Build: "22H31"},
			{OS: "iOS", Version: "18.7.2", Build: "22H123"},
		},
		"iPhone16,1": {
			{OS: "iOS", Version: "17.0", Build: "21A329"},
			{OS: "iOS", Version: "18.7.1", Build: "22H31"},
			{OS: "iOS", Version: "26.0", Build: "23A340"},
			{OS: "iOS", Version: "26.0.1", Build: "23A355"},
			{OS: "iOS", Version: "26.0.2", Build: "23A360"},
			{OS: "iOS", Version: "26.0 RC", Build: "23A339", RC: true},
		},
	}
	items, missing, err := Select(devices, []string{"iPhone 8", "iPhone11,2", "iPhone 15 Pro"}, "iOS", ranges, func(_, id string) ([]appledb.Build, error) {
		return firmware[id], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing: %v", missing)
	}
	got := map[string][]string{}
	for _, item := range items {
		if item.Board == "" || item.OS != "iOS" {
			t.Fatalf("%+v", item)
		}
		got[item.Identifier] = append(got[item.Identifier], item.Version+" "+item.Build+" "+item.Board)
	}
	want := map[string][]string{
		"iPhone10,1": {"15.0 19A346 d20ap", "16.7.11 20H360 d20ap"},
		"iPhone10,4": {"16.7.10 20H350 d20ap"},
		"iPhone11,2": {"15.0 19A346 d321ap", "18.7.1 22H31 d321ap"},
		"iPhone16,1": {"17.0 21A329 d84ap", "18.7.1 22H31 d84ap", "26.0 23A340 d84ap", "26.0.1 23A355 d84ap"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %#v", got)
	}
	for id, versions := range want {
		if strings.Join(got[id], "|") != strings.Join(versions, "|") {
			t.Fatalf("%s:\n got %v\nwant %v", id, got[id], versions)
		}
	}
}

func TestSelectMajorIncludesMinors(t *testing.T) {
	r := mustRange(t, "15-16")
	devices := []appledb.Device{
		{Name: "iPhone X", Identifier: "iPhone10,3", Boards: []string{"d22ap"}},
	}
	items, missing, err := Select(devices, []string{"iPhone10,3"}, "iOS", []appledb.VersionRange{r}, func(_, _ string) ([]appledb.Build, error) {
		return []appledb.Build{
			{OS: "iOS", Version: "15.0", Build: "19A346"},
			{OS: "iOS", Version: "16.7.12", Build: "20H364"},
			{OS: "iOS", Version: "17.0", Build: "21A329"},
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 || len(items) != 2 {
		t.Fatalf("items=%v missing=%v", items, missing)
	}
	if items[1].Version != "16.7.12" {
		t.Fatalf("last build %s", items[1].Version)
	}
}

func TestSelectNameDoesNotCrossModels(t *testing.T) {
	devices := []appledb.Device{
		{Name: "iPhone XS", Identifier: "iPhone11,2", Boards: []string{"d321ap"}},
		{Name: "iPhone XS Max", Identifier: "iPhone11,6", Boards: []string{"d331pap"}},
		{Name: "iPhone XS Max (China mainland)", Identifier: "iPhone11,4", Boards: []string{"d331ap"}},
		{Name: "iPhone XR", Identifier: "iPhone11,8", Boards: []string{"n841ap"}},
	}
	builds := func(_, _ string) ([]appledb.Build, error) {
		return []appledb.Build{{OS: "iOS", Version: "18.7.1", Build: "22H31"}}, nil
	}
	xs, _, err := Select(devices, []string{"iPhone XS"}, "iOS", []appledb.VersionRange{mustRange(t, "18.7.1")}, builds)
	if err != nil {
		t.Fatal(err)
	}
	if len(xs) != 1 || xs[0].Identifier != "iPhone11,2" {
		t.Fatalf("%+v", xs)
	}
	max, _, err := Select(devices, []string{"iPhone XS Max"}, "iOS", []appledb.VersionRange{mustRange(t, "18.7.1")}, builds)
	if err != nil {
		t.Fatal(err)
	}
	if len(max) != 2 || max[0].Identifier != "iPhone11,4" || max[1].Identifier != "iPhone11,6" {
		t.Fatalf("%+v", max)
	}
}

func mustRange(t *testing.T, spec string) appledb.VersionRange {
	t.Helper()
	r, err := appledb.ParseVersionRange(spec)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
