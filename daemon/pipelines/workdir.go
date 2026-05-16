package pipelines

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// CreateWorkDir creates a unique scratch directory rooted under root
// (or os.TempDir() when root is empty). The directory name is
// "discecho-<kind>-<discID>-<base36-nanos>" — when kind is empty the
// "<kind>-" segment is omitted (matches the audio-CD layout). Each
// pipeline calls this once per Run and defers RemoveAll on the result.
func CreateWorkDir(root, kind, discID string) (string, error) {
	if root == "" {
		root = os.TempDir()
	}
	name := "discecho-"
	if kind != "" {
		name += kind + "-"
	}
	name += discID + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create workdir: %w", err)
	}
	return dir, nil
}
