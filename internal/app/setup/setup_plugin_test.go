package setup

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/plugins"
)

// embedded reads the plugin of the binary, as the adapters do.
func embedded(t *testing.T) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	var order []string
	err := fs.WalkDir(plugins.FS, Name, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		order = append(order, p)
		files[p], err = plugins.FS.ReadFile(p)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal((Plugin{Embedded: files}).walkOrder(), order) {
		t.Fatal("walkOrder is the order of fs.WalkDir")
	}
	return files
}

func TestPluginNames(t *testing.T) {
	p := Plugin{Embedded: embedded(t), Version: "1"}
	if got := strings.Join(p.Skills(), " "); got != "add-rule audit build doctor init plan review risk workflow" {
		t.Fatalf("skills: %s", got)
	}
	want := "auditor builder reviewer rules-author rules-reviewer verifier workflow-author"
	if got := strings.Join(p.Agents(), " "); got != want {
		t.Fatalf("agents: %s", got)
	}
}

// A development build is always "dev", so the plugin carries a hash of its contents as version.
func TestManifestVersion(t *testing.T) {
	e := embedded(t)
	if v := (Plugin{Embedded: e, Version: "1.2.3"}).ManifestVersion(); v != "1.2.3" {
		t.Fatalf("a release writes its own version: %s", v)
	}
	dev := (Plugin{Embedded: e, Version: "dev"}).ManifestVersion()
	if !strings.HasPrefix(dev, "0.0.0-dev+") || len(dev) != len("0.0.0-dev+")+12 || (Plugin{Embedded: e}).ManifestVersion() != dev {
		t.Fatalf("a development build writes 0.0.0-dev+<hash>: %s", dev)
	}
}

// The plugin written is current when its manifest carries the ManifestVersion of the binary.
func TestPluginCurrent(t *testing.T) {
	p := Plugin{Written: true, Manifest: "1.2.3", Embedded: embedded(t), Version: "1.2.3"}
	if p.Valid() != nil || !p.Current() {
		t.Fatal("the plugin of the binary")
	}
	older := Plugin{Written: true, Manifest: "1.2.2", Version: "1.2.3"}
	none := Plugin{Version: "1.2.3"}
	if older.Current() || none.Current() || none.Valid() != nil {
		t.Fatal("an older plugin, or none")
	}
	if (Plugin{}).Valid() == nil || (Plugin{Manifest: "1", Version: "1"}).Valid() == nil {
		t.Fatal("a plugin needs the version, and only a written one has a manifest")
	}
	if !strings.Contains(p.Description(), "polix 1.2.3") {
		t.Fatal(p.Description())
	}
}
