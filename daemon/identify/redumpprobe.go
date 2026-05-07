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
// `isoinfo -R -i <devPath> -x /SYSTEM.CNF`. Empty isoinfoBin defaults
// to "isoinfo" (resolved via PATH).
func NewSystemCNFProber(isoinfoBin string) SystemCNFProber {
	if isoinfoBin == "" {
		isoinfoBin = "isoinfo"
	}
	return &isoinfoSystemCNFProber{bin: isoinfoBin}
}

type isoinfoSystemCNFProber struct{ bin string }

func (p *isoinfoSystemCNFProber) Probe(ctx context.Context, devPath string) (*SystemCNF, error) {
	cmd := exec.CommandContext(ctx, p.bin, "-R", "-i", devPath, "-x", "/SYSTEM.CNF")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("isoinfo -x SYSTEM.CNF: %w", err)
	}
	return ParseSystemCNF(string(out)), nil
}

// bootLineRE matches both PSX and PS2 boot lines:
//
//	BOOT  = cdrom:\SCUS_004.34;1
//	BOOT2 = cdrom0:\SCES_50051.ELF;1
//
// Capture groups:
//
//	1 = "" or "2" (PS2 marker)
//	2 = the boot code (filename stem). For PS2, this is the 5-digit
//	    form ("SCES_50051"); ParseSystemCNF normalises it to the
//	    dotted form ("SCES_500.51") so it matches the dat-file key.
var bootLineRE = regexp.MustCompile(`^\s*BOOT(2)?\s*=\s*cdrom\d?:\\([A-Z]{4}_\d{3}\.\d{2}|[A-Z]{4}_\d{5})`)

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
		isPS2 := m[1] == "2"
		code := m[2]
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
