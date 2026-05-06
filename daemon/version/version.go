package version

// These are overridden at build time via -ldflags
//
//	"-X github.com/jumpingmushroom/DiscEcho/daemon/version.Version=..."
//
// When unset (e.g. plain `go build`) sensible defaults apply so callers
// always get a non-empty struct.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

func Info() BuildInfo {
	return BuildInfo{Version: Version, Commit: Commit, BuildDate: BuildDate}
}
