package api

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/settings"
)

// HostInfo is the payload for GET /api/system/host. Disks lists at most
// one entry per resolvable mount; missing paths are skipped silently
// instead of erroring the whole response.
type HostInfo struct {
	Hostname      string     `json:"hostname"`
	Kernel        string     `json:"kernel"`
	CPUCount      int        `json:"cpu_count"`
	UptimeSeconds int64      `json:"uptime_seconds"`
	Disks         []DiskInfo `json:"disks"`
}

type DiskInfo struct {
	Path           string `json:"path"`
	TotalBytes     uint64 `json:"total_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
}

// IntegrationsInfo is the payload for GET /api/system/integrations.
// The TMDB key itself is never returned — only whether one is set.
type IntegrationsInfo struct {
	TMDB        TMDBIntegration        `json:"tmdb"`
	MusicBrainz MusicBrainzIntegration `json:"musicbrainz"`
	Apprise     AppriseIntegration     `json:"apprise"`
}

type TMDBIntegration struct {
	Configured bool   `json:"configured"`
	Language   string `json:"language"`
}

type MusicBrainzIntegration struct {
	BaseURL   string `json:"base_url"`
	UserAgent string `json:"user_agent"`
}

type AppriseIntegration struct {
	Bin     string `json:"bin"`
	Version string `json:"version,omitempty"`
}

// GetSystemHost returns kernel/CPU/uptime + disk usage for the paths
// the daemon writes to.
func (h *Handlers) GetSystemHost(w http.ResponseWriter, r *http.Request) {
	info := HostInfo{
		Hostname: readTrim("/proc/sys/kernel/hostname"),
		Kernel:   readTrim("/proc/sys/kernel/osrelease"),
		CPUCount: runtime.NumCPU(),
	}
	if up, ok := readUptime("/proc/uptime"); ok {
		info.UptimeSeconds = up
	}
	for _, p := range hostDiskPaths(h.Settings) {
		if p == "" {
			continue
		}
		if d, ok := statDisk(p); ok {
			info.Disks = append(info.Disks, d)
		}
	}
	writeJSON(w, http.StatusOK, info)
}

// GetSystemIntegrations returns connection status + non-secret config
// for external integrations (TMDB, MusicBrainz, Apprise).
func (h *Handlers) GetSystemIntegrations(w http.ResponseWriter, r *http.Request) {
	info := IntegrationsInfo{
		TMDB: TMDBIntegration{Configured: false},
		MusicBrainz: MusicBrainzIntegration{
			BaseURL:   "https://musicbrainz.org",
			UserAgent: "DiscEcho",
		},
		Apprise: AppriseIntegration{Bin: "apprise"},
	}
	if h.Settings != nil {
		info.TMDB.Configured = strings.TrimSpace(h.Settings.TMDBKey) != ""
		info.TMDB.Language = h.Settings.TMDBLang
		info.MusicBrainz.BaseURL = h.Settings.MusicBrainzBaseURL
		info.MusicBrainz.UserAgent = h.Settings.MusicBrainzUserAgent
		info.Apprise.Bin = h.Settings.AppriseBin
	}
	if v, ok := appriseVersion(r.Context(), info.Apprise.Bin); ok {
		info.Apprise.Version = v
	}
	writeJSON(w, http.StatusOK, info)
}

func hostDiskPaths(s *settings.Settings) []string {
	lib, data := "/library", "/var/lib/discecho"
	if s != nil {
		if s.LibraryPath != "" {
			lib = s.LibraryPath
		}
		if s.DataPath != "" {
			data = s.DataPath
		}
	}
	return []string{lib, data}
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readUptime(path string) (int64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0, false
	}
	f, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return int64(f), true
}

func statDisk(path string) (DiskInfo, bool) {
	if _, err := os.Stat(path); err != nil {
		return DiskInfo{}, false
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return DiskInfo{}, false
	}
	bs := uint64(st.Bsize)
	total := st.Blocks * bs
	avail := st.Bavail * bs
	used := uint64(0)
	if total > avail {
		used = total - avail
	}
	return DiskInfo{
		Path:           path,
		TotalBytes:     total,
		UsedBytes:      used,
		AvailableBytes: avail,
	}, true
}

// appriseVersion shells out with a tight timeout and returns the
// trimmed first line. Best-effort — failures omit the version field.
func appriseVersion(ctx context.Context, bin string) (string, bool) {
	if bin == "" {
		return "", false
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, bin, "--version").CombinedOutput()
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if line == "" {
		return "", false
	}
	return line, true
}
