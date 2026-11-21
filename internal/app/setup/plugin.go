package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/mlophez/polix-ai/internal/platform/semver"
)

// Plugin is the plugin embedded in the binary of Version, against the one polix wrote in the local marketplace,
// whose manifest says it is the plugin polix (Written) and of which version (Manifest). It is built with a literal
// followed by Valid.
type Plugin struct {
	Written  bool              // whether the manifest of the plugin polix is in the local marketplace
	Manifest string            // the version of that manifest; compare it with ManifestVersion
	Embedded map[string][]byte // the plugin embedded in the binary, below polix/
	Version  string            // the version of the binary
}

// ErrNoCLI is the error of asking the CLI of an agent that is not on the PATH.
var ErrNoCLI = errors.New("not on the PATH")

// Valid reports why p is not a plugin: it needs the version of the binary, and only a written one has a manifest.
func (p Plugin) Valid() error {
	switch {
	case p.Version == "":
		return errors.New("a plugin needs the version of the binary")
	case !p.Written && p.Manifest != "":
		return errors.New("a plugin not written has no manifest")
	}
	return nil
}

// Current reports whether the plugin written is the one of the binary: written, with its ManifestVersion.
func (p Plugin) Current() bool {
	return p.Written && p.Manifest == p.ManifestVersion()
}

// Skills returns the names of the skills of the plugin, from its files embedded in the binary.
func (p Plugin) Skills() []string {
	var out []string
	for f := range p.Embedded {
		rest, ok := strings.CutPrefix(f, Name+"/skills/")
		if name, _, dir := strings.Cut(rest, "/"); ok && dir && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// Agents returns the names of the agents of the plugin, which only Claude Code loads.
func (p Plugin) Agents() []string {
	var out []string
	for f := range p.Embedded {
		rest, ok := strings.CutPrefix(f, Name+"/agents/")
		if ok && !strings.Contains(rest, "/") && strings.HasSuffix(rest, ".md") {
			out = append(out, strings.TrimSuffix(rest, ".md"))
		}
	}
	slices.Sort(out)
	return out
}

// ManifestVersion is the version polix writes in the manifests of the plugin for the binary of Version. A release
// writes its own version. A development build is always "dev", and Claude Code only copies a plugin to its cache
// when its version changes, so it writes 0.0.0-dev+<hash of the plugin> instead.
func (p Plugin) ManifestVersion() string {
	if semver.IsRelease(p.Version) {
		return p.Version
	}
	return "0.0.0-dev+" + p.hash()
}

// Description is the description of the local marketplace that holds the plugin of the binary.
func (p Plugin) Description() string {
	return "The plugin of polix " + p.Version + ", written by polix install from its binary: do not edit."
}

// hash is a short hash of the paths and contents of the plugin embedded in the binary, walked as a tree of
// folders sorted by name.
func (p Plugin) hash() string {
	h := sha256.New()
	for _, f := range p.walkOrder() {
		data := p.Embedded[f]
		h.Write([]byte(f + "\x00" + strconv.Itoa(len(data)) + "\x00"))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// walkOrder returns the paths of the plugin embedded in the binary in the order a walk of their tree visits them:
// the entries of each folder sorted by name, a folder before the next entry.
func (p Plugin) walkOrder() []string {
	paths := make([]string, 0, len(p.Embedded))
	for f := range p.Embedded {
		paths = append(paths, f)
	}
	slices.SortFunc(paths, func(a, b string) int {
		return slices.Compare(strings.Split(a, "/"), strings.Split(b, "/"))
	})
	return paths
}
