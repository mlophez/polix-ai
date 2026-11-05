// Package file reads and writes files the way the agents expect theirs to change: a missing file is empty, a
// write is atomic (a temporary file and a rename) and keeps symlinks and permissions, and a folder is synced to
// exactly a set of files.
package file

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
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
			return created, updated, removed, err
		}
		if existed {
			updated = append(updated, p)
		} else {
			created = append(created, p)
		}
	}
	var stale []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if _, ok := files[filepath.ToSlash(rel)]; !ok {
			stale = append(stale, p)
		}
		return nil
	})
	for _, p := range stale {
		if err := os.Remove(p); err != nil {
			return created, updated, removed, err
		}
		removed = append(removed, p)
		for d := filepath.Dir(p); d != dir && strings.HasPrefix(d, dir); d = filepath.Dir(d) {
			if os.Remove(d) != nil {
				break
			}
		}
	}
	return created, updated, removed, nil
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
