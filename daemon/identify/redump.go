package identify

import (
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// RedumpEntry is one game from a Redump dat file. Only the fields the
// daemon actually consults are populated — full <rom> attributes
// (CRC32, SHA-1, size) are ignored at load time to keep the in-memory
// index small (~5 MB for ~5000 entries).
type RedumpEntry struct {
	Name     string // raw <game name="..."> as shipped by Redump
	Title    string // human title with disambiguators stripped
	Region   string // "USA" | "Europe" | "Japan" | ...
	Year     int    // 0 if not present in the name
	MD5      string // canonical Redump MD5 of the rip (.bin/.iso)
	BootCode string // "SCUS_004.34"
}

// RedumpDB is an in-memory index over a Redump .dat file. Loaded once
// at daemon startup, queried per disc-insert.
type RedumpDB struct {
	byBootCode map[string]RedumpEntry
	byMD5      map[string]RedumpEntry
}

// LoadRedumpDB parses a Redump XML dat file. Builds boot-code and MD5
// indexes. The boot code is extracted from each <rom>'s name attribute
// (the bracketed code in `<Title> [<BootCode>].<ext>`); region from
// the title's first parenthetical segment; year from a `(YYYY)`
// segment if present.
func LoadRedumpDB(path string) (*RedumpDB, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open redump dat: %w", err)
	}
	defer func() { _ = f.Close() }()

	type romXML struct {
		Name string `xml:"name,attr"`
		MD5  string `xml:"md5,attr"`
	}
	type gameXML struct {
		Name string   `xml:"name,attr"`
		ROMs []romXML `xml:"rom"`
	}
	type dat struct {
		Games []gameXML `xml:"game"`
	}

	var d dat
	dec := xml.NewDecoder(f)
	dec.Strict = false
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("decode redump dat: %w", err)
	}

	db := &RedumpDB{
		byBootCode: make(map[string]RedumpEntry),
		byMD5:      make(map[string]RedumpEntry),
	}
	for _, g := range d.Games {
		entry := RedumpEntry{Name: g.Name}
		entry.Title, entry.Region, entry.Year = parseGameName(g.Name)

		// Find the .bin/.iso ROM (the one with the MD5 we care about
		// for verify). PSX has both .bin and .cue; we want the .bin
		// MD5. PS2 has just .iso.
		var primary *romXML
		for i, r := range g.ROMs {
			lower := strings.ToLower(r.Name)
			if strings.HasSuffix(lower, ".bin") || strings.HasSuffix(lower, ".iso") {
				primary = &g.ROMs[i]
				break
			}
		}
		if primary == nil && len(g.ROMs) > 0 {
			primary = &g.ROMs[0]
		}
		if primary == nil {
			continue
		}
		entry.MD5 = primary.MD5
		entry.BootCode = parseBootCodeFromROMName(primary.Name)
		if entry.BootCode == "" {
			continue
		}
		db.byBootCode[entry.BootCode] = entry
		if entry.MD5 != "" {
			db.byMD5[entry.MD5] = entry
		}
	}
	return db, nil
}

// LookupByBootCode returns the entry for a SYSTEM.CNF boot code like
// "SCUS_004.34". Returns nil if not found.
func (db *RedumpDB) LookupByBootCode(code string) *RedumpEntry {
	if db == nil {
		return nil
	}
	if e, ok := db.byBootCode[code]; ok {
		return &e
	}
	return nil
}

// LookupByMD5 returns the entry for a hex MD5 string. Used only for
// post-rip verification.
func (db *RedumpDB) LookupByMD5(md5 string) *RedumpEntry {
	if db == nil {
		return nil
	}
	if e, ok := db.byMD5[md5]; ok {
		return &e
	}
	return nil
}

// parseGameName extracts (title, region, year) from a Redump game name
// like "Final Fantasy VII (USA) (Disc 1)" or "Crash Bandicoot (Europe)
// (En,Fr,De,Es,It) (1996)".
//
// Title: everything before the first " (" segment.
// Region: first parenthetical segment that matches a known region.
// Year: any 4-digit-19xx-or-20xx parenthetical segment.
//
// Unknown regions return "" — caller falls back to displaying the raw
// name. We don't try to be exhaustive; "USA" / "Europe" / "Japan" /
// "World" / "Korea" cover ~99% of Redump entries.
func parseGameName(name string) (title, region string, year int) {
	parens := parenSegments(name)
	idx := strings.Index(name, " (")
	if idx > 0 {
		title = strings.TrimSpace(name[:idx])
	} else {
		title = name
	}
	for _, seg := range parens {
		if isRegion(seg) && region == "" {
			region = seg
			continue
		}
		if y, ok := parseRedumpYear(seg); ok && year == 0 {
			year = y
		}
	}
	return title, region, year
}

func parenSegments(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i, r := range s {
		switch r {
		case '(':
			if depth == 0 {
				start = i + 1
			}
			depth++
		case ')':
			if depth == 1 {
				out = append(out, s[start:i])
			}
			depth--
		}
	}
	return out
}

func isRegion(s string) bool {
	switch s {
	case "USA", "Europe", "Japan", "World", "Korea", "Asia", "Brazil", "Australia":
		return true
	}
	return false
}

func parseRedumpYear(s string) (int, bool) {
	if len(s) != 4 {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	if n < 1980 || n > 2100 {
		return 0, false
	}
	return n, true
}

var bootCodeRE = regexp.MustCompile(`\[([A-Z]{4}_\d{3}\.\d{2})\]`)

// parseBootCodeFromROMName extracts a Redump boot code from a filename
// like "Final Fantasy VII (USA) (Disc 1) [SCUS_004.34].bin".
//
// Returns "" if no `[CCCC_NNN.NN]` bracketed code is present (some
// older Redump entries lack one — those discs go via manualIdentify).
func parseBootCodeFromROMName(name string) string {
	m := bootCodeRE.FindStringSubmatch(name)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
