package appledb

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveDevices turns identifiers or product names into devices.
// An exact identifier wins. Otherwise a name matches exactly, or with a
// parenthetical suffix, so "iPhone 8" selects both "iPhone 8 (CDMA)" and
// "iPhone 8 (GSM)" and does not select "iPhone 8 Plus".
func ResolveDevices(devs []Device, queries []string) ([]Device, error) {
	byID := make(map[string]Device, len(devs))
	for _, d := range devs {
		byID[d.Identifier] = d
	}

	var out []Device
	seen := map[string]bool{}
	var missing []string
	add := func(d Device) {
		if seen[d.Identifier] {
			return
		}
		seen[d.Identifier] = true
		out = append(out, d)
	}

	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		if d, ok := byID[q]; ok {
			add(d)
			continue
		}
		var matches []Device
		ql := strings.ToLower(q)
		prefix := ql + " ("
		for _, d := range devs {
			name := strings.ToLower(strings.TrimSpace(d.Name))
			if name == ql || strings.HasPrefix(name, prefix) {
				matches = append(matches, d)
			}
		}
		if len(matches) == 0 {
			missing = append(missing, q)
			continue
		}
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].Identifier < matches[j].Identifier
		})
		for _, d := range matches {
			add(d)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("unknown device %s", strings.Join(missing, ", "))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("identifier is required")
	}
	return out, nil
}
