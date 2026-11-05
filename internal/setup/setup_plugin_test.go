package setup

import (
	"encoding/json"
	"errors"
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
	if !slices.Equal(walkOrder(files), order) {
		t.Fatal("walkOrder is the order of fs.WalkDir")
	}
	return files
}

func TestPluginNames(t *testing.T) {
	e := embedded(t)
	if got := strings.Join(PluginSkills(e), " "); got != "add-rule audit doctor init plan review workflow" {
		t.Fatalf("skills: %s", got)
	}
	if got := strings.Join(PluginAgents(e), " "); got != "auditor reviewer rules-author rules-reviewer workflow-author" {
		t.Fatalf("agents: %s", got)
	}
}

// A development build is always "dev", so the plugin carries a hash of its contents as version.
func TestManifestVersion(t *testing.T) {
	e := embedded(t)
	if v := ManifestVersion(e, "1.2.3"); v != "1.2.3" {
		t.Fatalf("a release writes its own version: %s", v)
	}
	dev := ManifestVersion(e, "dev")
	if !strings.HasPrefix(dev, "0.0.0-dev+") || len(dev) != len("0.0.0-dev+")+12 || ManifestVersion(e, "") != dev {
		t.Fatalf("a development build writes 0.0.0-dev+<hash>: %s", dev)
	}
	files, err := MarketplaceFiles(e, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := PluginVersion(files["polix/.claude-plugin/plugin.json"]); !ok || v != dev ||
		!strings.Contains(string(files["polix/.codex-plugin/plugin.json"]), `"version": "`+dev+`"`) {
		t.Fatalf("both manifests carry it: %s", v)
	}
}

// The marketplace holds the plugin and a manifest for each agent, read with the exact names of their fields.
func TestMarketplaceFiles(t *testing.T) {
	files, err := MarketplaceFiles(embedded(t), "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	var cm struct {
		Name  string `json:"name"`
		Owner struct {
			Name string `json:"name"`
		} `json:"owner"`
		Plugins []struct {
			Source      string `json:"source"`
			Description string `json:"description"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(files[".claude-plugin/marketplace.json"], &cm); err != nil || cm.Name != Name ||
		cm.Owner.Name == "" || len(cm.Plugins) != 1 || cm.Plugins[0].Source != "./polix" || cm.Plugins[0].Description == "" {
		t.Fatalf("marketplace of Claude Code: %+v %v", cm, err)
	}
	var xm struct {
		Plugins []struct {
			Source struct {
				Path string `json:"path"`
			} `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(files[".agents/plugins/marketplace.json"], &xm); err != nil || len(xm.Plugins) != 1 ||
		xm.Plugins[0].Source.Path != "./polix" {
		t.Fatalf("marketplace of Codex: %+v %v", xm, err)
	}
	if !strings.Contains(string(files["polix/skills/add-rule/SKILL.md"]), "name: add-rule\n") {
		t.Fatal("the skills")
	}
	if _, err := MarketplaceFiles(map[string][]byte{"polix/.claude-plugin/plugin.json": []byte(`{}`)}, "1"); err == nil {
		t.Fatal("a manifest without version")
	}
	if _, ok := PluginVersion([]byte(`{"name": "other", "version": "1"}`)); ok {
		t.Fatal("only the plugin polix")
	}
}

func TestClaudeAnswer(t *testing.T) {
	args := []string{"plugin", "install"}
	o, inst, avail, err := ClaudeAnswer(args, "noise\n"+`{"outcome":"ok","installedVersion":"1","availableVersion":"2"}`, nil, nil)
	if err != nil || o != "ok" || inst != "1" || avail != "2" {
		t.Fatal(o, inst, avail, err)
	}
	failed := `{"outcome":"failed","failureCode":"not_installed","message":"nope"}`
	if _, _, _, err := ClaudeAnswer(args, failed, errors.New("exit 1"), []string{"not_installed"}); err != nil {
		t.Fatalf("tolerated: %v", err)
	}
	if _, _, _, err := ClaudeAnswer(args, failed, nil, nil); err == nil || err.Error() != "claude plugin install: nope" {
		t.Fatalf("failure: %v", err)
	}
	if _, _, _, err := ClaudeAnswer(args, "garbage", nil, nil); err == nil || !strings.Contains(err.Error(), "unexpected output") {
		t.Fatalf("garbage: %v", err)
	}
}

func TestClaudePlugin(t *testing.T) {
	inst, en, folder, err := ClaudePlugin([]byte(`[{"id":"polix@polix","scope":"user","enabled":true,"readFromFolder":"/f"}]`))
	if !inst || !en || folder != "/f" || err != nil || ClaudeFolder([]byte("[]")) != "" {
		t.Fatal(inst, en, folder, err)
	}
	if _, _, _, err := ClaudePlugin([]byte("x")); err == nil || !strings.HasPrefix(err.Error(), "claude plugin list: ") {
		t.Fatal(err)
	}
}

func TestTodos(t *testing.T) {
	if got := ClaudeTodo("/a b"); !strings.Contains(got, `add "/a b" --scope user && claude plugin install polix@polix`) {
		t.Fatal(got)
	}
	if got := CodexTodo("/m", errors.New("boom\nmore")); got != "codex plugin marketplace add /m (it failed: boom)" {
		t.Fatal(got)
	}
	if !CodexAdded("", errors.New("already added")) || CodexAdded("", errors.New("boom")) || !CodexAdded("", nil) {
		t.Fatal("added")
	}
}
