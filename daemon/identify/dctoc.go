package identify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DCTOCProber implements DCProber by shelling out to `cdrdao disk-info`
// and checking for a two-session layout with a high-density-area
// session-2 start LBA.
//
// GD-ROM discs put the high-density area at LBA ≥ 45000. Standard
// optical drives can read the multi-session TOC even when they cannot
// pull data from the HD area itself.
type DCTOCProber struct {
	Bin string // "" → defaults to "cdrdao"
}

func (p *DCTOCProber) Probe(ctx context.Context, devPath string) (bool, error) {
	bin := p.Bin
	if bin == "" {
		bin = "cdrdao"
	}
	out, err := exec.CommandContext(ctx, bin, "disk-info", "--device", devPath).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("cdrdao disk-info: %w", err)
	}
	return looksLikeDreamcast(string(out)), nil
}

// looksLikeDreamcast inspects cdrdao disk-info output and returns true
// if the disc has 2 sessions and session 2 starts at LBA ≥ 45000.
func looksLikeDreamcast(disk string) bool {
	sessions, s2Start := 0, 0
	for _, line := range strings.Split(disk, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "Sessions") {
			_, _ = fmt.Sscanf(l, "Sessions: %d", &sessions)
		}
		if strings.HasPrefix(l, "Session 2 first LBA") {
			_, _ = fmt.Sscanf(l, "Session 2 first LBA: %d", &s2Start)
		}
	}
	return sessions == 2 && s2Start >= 45000
}
