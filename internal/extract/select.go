package extract

import (
	"fmt"

	"github.com/ac0d3r/grabkernel2xpf/internal/appledb"
)

type Request struct {
	OS         string
	Name       string
	Identifier string
	Build      string
	Board      string
	Version    string
	Refresh    bool
}

// Select expands device queries and version ranges into one release build per
// item. Each device only contributes builds it shipped, so a range such as
// 15.0-18.7.1 stops at that device's last release inside the window.
// The board is the device's first board config.
func Select(devices []appledb.Device, queries []string, osName string, ranges []appledb.VersionRange, builds func(osName, identifier string) ([]appledb.Build, error)) (items []Request, missing []string, err error) {
	chosen, err := appledb.ResolveDevices(devices, queries)
	if err != nil {
		return nil, nil, err
	}
	for _, d := range chosen {
		if len(d.Boards) == 0 {
			return nil, nil, fmt.Errorf("%s has no board config", d.Identifier)
		}
		list, err := builds(osName, d.Identifier)
		if err != nil {
			return nil, nil, err
		}
		matched := appledb.SelectReleases(list, osName, ranges)
		if len(matched) == 0 {
			missing = append(missing, d.Identifier)
			continue
		}
		board := d.Boards[0]
		for _, b := range matched {
			items = append(items, Request{
				OS:         osName,
				Name:       d.Name,
				Identifier: d.Identifier,
				Build:      b.Build,
				Board:      board,
				Version:    b.Version,
			})
		}
	}
	return items, missing, nil
}
