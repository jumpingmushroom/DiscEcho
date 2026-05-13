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
//
// The legacy TMDB / MusicBrainz / Apprise objects are kept alongside
// Items for one release so existing clients (mobile read-only view,
// older webui) keep working. New UI code should prefer Items.
type IntegrationsInfo struct {
	TMDB         TMDBIntegration        `json:"tmdb"`
	MusicBrainz  MusicBrainzIntegration `json:"musicbrainz"`
	Apprise      AppriseIntegration     `json:"apprise"`
	LibraryRoots map[string]string      `json:"library_roots,omitempty"`
	Items        []IntegrationStatus    `json:"items,omitempty"`
}

// IntegrationStatus is a single row in the connections list. Status
// values: "connected", "not configured", or "error: <detail>". Detail
// is a free-form short string ("topic: homelab-disc", "v1.7.0", etc).
// Editable is the env var an operator would change to set this up;
// empty when the row is configured indirectly (e.g. via Apprise URLs).
type IntegrationStatus struct {
	Name     string `json:"name"`
	Hint     string `json:"hint,omitempty"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Editable string `json:"editable,omitempty"`
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
		info.LibraryRoots = h.Settings.LibraryRootsMap()
	}
	if v, ok := appriseVersion(r.Context(), info.Apprise.Bin); ok {
		info.Apprise.Version = v
	}
	info.Items = h.buildIntegrationItems(r.Context(), info)
	writeJSON(w, http.StatusOK, info)
}

// buildIntegrationItems composes the connections list shown on the
// Settings → System tab. Order matches the original mockup: TMDB,
// MusicBrainz, redump, Apprise. Jellyfin / Discord webhook / ntfy
// rows from the design were aspirational and aren't wired in the
// daemon — they're only surfaced once explicit Apprise URLs target
// them.
func (h *Handlers) buildIntegrationItems(ctx context.Context, info IntegrationsInfo) []IntegrationStatus {
	items := []IntegrationStatus{
		{
			Name:     "TMDB",
			Hint:     "movie & TV metadata",
			Editable: "DISCECHO_TMDB_KEY",
			Status: func() string {
				if info.TMDB.Configured {
					return "connected"
				}
				return "not configured"
			}(),
			Detail: info.TMDB.Language,
		},
		{
			Name:   "MusicBrainz",
			Hint:   "audio CD metadata + AccurateRip",
			Status: "connected",
			Detail: info.MusicBrainz.BaseURL,
		},
		{
			Name:     "redump",
			Hint:     "game disc fingerprints",
			Editable: "DISCECHO_REDUMPER_BIN",
			Status:   redumpStatus(h.Settings),
			Detail:   redumpDetail(h.Settings),
		},
		{
			Name:   "Apprise",
			Hint:   "notification dispatch",
			Status: appriseStatus(ctx, h, info),
			Detail: info.Apprise.Version,
		},
		{
			Name:   "GPU transcoding",
			Hint:   "NVIDIA NVENC hardware encoder",
			Status: gpuStatus(h.NVENCAvailable),
			Detail: gpuDetail(h.NVENCAvailable),
		},
	}
	return items
}

func redumpStatus(s *settings.Settings) string {
	if s == nil || strings.TrimSpace(s.RedumperBin) == "" {
		return "not configured"
	}
	if _, err := exec.LookPath(s.RedumperBin); err != nil {
		return "error: redumper binary not found on PATH"
	}
	return "connected"
}

func redumpDetail(s *settings.Settings) string {
	if s == nil {
		return ""
	}
	return s.RedumperBin
}

func gpuStatus(available bool) string {
	if available {
		return "connected"
	}
	return "not configured"
}

func gpuDetail(available bool) string {
	if available {
		return "NVENC (h264, h265)"
	}
	return "no NVIDIA GPU detected"
}

func appriseStatus(ctx context.Context, h *Handlers, info IntegrationsInfo) string {
	if h == nil || h.Store == nil {
		return "not configured"
	}
	notifs, err := h.Store.ListNotifications(ctx)
	if err != nil {
		return "error: " + err.Error()
	}
	enabled := 0
	for _, n := range notifs {
		if n.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return "no URLs configured"
	}
	if info.Apprise.Version == "" {
		return "error: apprise binary missing or unresponsive"
	}
	return "connected"
}

func hostDiskPaths(s *settings.Settings) []string {
	data := "/var/lib/discecho"
	roots := []string{"/library"}
	if s != nil {
		if s.DataPath != "" {
			data = s.DataPath
		}
		// Stat each unique typed root, falling back to LibraryRoot when
		// none are populated (e.g. handler called pre-Load).
		seen := map[string]bool{}
		var unique []string
		for _, m := range settings.AllMediaRoots {
			p := s.LibraryFor(m)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			unique = append(unique, p)
		}
		if len(unique) > 0 {
			roots = unique
		} else if s.LibraryRoot != "" {
			roots = []string{s.LibraryRoot}
		}
	}
	return append(roots, data)
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
