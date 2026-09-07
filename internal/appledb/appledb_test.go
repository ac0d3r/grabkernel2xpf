package appledb

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	k := parseVersion("18.7.8", false, false)
	if k.major != 18 || k.minor != 7 || k.patch != 8 || k.channel != 0 {
		t.Fatalf("%+v", k)
	}
	k = parseVersion("26.6.1 RC", false, true)
	if k.major != 26 || k.minor != 6 || k.patch != 1 || k.channel != 1 {
		t.Fatalf("%+v", k)
	}
	k = parseVersion("27.0 beta 8", true, false)
	if k.major != 27 || k.minor != 0 || k.channel != 2 || k.pre != 8 {
		t.Fatalf("%+v", k)
	}
}

func TestParseIdentifier(t *testing.T) {
	major, minor := parseIdentifier("iPad16,11")
	if major != 16 || minor != 11 {
		t.Fatalf("got %d,%d", major, minor)
	}
	major, minor = parseIdentifier("iPhone15,2")
	if major != 15 || minor != 2 {
		t.Fatalf("got %d,%d", major, minor)
	}
}

func TestStringList(t *testing.T) {
	var s StringList
	if err := json.Unmarshal([]byte(`"D73AP"`), &s); err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || s[0] != "D73AP" {
		t.Fatalf("%v", s)
	}
	if err := json.Unmarshal([]byte(`["D73AP","D74AP"]`), &s); err != nil {
		t.Fatal(err)
	}
	if len(s) != 2 {
		t.Fatalf("%v", s)
	}
}

func TestDevicesAndBuildsLive(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	c := New()
	devs, err := c.Devices("iOS")
	if err != nil {
		t.Fatal(err)
	}
	var found *Device
	for i := range devs {
		if devs[i].Identifier == "iPhone15,2" {
			found = &devs[i]
			break
		}
	}
	if found == nil || len(found.Boards) == 0 {
		t.Fatalf("iPhone15,2 missing: %#v", found)
	}
	if identifierFamily(devs[0].Identifier) < identifierFamily(devs[len(devs)-1].Identifier) {
		t.Fatalf("expected newest first, got %s ... %s", devs[0].Identifier, devs[len(devs)-1].Identifier)
	}
	builds, err := c.Builds("iOS", "iPhone15,2")
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) == 0 {
		t.Fatal("no builds")
	}
	has := false
	for _, b := range builds {
		if b.Build == "21E219" {
			has = true
			break
		}
	}
	if !has {
		t.Fatalf("21E219 not in %d builds (first %+v)", len(builds), builds[0])
	}

	ipads, err := c.Devices("iPadOS")
	if err != nil {
		t.Fatal(err)
	}
	var ipad *Device
	for i := range ipads {
		if ipads[i].Identifier == "iPad14,1" {
			ipad = &ipads[i]
			break
		}
	}
	for _, d := range ipads {
		if strings.Contains(strings.ToLower(d.Name), "unreleased") {
			t.Fatalf("unreleased shown: %s %s", d.Name, d.Identifier)
		}
	}
	if ipad == nil {
		t.Fatal("iPad14,1 missing")
	}
	if !strings.Contains(ipads[0].Name, "iPad Pro") {
		t.Fatalf("expected iPad Pro first, got %s (%s)", ipads[0].Name, ipads[0].Identifier)
	}
	var sawAir bool
	for i, d := range ipads {
		if strings.Contains(d.Name, "iPad Air") {
			sawAir = true
			continue
		}
		if sawAir && strings.Contains(d.Name, "iPad Pro") {
			t.Fatalf("iPad Pro after Air at %d: %s", i, d.Name)
		}
	}
	ipadBuilds, err := c.Builds("iPadOS", "iPad14,1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ipadBuilds) == 0 {
		t.Fatal("no iPadOS builds for iPad14,1")
	}
	if ipadBuilds[0].OS != "iPadOS" && ipadBuilds[0].OS != "iOS" {
		t.Fatalf("unexpected os %q", ipadBuilds[0].OS)
	}
	prev := parseVersion(ipadBuilds[0].Version, ipadBuilds[0].Beta, ipadBuilds[0].RC).major
	for _, b := range ipadBuilds {
		maj := parseVersion(b.Version, b.Beta, b.RC).major
		if maj > prev {
			t.Fatalf("builds not grouped by version: %s after major %d", b.Version, prev)
		}
		prev = maj
	}
}
