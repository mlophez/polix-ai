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

// component is a skill or an agent of the plugin: its path, the name it must carry, its frontmatter and its body.
type component struct {
	path, name, front, body string
}

// field returns the value of a key of the frontmatter, joining the lines of a folded value and collapsing their
// whitespace, so a sentence wrapped across lines matches as one.
func (c component) field(key string) (string, bool) {
	lines := strings.Split(c.front, "\n")
	for i, l := range lines {
		value, ok := strings.CutPrefix(l, key+":")
		if !ok {
			continue
		}
		parts := []string{strings.TrimSpace(value)}
		if parts[0] == ">" || parts[0] == "|" {
			parts = nil
			for _, next := range lines[i+1:] {
				if !strings.HasPrefix(next, " ") {
					break
				}
				parts = append(parts, next)
			}
		}
		return strings.Join(strings.Fields(strings.Join(parts, " ")), " "), true
	}
	return "", false
}

// components reads every skill or every agent of the plugin and fails when one has no frontmatter.
func components(t *testing.T, kind string) []component {
	t.Helper()
	entries, err := fs.ReadDir(FS, "polix/"+kind)
	if err != nil || len(entries) == 0 {
		t.Fatalf("the plugin has %s: %v", kind, err)
	}
	var out []component
	for _, e := range entries {
		c := component{path: path.Join("polix", kind, e.Name()), name: strings.TrimSuffix(e.Name(), ".md")}
		if kind == "skills" {
			c.path = path.Join(c.path, "SKILL.md")
		}
		data, err := FS.ReadFile(c.path)
		if err != nil {
			t.Fatal(err)
		}
		front, body, ok := strings.Cut(strings.TrimPrefix(string(data), "---\n"), "\n---\n")
		if !strings.HasPrefix(string(data), "---\n") || !ok {
			t.Fatalf("%s: no frontmatter", c.path)
		}
		c.front, c.body = front, body
		out = append(out, c)
	}
	return out
}

// The name in the frontmatter of every skill and agent is the name of its folder or file, which is how the agents
// invoke it.
func TestFrontmatterNames(t *testing.T) {
	for _, c := range append(components(t, "skills"), components(t, "agents")...) {
		name, _ := c.field("name")
		if _, ok := c.field("description"); name != c.name || !ok {
			t.Errorf("%s: the frontmatter needs name: %s and a description", c.path, c.name)
		}
	}
}

// Every skill and every agent runs only when the user or the workflow of polix asks for it: its description says so
// with a fixed sentence and invites the model to run it by itself with no other.
func TestInvocation(t *testing.T) {
	check := func(c component, want, banned string) {
		d, _ := c.field("description")
		if !strings.Contains(d, want) {
			t.Errorf("%s: the description needs the sentence %q", c.path, want)
		}
		if strings.Contains(d, banned) {
			t.Errorf("%s: the description must not say %q: only the user or the workflow of polix runs it", c.path, banned)
		}
	}
	for _, c := range components(t, "skills") {
		check(c, "It runs only when the user or the workflow of polix asks for it.", "Use it")
	}
	for _, c := range components(t, "agents") {
		check(c, "Launch it only when the user or the workflow of polix asks for it", "Launched by")
	}
}

// Every skill and agent names the invariants of its whole task Limits, never Rules, which in polix are the rules of
// the folder of rules.
func TestLimitsSection(t *testing.T) {
	for _, c := range append(components(t, "skills"), components(t, "agents")...) {
		if strings.Contains("\n"+c.body, "\n## Rules\n") {
			t.Errorf("%s: name the section ## Limits, not ## Rules", c.path)
		}
	}
}

// Every agent that cannot write a file reports, and its report follows a template inside its own file, since an
// agent launched by polix cannot read files next to it.
func TestAgentReportTemplates(t *testing.T) {
	for _, c := range components(t, "agents") {
		tools, ok := c.field("tools")
		if !ok || strings.Contains(tools, "Edit") || strings.Contains(tools, "Write") {
			continue
		}
		if !strings.Contains("\n"+c.body, "\n### Report template\n") {
			t.Errorf("%s: an agent without Edit nor Write reports, and needs a ### Report template section", c.path)
		}
	}
}

// Every skill that saves a report, a plan or a task writes it from a template.md next to its SKILL.md.
func TestSkillFileTemplates(t *testing.T) {
	for _, c := range components(t, "skills") {
		body := "\n" + c.body
		if !strings.Contains(body, "\n### Report file\n") && !strings.Contains(body, "\n### Plan file\n") &&
			!strings.Contains(body, "\n### Task file\n") {
			continue
		}
		if _, err := fs.Stat(FS, path.Join(path.Dir(c.path), "template.md")); err != nil {
			t.Errorf("%s: a skill that saves a file needs a template.md next to it: %v", c.path, err)
		}
	}
}

func TestFiles(t *testing.T) {
	files, err := Files()
	if err != nil || len(files["polix/.claude-plugin/plugin.json"]) == 0 {
		t.Fatal(len(files), err)
	}
}
