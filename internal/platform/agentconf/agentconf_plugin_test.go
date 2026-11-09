package agentconf

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

var plugin = map[string][]byte{
	"polix/.claude-plugin/plugin.json": []byte(`{"name": "polix", "version": "0", "description": "the plugin"}`),
	"polix/.codex-plugin/plugin.json":  []byte(`{"name": "polix", "version": "0", "skills": "./skills/"}`),
	"polix/skills/add-rule/SKILL.md":   []byte("---\nname: add-rule\n---\n"),
}

// The marketplace holds the plugin with the version in its manifests, and a manifest for each agent, read with the
// exact names of their fields.
func TestMarketplace(t *testing.T) {
	files, err := Marketplace("polix", "1.2.3", "written by polix", plugin)
	if err != nil {
		t.Fatal(err)
	}
	var cm struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Owner       struct {
			Name string `json:"name"`
		} `json:"owner"`
		Plugins []struct {
			Source      string `json:"source"`
			Description string `json:"description"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(files[".claude-plugin/marketplace.json"], &cm); err != nil || cm.Name != "polix" ||
		cm.Description != "written by polix" || cm.Owner.Name == "" || len(cm.Plugins) != 1 ||
		cm.Plugins[0].Source != "./polix" || cm.Plugins[0].Description != "the plugin" {
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
	if got := string(files["polix/.codex-plugin/plugin.json"]); got != "{\n  \"name\": \"polix\",\n  \"version\": \"1.2.3\",\n  \"skills\": \"./skills/\"\n}\n" {
		t.Fatalf("both manifests carry the version, in their order: %s", got)
	}
	if name, v, err := ReadManifest(files["polix/.claude-plugin/plugin.json"]); err != nil || name != "polix" || v != "1.2.3" {
		t.Fatal(name, v, err)
	}
	for _, bad := range []string{`{}`, `[]`} {
		if _, err := Marketplace("polix", "1", "", map[string][]byte{"polix/.claude-plugin/plugin.json": []byte(bad)}); err == nil ||
			!strings.HasPrefix(err.Error(), "polix/.claude-plugin/plugin.json: ") {
			t.Fatalf("a manifest without version: %v", err)
		}
	}
	if _, _, err := ReadManifest([]byte("x")); err == nil {
		t.Fatal("a manifest that is not JSON")
	}
}

// The files of the plugin are walked as a tree of folders sorted by name.
func TestWalkOrder(t *testing.T) {
	got := walkOrder(map[string][]byte{"a/x": nil, "a-b/x": nil, "a/b/y": nil})
	if !slices.Equal(got, []string{"a/b/y", "a/x", "a-b/x"}) {
		t.Fatal(got)
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
