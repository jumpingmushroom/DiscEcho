package identify

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// SystemCNF holds the boot-code line parsed out of a PSX/PS2 disc's
// SYSTEM.CNF file. IsPS2 is true if the file used `BOOT2 = ...`
// (PS2 convention) rather than `BOOT = ...` (PSX convention).
type SystemCNF struct {
	BootCode string
	IsPS2    bool
}

// SystemCNFProber reads SYSTEM.CNF off a disc.
type SystemCNFProber interface {
	Probe(ctx context.Context, devPath string) (*SystemCNF, error)
}

// NewSystemCNFProber returns a prober that shells out to
// `isoinfo -i <devPath> -x /SYSTEM.CNF;1`. Empty isoinfoBin defaults
// to "isoinfo" (resolved via PATH).
//
// The `;1` ISO9660 version suffix and the omission of `-R` are both
// required. PSX/PS2 discs do not carry Rock Ridge extensions; on a
// disc without RR, `isoinfo -R -x /SYSTEM.CNF` exits 0 with only
// `**BAD RRVERSION (0)` warnings on stdout and *no* file content,
// leaving the classifier unable to discriminate PSX/PS2 from DATA.
// Listings strip the `;1`, so callers can keep using `/SYSTEM.CNF` as
// the in-process path constant — only the extract argument needs it.
func NewSystemCNFProber(isoinfoBin string) SystemCNFProber {
	if isoinfoBin == "" {
		isoinfoBin = "isoinfo"
	}
	return &isoinfoSystemCNFProber{bin: isoinfoBin}
}

type isoinfoSystemCNFProber struct{ bin string }

func (p *isoinfoSystemCNFProber) Probe(ctx context.Context, devPath string) (*SystemCNF, error) {
	cmd := exec.CommandContext(ctx, p.bin, "-i", devPath, "-x", "/SYSTEM.CNF;1")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("isoinfo -x SYSTEM.CNF: %w", err)
	}
	return ParseSystemCNF(string(out)), nil
}

// bootLineRE matches both PSX and PS2 boot lines, case-insensitive:
//
//	BOOT  = cdrom:\SCUS_004.34;1   (PSX, uppercase)
//	boot2 = cdrom0:\sces_50051.elf;1   (PS2, rare lowercase shipment)
//
// Capture groups:
//
//	1 = "" or "2" (PS2 marker)
//	2 = the boot code (filename stem)
var bootLineRE = regexp.MustCompile(`(?i)^\s*BOOT(2)?\s*=\s*cdrom\d?:\\([A-Z]{4}_\d{3}\.\d{2}|[A-Z]{4}_\d{5})`)

// ParseSystemCNF parses raw SYSTEM.CNF content, returning nil if no
// recognisable BOOT/BOOT2 line is present.
//
// PSX: "BOOT = cdrom:\SCUS_004.34;1" → {BootCode: "SCUS_004.34"}.
// PS2: "BOOT2 = cdrom0:\SCES_50051.ELF;1" → reformat the 5-digit code
// as "SCES_500.51" so it's comparable against the same Redump
// boot-code format the dat-file extractor produces.
func ParseSystemCNF(content string) *SystemCNF {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		m := bootLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		isPS2 := strings.EqualFold(m[1], "2")
		code := strings.ToUpper(m[2])
		// Normalise the PS2 5-digit code (e.g. "SCES_50051") to the
		// dotted form ("SCES_500.51") so it matches the dat-file
		// boot-code key.
		if !strings.Contains(code, ".") && len(code) == 10 {
			code = code[:8] + "." + code[8:]
		}
		return &SystemCNF{BootCode: code, IsPS2: isPS2}
	}
	return nil
}
