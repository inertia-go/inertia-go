package inertia

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"sort"
)

func (i *Inertia) currentVersion(r *http.Request) string {
	switch {
	case i.cfg.Version != "":
		return i.cfg.Version
	case i.cfg.VersionFunc != nil:
		return i.cfg.VersionFunc(r)
	case i.cfg.VersionFromFS != nil:
		return i.fsVersion()
	default:
		return ""
	}
}

// fsVersion computes a stable hash over the files in VersionFromFS.
// The hash incorporates each file's path and full content, providing a
// strong fingerprint suitable for asset versioning. The result is cached
// per *Inertia instance via sync.Once, so subsequent requests are free.
//
// File-system errors (WalkDir or ReadFile failures) are intentionally
// swallowed: version computation must not fail a request. A partial scan
// produces a deterministic-but-possibly-stale fingerprint — restart the
// process to force a fresh hash.
func (i *Inertia) fsVersion() string {
	i.fsVerOnce.Do(func() {
		h := sha256.New()
		type entry struct {
			path string
			data []byte
		}
		var entries []entry
		_ = fs.WalkDir(i.cfg.VersionFromFS, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			data, err := fs.ReadFile(i.cfg.VersionFromFS, path)
			if err != nil {
				return nil
			}
			entries = append(entries, entry{path, data})
			return nil
		})
		sort.Slice(entries, func(a, b int) bool { return entries[a].path < entries[b].path })
		for _, e := range entries {
			h.Write([]byte(e.path))
			h.Write([]byte{0})
			h.Write(e.data)
			h.Write([]byte{0})
		}
		i.fsVer = hex.EncodeToString(h.Sum(nil)[:8]) // 16 hex chars
	})
	return i.fsVer
}
