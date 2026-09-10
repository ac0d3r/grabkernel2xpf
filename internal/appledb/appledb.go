package appledb

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const baseURL = "https://api.appledb.dev"

type StringList []string

func (s *StringList) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*s = nil
		return nil
	}
	if data[0] == '"' {
		var v string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		if v == "" {
			*s = nil
			return nil
		}
		*s = []string{v}
		return nil
	}
	var v []string
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*s = v
	return nil
}

type Device struct {
	Name       string   `json:"name"`
	Identifier string   `json:"identifier"`
	Boards     []string `json:"boards"`
}

type Build struct {
	OS      string `json:"os"`
	Version string `json:"version"`
	Build   string `json:"build"`
	Beta    bool   `json:"beta"`
	RC      bool   `json:"rc"`
}

type deviceRaw struct {
	Name       string     `json:"name"`
	Identifier StringList `json:"identifier"`
	Board      StringList `json:"board"`
	Type       string     `json:"type"`
	Released   StringList `json:"released"`
	Internal   bool       `json:"internal"`
}

type firmwareRaw struct {
	OSStr     string     `json:"osStr"`
	Version   string     `json:"version"`
	Build     string     `json:"build"`
	Released  StringList `json:"released"`
	DeviceMap StringList `json:"deviceMap"`
	Beta      bool       `json:"beta"`
	RC        bool       `json:"rc"`
	Internal  bool       `json:"internal"`
}

type Client struct {
	HTTP *http.Client
	TTL  time.Duration

	mu        sync.Mutex
	devices   []deviceRaw
	devicesAt time.Time
	firmwares []firmwareRaw
	fwAt      time.Time
}

func New() *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 90 * time.Second},
		TTL:  time.Hour,
	}
}

func (c *Client) Devices(osName string) ([]Device, error) {
	raw, err := c.devicesCached()
	if err != nil {
		return nil, err
	}
	type keyed struct {
		d        Device
		typ      string
		released string
		major    int
		minor    int
	}
	var rows []keyed
	seen := map[string]bool{}
	for _, d := range raw {
		if !deviceMatchesOS(d.Type, osName) || skipDevice(d) {
			continue
		}
		boards := normalizeBoards(d.Board)
		if len(boards) == 0 {
			continue
		}
		for _, id := range d.Identifier {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			major, minor := parseIdentifier(id)
			rows = append(rows, keyed{
				d: Device{
					Name:       d.Name,
					Identifier: id,
					Boards:     boards,
				},
				typ:      d.Type,
				released: first(d.Released),
				major:    major,
				minor:    minor,
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if ri, rj := deviceTypeRank(rows[i].typ), deviceTypeRank(rows[j].typ); ri != rj {
			return ri < rj
		}
		if rows[i].released != rows[j].released {
			return rows[i].released > rows[j].released
		}
		if rows[i].major != rows[j].major {
			return rows[i].major > rows[j].major
		}
		if rows[i].minor != rows[j].minor {
			return rows[i].minor > rows[j].minor
		}
		return rows[i].d.Identifier > rows[j].d.Identifier
	})
	out := make([]Device, len(rows))
	for i, row := range rows {
		out[i] = row.d
	}
	return out, nil
}

func (c *Client) Builds(osName, identifier string) ([]Build, error) {
	raw, err := c.firmwareCached()
	if err != nil {
		return nil, err
	}
	var out []Build
	type keyed struct {
		b        Build
		released string
		ver      versionKey
	}
	var rows []keyed
	for _, fw := range raw {
		if fw.Internal || fw.Build == "" {
			continue
		}
		if !contains(fw.DeviceMap, identifier) {
			continue
		}
		osStr := fw.OSStr
		if osStr == "" {
			osStr = osName
		}
		b := Build{
			OS:      osStr,
			Version: fw.Version,
			Build:   fw.Build,
			Beta:    fw.Beta,
			RC:      fw.RC,
		}
		rows = append(rows, keyed{
			b:        b,
			released: first(fw.Released),
			ver:      parseVersion(fw.Version, fw.Beta, fw.RC),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].ver, rows[j].ver
		if a.major != b.major {
			return a.major > b.major
		}
		if a.minor != b.minor {
			return a.minor > b.minor
		}
		if a.patch != b.patch {
			return a.patch > b.patch
		}
		if a.channel != b.channel {
			return a.channel < b.channel
		}
		if a.pre != b.pre {
			return a.pre > b.pre
		}
		if rows[i].released != rows[j].released {
			return rows[i].released > rows[j].released
		}
		return rows[i].b.Build > rows[j].b.Build
	})
	seen := map[string]bool{}
	for _, row := range rows {
		if seen[row.b.Build] {
			continue
		}
		seen[row.b.Build] = true
		out = append(out, row.b)
	}
	if out == nil {
		out = []Build{}
	}
	return out, nil
}

func (c *Client) devicesCached() ([]deviceRaw, error) {
	c.mu.Lock()
	if time.Since(c.devicesAt) < c.TTL && c.devices != nil {
		list := c.devices
		c.mu.Unlock()
		return list, nil
	}
	c.mu.Unlock()

	var list []deviceRaw
	if err := c.getJSON(baseURL+"/device/main.json.gz", &list); err != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.devices != nil {
			return c.devices, nil
		}
		return nil, err
	}

	c.mu.Lock()
	c.devices = list
	c.devicesAt = time.Now()
	c.mu.Unlock()
	return list, nil
}

func (c *Client) firmwareCached() ([]firmwareRaw, error) {
	c.mu.Lock()
	if time.Since(c.fwAt) < c.TTL && c.firmwares != nil {
		list := c.firmwares
		c.mu.Unlock()
		return list, nil
	}
	c.mu.Unlock()

	// AppleDB has no /ios/iPadOS/main.json; iPhone and iPad builds
	// both live in the iOS collection, distinguished by osStr.
	var list []firmwareRaw
	if err := c.getJSON(baseURL+"/ios/iOS/main.json.gz", &list); err != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.firmwares != nil {
			return c.firmwares, nil
		}
		return nil, err
	}

	c.mu.Lock()
	c.firmwares = list
	c.fwAt = time.Now()
	c.mu.Unlock()
	return list, nil
}

func (c *Client) getJSON(url string, dest any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "grabkernel2xpf")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("appledb %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		gr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return err
		}
		defer gr.Close()
		body, err = io.ReadAll(gr)
		if err != nil {
			return err
		}
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("appledb parse %s: %w", url, err)
	}
	return nil
}

func deviceMatchesOS(typ, osName string) bool {
	switch osName {
	case "iOS":
		return typ == "iPhone"
	case "iPadOS":
		return strings.HasPrefix(typ, "iPad")
	default:
		return false
	}
}

func normalizeBoards(boards StringList) []string {
	var out []string
	seen := map[string]bool{}
	for _, b := range boards {
		b = strings.ToLower(strings.TrimSpace(b))
		if b == "" || seen[b] {
			continue
		}
		seen[b] = true
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}

func contains(list StringList, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func first(list StringList) string {
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

func skipDevice(d deviceRaw) bool {
	if d.Internal {
		return true
	}
	return strings.Contains(strings.ToLower(d.Name), "unreleased")
}

func deviceTypeRank(typ string) int {
	switch typ {
	case "iPhone", "iPad Pro":
		return 0
	case "iPad Air":
		return 1
	case "iPad mini":
		return 2
	case "iPad":
		return 3
	default:
		return 9
	}
}

func parseVersion(v string, beta, rc bool) versionKey {
	var k versionKey
	lower := strings.ToLower(v)
	switch {
	case strings.Contains(lower, "beta") || beta:
		k.channel = 2
		if i := strings.Index(lower, "beta"); i >= 0 {
			k.pre, _ = readInt(strings.TrimSpace(v[i+4:]))
		}
	case strings.Contains(lower, "rc") || rc:
		k.channel = 1
		if i := strings.Index(lower, "rc"); i >= 0 {
			k.pre, _ = readInt(strings.TrimSpace(v[i+2:]))
		}
	}
	var rest string
	k.major, rest = readInt(strings.TrimSpace(v))
	if len(rest) > 0 && rest[0] == '.' {
		k.minor, rest = readInt(rest[1:])
	}
	if len(rest) > 0 && rest[0] == '.' {
		k.patch, _ = readInt(rest[1:])
	}
	return k
}

type versionKey struct {
	major, minor, patch int
	channel             int // 0=release, 1=rc, 2=beta
	pre                 int
}

func parseIdentifier(id string) (major, minor int) {
	i := strings.IndexFunc(id, func(r rune) bool { return r >= '0' && r <= '9' })
	if i < 0 {
		return 0, 0
	}
	major, rest := readInt(id[i:])
	if len(rest) > 0 && rest[0] == ',' {
		minor, _ = readInt(rest[1:])
	}
	return major, minor
}

func readInt(s string) (n int, rest string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		i++
	}
	return n, s[i:]
}

func identifierFamily(id string) int {
	major, _ := parseIdentifier(id)
	return major
}
