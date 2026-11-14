// Package file reads and writes files the way the agents expect theirs to change: a missing file is empty, a
// write is atomic (a temporary file and a rename) and keeps symlinks and permissions, and a folder is synced to
// exactly a set of files.
package file

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Read returns the content of path and whether it exists; a missing file is empty and not an error.
func Read(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

// ReadDir returns the content of the files of dir whose name ends in ext, by name; a missing folder has none.
func ReadDir(dir, ext string) (map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) {
			continue
		}
		if files[e.Name()], err = os.ReadFile(filepath.Join(dir, e.Name())); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// Write writes data to path through a temporary file and a rename, so a reader meanwhile sees the old file or
// the new one, never half of it. A symlink is followed and its target written, so the link stays; an existing
// file keeps its permissions, a new one gets 0644, and the folder is created.
func Write(path string, data []byte) error {
	mode := fs.FileMode(0o644)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), mode)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// Sync makes dir hold exactly files (paths relative to it, with slashes): it writes the ones that differ and
// removes the rest, with the folders they leave empty. It returns the paths created, updated and removed, in order.
func Sync(dir string, files map[string][]byte) (created, updated, removed []string, err error) {
	if created, updated, err = writeChanged(dir, files); err != nil {
		return created, updated, nil, err
	}
	removed, err = removeStale(dir, staleFiles(dir, files))
	return created, updated, removed, err
}

// writeChanged writes the files whose content differs from the one in dir, by name, and returns the paths it
// created and updated until the first failure.
func writeChanged(dir string, files map[string][]byte) (created, updated []string, err error) {
	names := make([]string, 0, len(files))
	for rel := range files {
		names = append(names, rel)
	}
	sort.Strings(names)
	for _, rel := range names {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		old, existed, err := Read(p)
		if err == nil && existed && bytes.Equal(old, files[rel]) {
			continue
		}
		if err := Write(p, files[rel]); err != nil {
			return created, updated, err
		}
		if existed {
			updated = append(updated, p)
		} else {
			created = append(created, p)
		}
	}
	return created, updated, nil
}

// staleFiles returns the paths of the files under dir that files does not hold, in the order of the walk.
func staleFiles(dir string, files map[string][]byte) []string {
	var stale []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an entry that cannot be read is skipped, not removed as stale
		}
		rel, _ := filepath.Rel(dir, p)
		if _, ok := files[filepath.ToSlash(rel)]; !ok {
			stale = append(stale, p)
		}
		return nil
	})
	return stale
}

// removeStale removes the stale files and the folders under dir they leave empty, and returns the paths it
// removed until the first failure.
func removeStale(dir string, stale []string) ([]string, error) {
	var removed []string
	for _, p := range stale {
		if err := os.Remove(p); err != nil {
			return removed, err
		}
		removed = append(removed, p)
		removeEmptyParents(dir, p)
	}
	return removed, nil
}

// removeEmptyParents removes the folders of p up to dir, dir excluded, while they are empty.
func removeEmptyParents(dir, p string) {
	for d := filepath.Dir(p); d != dir && strings.HasPrefix(d, dir); d = filepath.Dir(d) {
		if os.Remove(d) != nil {
			break
		}
	}
}

// Walk returns every entry under the folders that is not a folder, relative to root with slashes, sorted and unique.
// The folders are relative to root with slashes; an empty one means the whole root, and a folder inside another of
// the list is walked once. A folder outside root once cleaned (../secret) adds nothing, a missing folder adds nothing
// (also one whose case differs from the names on disk, on a file system that ignores case)
// and one that cannot be read is skipped. A symbolic link, or a junction on Windows, is listed as an entry and never
// followed, also when it is a folder above the last one of a folder of the list (link of link/sub).
func Walk(root string, folders []string) []string {
	var tops []string
	for _, f := range folders {
		top := path.Clean(strings.Trim(f, "/"))
		if f == "" || top == "." {
			tops = []string{""}
			break
		}
		if !filepath.IsLocal(filepath.FromSlash(top)) {
			continue
		}
		tops = append(tops, top)
	}
	sort.Strings(tops)
	set := map[string]bool{}
	var done []string
	for _, top := range tops {
		if slices.ContainsFunc(done, func(d string) bool { return d == "" || top == d || strings.HasPrefix(top, d+"/") }) {
			continue
		}
		done = append(done, top)
		walkFolder(root, top, set)
	}
	files := make([]string, 0, len(set))
	for f := range set {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

// walkFolder adds to set every entry under the folder top of root that is not a folder, relative to root with
// slashes: nothing when top is missing with the names on disk, and only the link when a folder above its last
// segment is a symbolic link.
func walkFolder(root, top string, set map[string]bool) {
	link, found := folderOnDisk(root, top)
	if link != "" {
		set[link] = true
	}
	if !found || link != "" {
		return
	}
	_ = filepath.WalkDir(filepath.Join(root, filepath.FromSlash(top)), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil //nolint:nilerr // a folder that cannot be read is skipped
		}
		if rel, err := filepath.Rel(root, p); err == nil {
			set[filepath.ToSlash(rel)] = true
		}
		return nil
	})
}

// folderOnDisk checks each segment of top against the names of its parent folder under root, so a segment matches
// only the name the file system uses, also where it ignores case (Docs is missing when the folder is docs). It
// reports whether top exists with those names, and the first segment above its last one (link of link/sub) that is
// a link, with slashes, so Walk lists that link and does not walk through it. A link is a symbolic link or an
// irregular entry, such as a junction on Windows, which Go marks ModeIrregular, not ModeSymlink; any other entry
// that is not a folder, such as a regular file, means top is missing. The last segment is left to filepath.WalkDir,
// which lists a link or a junction there without following it. The whole root (an empty top) always exists.
func folderOnDisk(root, top string) (link string, found bool) {
	if top == "" {
		return "", true
	}
	segments := strings.Split(top, "/")
	for i, segment := range segments {
		parent := filepath.Join(root, filepath.FromSlash(strings.Join(segments[:i], "/")))
		entries, err := os.ReadDir(parent)
		if err != nil {
			return "", false
		}
		j := slices.IndexFunc(entries, func(e fs.DirEntry) bool { return e.Name() == segment })
		if j < 0 {
			return "", false
		}
		if i == len(segments)-1 || entries[j].IsDir() {
			continue
		}
		if entries[j].Type()&(fs.ModeSymlink|fs.ModeIrregular) == 0 {
			return "", false
		}
		return strings.Join(segments[:i+1], "/"), true
	}
	return "", true
}

// Exists reports whether path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDir reports whether path is a folder.
func IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// CacheFile is the file name in the cache folder of app: the folder named by the environment variable envVar
// when set, else <user cache dir>/<app>.
func CacheFile(envVar, app, name string) (string, error) {
	dir := os.Getenv(envVar)
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, app)
	}
	return filepath.Join(dir, name), nil
}
