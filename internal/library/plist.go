package library

import (
	"sort"
	"strings"
)

// OffsetsPlist writes identifier → build → offset for manifests that have offsets.
// The first manifest for an identifier and build wins.
func OffsetsPlist(items []Manifest) string {
	tree := map[string]map[string][][2]string{}
	var ids []string
	for _, m := range items {
		if !m.HasOffsets() || m.Identifier == "" || m.Build == "" {
			continue
		}
		builds, ok := tree[m.Identifier]
		if !ok {
			builds = map[string][][2]string{}
			tree[m.Identifier] = builds
			ids = append(ids, m.Identifier)
		}
		if _, ok := builds[m.Build]; ok {
			continue
		}
		names := make([]string, 0, len(m.Offsets))
		for name := range m.Offsets {
			names = append(names, name)
		}
		sort.Strings(names)
		offsets := make([][2]string, len(names))
		for i, name := range names {
			offsets[i] = [2]string{name, m.Offsets[name]}
		}
		builds[m.Build] = offsets
	}
	sort.Strings(ids)

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n")
	b.WriteString("<dict>\n")
	for _, id := range ids {
		writeKey(&b, 1, id)
		b.WriteString("\t<dict>\n")
		buildNames := make([]string, 0, len(tree[id]))
		for name := range tree[id] {
			buildNames = append(buildNames, name)
		}
		sort.Strings(buildNames)
		for _, build := range buildNames {
			writeKey(&b, 2, build)
			b.WriteString("\t\t<dict>\n")
			for _, off := range tree[id][build] {
				writeKey(&b, 3, off[0])
				b.WriteString("\t\t\t<string>")
				b.WriteString(xmlEscape(off[1]))
				b.WriteString("</string>\n")
			}
			b.WriteString("\t\t</dict>\n")
		}
		b.WriteString("\t</dict>\n")
	}
	b.WriteString("</dict>\n")
	b.WriteString("</plist>\n")
	return b.String()
}

func writeKey(b *strings.Builder, indent int, key string) {
	b.WriteString(strings.Repeat("\t", indent))
	b.WriteString("<key>")
	b.WriteString(xmlEscape(key))
	b.WriteString("</key>\n")
}

func xmlEscape(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	).Replace(s)
}
