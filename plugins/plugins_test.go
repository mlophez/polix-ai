package plugins

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// The reference of the workflow skill is the annotated example of the repository.
func TestWorkflowReference(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("..", "examples", "polix.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := FS.ReadFile("polix/skills/workflow/reference.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("plugins/polix/skills/workflow/reference.yaml differs from examples/polix.yaml: copy it again")
	}
}

// Both manifests name the plugin polix and carry a version, which install replaces with the one of the binary.
func TestManifests(t *testing.T) {
	for _, p := range []string{"polix/.claude-plugin/plugin.json", "polix/.codex-plugin/plugin.json"} {
		data, err := FS.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var m struct{ Name, Version, Description string }
		if err := json.Unmarshal(data, &m); err != nil || m.Name != "polix" || m.Version == "" || m.Description == "" {
			t.Fatalf("%s: %+v %v", p, m, err)
		}
	}
}

// The name in the frontmatter of every skill and agent is the name of its folder or file, which is how the agents
// invoke it.
func TestFrontmatterNames(t *testing.T) {
	check := func(p, want string) {
		data, err := FS.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		front, _, ok := strings.Cut(strings.TrimPrefix(string(data), "---\n"), "\n---\n")
		if !strings.HasPrefix(string(data), "---\n") || !ok {
			t.Fatalf("%s: no frontmatter", p)
		}
		if !strings.Contains("\n"+front+"\n", "\nname: "+want+"\n") || !strings.Contains(front, "description:") {
			t.Fatalf("%s: the frontmatter needs name: %s and a description", p, want)
		}
	}
	skills, _ := fs.ReadDir(FS, "polix/skills")
	for _, s := range skills {
		check(path.Join("polix/skills", s.Name(), "SKILL.md"), s.Name())
	}
	agents, _ := fs.ReadDir(FS, "polix/agents")
	for _, a := range agents {
		check(path.Join("polix/agents", a.Name()), strings.TrimSuffix(a.Name(), ".md"))
	}
	if len(skills) == 0 || len(agents) == 0 {
		t.Fatal("the plugin has skills and agents")
	}
}

// The marketplaces of the repository serve the same plugin folder.
func TestRepositoryMarketplaces(t *testing.T) {
	for _, p := range []string{filepath.Join("..", ".claude-plugin", "marketplace.json"), filepath.Join("..", ".agents", "plugins", "marketplace.json")} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"./plugins/polix"`) {
			t.Fatalf("%s does not point to ./plugins/polix", p)
		}
	}
}
