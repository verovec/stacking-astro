package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/verove-jordan/astronomy/internal/localfs"
)

// browseRoots resolves the external-drive roots the UI may browse: the platform removable-media defaults
// (macOS /Volumes; Linux /media, /mnt, /run/media) plus any ASTRO_BROWSE_ROOTS extras.
func (s *Server) browseRoots() []string {
	return localfs.Roots(s.cfg.BrowseRoots)
}

// appDir is one of the app's own configured directories exposed as a browse/copy source in the
// "Local disks → S3" panel: the input (DataDir), output (OutputDir) and work (WorkDir) folders.
type appDir struct {
	Key  string `json:"key"`  // input | output | work
	Path string `json:"path"` // absolute, existing
}

// appDirs returns the app's own DataDir/OutputDir/WorkDir as labeled sources — ABSOLUTIZED (the config
// defaults are relative, e.g. ./data) and existing-only, mirroring how localfs.Roots drops missing roots.
// The filepath.Abs is load-bearing: localfs.Allowed resolves a root's symlinks without absolutizing it, so
// a relative root would never prefix-match an absolutized request path and the folder would be unbrowsable.
func (s *Server) appDirs() []appDir {
	candidates := []appDir{
		{Key: "input", Path: s.cfg.DataDir},
		{Key: "output", Path: s.cfg.OutputDir},
		{Key: "work", Path: s.cfg.WorkDir},
	}
	out := make([]appDir, 0, len(candidates))
	for _, d := range candidates {
		abs, err := filepath.Abs(d.Path)
		if err != nil {
			continue
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			continue // a missing app dir is simply not offered, never fatal
		}
		out = append(out, appDir{Key: d.Key, Path: abs})
	}
	return out
}

// appRootPaths is appDirs() reduced to bare absolute paths, for the browse/upload confinement allow-list.
func (s *Server) appRootPaths() []string {
	dirs := s.appDirs()
	paths := make([]string, 0, len(dirs))
	for _, d := range dirs {
		paths = append(paths, d.Path)
	}
	return paths
}

// localAllowRoots is the allow-list for BROWSE + UPLOAD (not the drive listing): the removable-media browse
// roots PLUS the app's own dirs. localDrives deliberately does NOT use this — it lists only removable media,
// so the app dirs enter solely through Allowed/Browse here, never as fake "drives".
func (s *Server) localAllowRoots() []string {
	return append(s.browseRoots(), s.appRootPaths()...)
}

// localSources lists the app's own configured directories (Input=DataDir, Output=OutputDir, Work=WorkDir)
// as labeled shortcuts the drive-list view offers alongside removable drives. Existing dirs only, absolute
// paths; the label is i18n'd on the frontend from key. GET /api/local/sources
func (s *Server) localSources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sources": s.appDirs()})
}

// localDrives lists the mounted external drives so the UI can offer "inspect an external drive and copy it
// to S3". GET /api/local/drives
func (s *Server) localDrives(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"drives": localfs.Drives(s.browseRoots())})
}

// localBrowse lists one directory level under an allowed path — the removable-drive roots OR the app's own
// input/output/work dirs (symlink escapes rejected). GET /api/local/browse?path=<abs>
func (s *Server) localBrowse(w http.ResponseWriter, r *http.Request) {
	listing, err := localfs.Browse(s.localAllowRoots(), r.URL.Query().Get("path"))
	switch {
	case errors.Is(err, localfs.ErrForbidden):
		badRequest(w, "path is outside the allowed browse roots")
	case err != nil:
		serverError(w, err)
	default:
		writeJSON(w, http.StatusOK, listing)
	}
}
