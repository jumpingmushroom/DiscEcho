package embed

import (
	"embed"
	"io/fs"
)

// staticFS embeds the SvelteKit static build. The webui_build directory
// must contain at least an index.html before `go build`. The committed
// placeholder is overwritten by the build pipeline.
//
//go:embed all:webui_build
var staticFS embed.FS

// FS returns a sub-FS rooted at the SvelteKit build directory.
func FS() (fs.FS, error) {
	return fs.Sub(staticFS, "webui_build")
}
