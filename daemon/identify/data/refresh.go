//go:build ignore

// refresh fetches per-system boot-code → game-name maps from upstream
// community sources, normalises them to DiscEcho's canonical key form,
// and writes the result to daemon/identify/data/bootcodes_<sys>.json.
//
// Run locally before each release:
//
//	cd daemon/identify/data && go run refresh.go
//
// Sources:
//
//	PS2  → PCSX2 GameIndex.yaml      (https://raw.githubusercontent.com/PCSX2/pcsx2/master/bin/resources/GameIndex.yaml)
//	PSX  → DuckStation gamedb.yaml   (https://raw.githubusercontent.com/stenzek/duckstation/master/data/resources/gamedb.yaml)
//	SAT  → Libretro Sega - Saturn.dat
//	DC   → Libretro Sega - Dreamcast.dat
//	XBOX → Libretro Microsoft - Xbox.dat
//
// CI does NOT run this — it's a maintainer tool. Committed output files
// are embedded into the binary via daemon/identify/bootcodeindex.go.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const outputDir = "." // run from daemon/identify/data/

type outputFile struct {
	System    string                 `json:"system"`
	Source    string                 `json:"source"`
	SourceURL string                 `json:"source_url"`
	FetchedAt string                 `json:"fetched_at"`
	Entries   map[string]outputEntry `json:"entries"`
}

type outputEntry struct {
	Title    string `json:"title"`
	Region   string `json:"region,omitempty"`
	Year     int    `json:"year,omitempty"`
	CoverURL string `json:"cover_url,omitempty"`
}

func main() {
	only := flag.String("only", "", "comma-separated system list (psx,ps2,sat,dc,xbox); default = all")
	flag.Parse()

	wanted := map[string]bool{}
	if *only == "" {
		for _, s := range []string{"psx", "ps2", "sat", "dc", "xbox"} {
			wanted[s] = true
		}
	} else {
		for _, s := range strings.Split(*only, ",") {
			wanted[strings.TrimSpace(s)] = true
		}
	}

	fetchers := []struct {
		key  string
		name string
		run  func() error
	}{
		{"ps2", "PS2", refreshPS2},
		{"psx", "PSX", refreshPSX},
		{"sat", "SAT", func() error { return refreshLibretro("SAT", "Sega - Saturn") }},
		{"dc", "DC", func() error { return refreshLibretro("DC", "Sega - Dreamcast") }},
		{"xbox", "XBOX", func() error { return refreshLibretro("XBOX", "Microsoft - Xbox") }},
	}

	for _, f := range fetchers {
		if !wanted[f.key] {
			continue
		}
		log.Printf("refreshing %s...", f.name)
		if err := f.run(); err != nil {
			log.Fatalf("%s: %v", f.name, err)
		}
	}
	log.Println("done")
}

func writeOutput(file outputFile) error {
	suffix := strings.ToLower(file.System)
	path := filepath.Join(outputDir, fmt.Sprintf("bootcodes_%s.json", suffix))
	buf, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o644)
}

func httpGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// --- PS2: PCSX2 GameIndex.yaml ---

func refreshPS2() error {
	const url = "https://raw.githubusercontent.com/PCSX2/pcsx2/master/bin/resources/GameIndex.yaml"
	raw, err := httpGet(url)
	if err != nil {
		return err
	}
	var doc map[string]struct {
		Name   string `yaml:"name"`
		Region string `yaml:"region"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return err
	}
	out := outputFile{
		System:    "PS2",
		Source:    "PCSX2 GameIndex.yaml",
		SourceURL: url,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:   map[string]outputEntry{},
	}
	for hyphenated, entry := range doc {
		key := normalisePS2BootCode(hyphenated)
		if key == "" {
			continue
		}
		out.Entries[key] = outputEntry{
			Title:  entry.Name,
			Region: normalisePCSX2Region(entry.Region),
		}
	}
	return writeOutput(out)
}

// normalisePS2BootCode converts PCSX2's "SCES-53409" form to the daemon's
// canonical "SCES_534.09" used by ParseSystemCNF.
func normalisePS2BootCode(hyphenated string) string {
	re := regexp.MustCompile(`^([A-Z]{4})-(\d{5})$`)
	m := re.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(hyphenated)))
	if m == nil {
		return ""
	}
	return m[1] + "_" + m[2][:3] + "." + m[2][3:]
}

func normalisePCSX2Region(r string) string {
	switch strings.ToUpper(r) {
	case "PAL-E", "PAL-EU", "PAL-EUROPE":
		return "Europe"
	case "NTSC-U", "NTSC-USA":
		return "USA"
	case "NTSC-J", "NTSC-JP":
		return "Japan"
	case "NTSC-K":
		return "Korea"
	case "NTSC-C":
		return "Asia"
	}
	return ""
}

// --- PSX: DuckStation gamedb.yaml ---
//
// As of 2025 DuckStation renamed gamedb.json to gamedb.yaml and changed
// the schema: top-level keys are the serial codes (e.g. "SCUS-94163") and
// values are game objects with a "name" field. No explicit region field;
// the region is implicit in the serial prefix.

func refreshPSX() error {
	const url = "https://raw.githubusercontent.com/stenzek/duckstation/master/data/resources/gamedb.yaml"
	raw, err := httpGet(url)
	if err != nil {
		return err
	}
	// Map from serial → game record.
	var doc map[string]struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return err
	}
	out := outputFile{
		System:    "PSX",
		Source:    "DuckStation gamedb.yaml",
		SourceURL: url,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:   map[string]outputEntry{},
	}
	for hyphenated, g := range doc {
		key := normalisePSXBootCode(hyphenated)
		if key == "" {
			continue
		}
		out.Entries[key] = outputEntry{
			Title:  g.Name,
			Region: regionFromSerial(hyphenated),
		}
	}
	return writeOutput(out)
}

// regionFromSerial infers the release region from the PSX serial prefix.
// SCUS / SLUS = USA; SCES / SLES = Europe; SCPS / SLPS / SCAJ = Japan;
// PAPX = Japan (PSone); SCED / SLED = Europe (demo); falls through to "".
func regionFromSerial(serial string) string {
	s := strings.ToUpper(strings.TrimSpace(serial))
	switch {
	case strings.HasPrefix(s, "SCUS"), strings.HasPrefix(s, "SLUS"),
		strings.HasPrefix(s, "SCZS"):
		return "USA"
	case strings.HasPrefix(s, "SCES"), strings.HasPrefix(s, "SLES"),
		strings.HasPrefix(s, "SCED"), strings.HasPrefix(s, "SLED"):
		return "Europe"
	case strings.HasPrefix(s, "SCPS"), strings.HasPrefix(s, "SLPS"),
		strings.HasPrefix(s, "SCAJ"), strings.HasPrefix(s, "PAPX"),
		strings.HasPrefix(s, "SLAJ"):
		return "Japan"
	case strings.HasPrefix(s, "SCZS"):
		return "Asia"
	}
	return ""
}

// normalisePSXBootCode converts "SCUS-94163" → "SCUS_941.63".
func normalisePSXBootCode(hyphenated string) string {
	re := regexp.MustCompile(`^([A-Z]{4})-(\d{5})$`)
	m := re.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(hyphenated)))
	if m == nil {
		return ""
	}
	return m[1] + "_" + m[2][:3] + "." + m[2][3:]
}

// --- Saturn / Dreamcast / Xbox: Libretro CLRMAMEPRO DAT ---
//
// Full game lists live in metadat/redump/, not dat/.  Each game block looks like:
//
//	game (
//	    name "Game Title"
//	    region "USA"
//	    serial "PRODUCT-CODE"
//	    releaseyear 1998
//	    rom ( name "..." serial "..." )
//	)

func refreshLibretro(systemKey, libretroName string) error {
	url := fmt.Sprintf(
		"https://raw.githubusercontent.com/libretro/libretro-database/master/metadat/redump/%s.dat",
		strings.ReplaceAll(libretroName, " ", "%20"),
	)
	raw, err := httpGet(url)
	if err != nil {
		return err
	}
	games, err := parseClrMamePro(string(raw))
	if err != nil {
		return fmt.Errorf("parse libretro dat: %w", err)
	}
	out := outputFile{
		System:    systemKey,
		Source:    "Libretro " + libretroName,
		SourceURL: url,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:   map[string]outputEntry{},
	}
	for _, g := range games {
		key := normaliseLibretroSerial(systemKey, g.serial)
		if key == "" {
			continue
		}
		out.Entries[key] = outputEntry{
			Title:  g.name,
			Region: g.region,
			Year:   g.year,
		}
	}
	return writeOutput(out)
}

type clrmameGame struct {
	name   string
	serial string
	region string
	year   int
}

// parseClrMamePro extracts game entries from CLRMAMEPRO DAT content.
// It handles the `game ( ... )` block structure with quoted string values
// and bare integer values.
func parseClrMamePro(content string) ([]clrmameGame, error) {
	var games []clrmameGame
	// Match each game ( ... ) block, including nested parens for rom entries.
	// We walk the string manually to handle nested parens correctly.
	i := 0
	for i < len(content) {
		// Find "game ("
		idx := strings.Index(content[i:], "game (")
		if idx == -1 {
			break
		}
		start := i + idx + len("game (")
		// Find the matching close paren by counting depth.
		depth := 1
		j := start
		for j < len(content) && depth > 0 {
			switch content[j] {
			case '(':
				depth++
			case ')':
				depth--
			}
			j++
		}
		block := content[start : j-1]
		i = j
		g := parseClrMameBlock(block)
		if g.name != "" {
			games = append(games, g)
		}
	}
	return games, nil
}

var clrmameFieldRe = regexp.MustCompile(`(?m)^\s*(\w+)\s+(?:"([^"]*)"|([-\w.]+))`)

func parseClrMameBlock(block string) clrmameGame {
	var g clrmameGame
	matches := clrmameFieldRe.FindAllStringSubmatch(block, -1)
	for _, m := range matches {
		key := m[1]
		val := m[2]
		if val == "" {
			val = m[3]
		}
		switch key {
		case "name":
			if g.name == "" { // first name wins (some blocks repeat)
				g.name = val
			}
		case "serial":
			if g.serial == "" {
				g.serial = val
			}
		case "region":
			if g.region == "" {
				g.region = val
			}
		case "releaseyear":
			if g.year == 0 {
				fmt.Sscanf(val, "%d", &g.year)
			}
		}
	}
	return g
}

func normaliseLibretroSerial(systemKey, serial string) string {
	s := strings.TrimSpace(strings.ToUpper(serial))
	if s == "" {
		return ""
	}
	if systemKey == "XBOX" {
		// Redump Xbox serials are publisher codes (e.g. "MS-004", "EA-013"),
		// not XBE TitleIDs. The daemon's ProbeXBE returns a uint32 TitleID
		// formatted as 8-digit uppercase hex (e.g. "4D530002"), so publisher
		// codes never match. Only keep true 8-digit hex strings; the result
		// is currently empty — Xbox identification falls back to Redump MD5
		// matching and IGDB manual search.
		if matched, _ := regexp.MatchString(`^[0-9A-F]{8}$`, s); !matched {
			return ""
		}
	}
	return s
}
