package format

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	ruleload "github.com/mlophez/polix-ai/internal/app/rule/load"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	workflowload "github.com/mlophez/polix-ai/internal/app/workflow/load"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// fakePort answers the port of FormatRule from memory, and records the folders read and the files written.
type fakePort struct {
	wf       workflow.Workflow
	wfErr    error
	files    map[string][]byte
	readErr  error
	read     []string
	written  []string
	data     map[string]string
	repeated []string
}

// TestMain runs the tests without the variables git exports to its hooks (see testenv.ClearGitHookEnv): the githooks
// of polix.yaml run go test, and these tests run git in repositories of their own.
func TestMain(m *testing.M) {
	testenv.ClearGitHookEnv()
	os.Exit(m.Run())
}

func (f *fakePort) RepeatedIDs(_ context.Context, _ string) []string {
	return f.repeated
}

func (f *fakePort) LoadWorkflow(_ context.Context, _ string) (workflow.Workflow, error) {
	return f.wf, f.wfErr
}

func (f *fakePort) ReadRuleFiles(_ context.Context, dir string) (map[string][]byte, error) {
	f.read = append(f.read, dir)
	return f.files, f.readErr
}

func (f *fakePort) WriteRuleFile(_ context.Context, dir, name string, data []byte) error {
	f.written = append(f.written, dir+"/"+name)
	if f.data == nil {
		f.data = map[string]string{}
	}
	f.data[dir+"/"+name] = string(data)
	return nil
}

// printed prints the response and returns its exit code, stdout and stderr.
func printed(res Response) (int, string, string) {
	var out, errb bytes.Buffer
	code := res.print(&out, &errb)
	return code, out.String(), errb.String()
}

// The shell reads the folder of rules below path and writes each changed file, in the order of the names, and
// nothing else.
func TestShellWrites(t *testing.T) {
	port := &fakePort{wf: workflow.Workflow{Path: "docs"}, files: map[string][]byte{
		"b.yaml": []byte(unformatted), "a.yaml": []byte(unformatted), "c.yaml": []byte(formatted)}}
	res, err := run(context.Background(), port, Request{})
	if err != nil || !slices.Equal(port.read, []string{"docs/rules"}) {
		t.Fatalf("got %+v %v, read %v", res, err, port.read)
	}
	want := []string{"docs/rules/a.yaml", "docs/rules/b.yaml"}
	if !slices.Equal(port.written, want) || !slices.Equal(res.Written, want) || port.data[want[0]] != formatted {
		t.Fatalf("written %v, response %+v", port.written, res)
	}
	code, stdout, stderr := printed(res)
	if code != 0 || stdout != "ok: formatted docs/rules/a.yaml\nok: formatted docs/rules/b.yaml\n" || stderr != "" {
		t.Fatalf("print: %d %q %q", code, stdout, stderr)
	}
}

// --check writes nothing and names each file that is not formatted, with exit code 1; formatted files are ok.
func TestShellCheck(t *testing.T) {
	port := &fakePort{files: map[string][]byte{"a.yaml": []byte(unformatted)}}
	res, err := run(context.Background(), port, Request{Check: true})
	if err != nil || port.written != nil || !slices.Equal(res.Changed, []string{"ai/rules/a.yaml"}) {
		t.Fatalf("got %+v %v", res, err)
	}
	code, stdout, stderr := printed(res)
	if code != 1 || stdout != "" || stderr != "✗ ai/rules/a.yaml is not formatted: run polix rules fmt\n" {
		t.Fatalf("print: %d %q %q", code, stdout, stderr)
	}
	port = &fakePort{files: map[string][]byte{"a.yaml": []byte(formatted)}}
	res, _ = run(context.Background(), port, Request{Check: true})
	if code, stdout, _ := printed(res); code != 0 || stdout != "ok: the rules are formatted\n" {
		t.Fatalf("formatted: %d %q", code, stdout)
	}
	res, _ = run(context.Background(), port, Request{})
	if code, stdout, _ := printed(res); code != 0 || stdout != "ok: the rules are formatted\n" || port.written != nil {
		t.Fatalf("formatted, without --check: %d %q %v", code, stdout, port.written)
	}
}

// A file that cannot be formatted leaves every file as it is, and its problem goes to stderr with exit code 1.
func TestShellProblem(t *testing.T) {
	port := &fakePort{files: map[string][]byte{"a.yaml": []byte(unformatted), "b.yaml": []byte("rules: [\n")}}
	res, err := run(context.Background(), port, Request{})
	if err != nil || port.written != nil || res.Written != nil || len(res.Problems) != 1 {
		t.Fatalf("got %+v %v", res, err)
	}
	code, stdout, stderr := printed(res)
	if code != 1 || stdout != "" || !strings.Contains(stderr, "✗ ai/rules/b.yaml: ") ||
		!strings.Contains(stderr, "ai/rules/a.yaml is not formatted: run polix rules fmt") {
		t.Fatalf("print: %d %q %q", code, stdout, stderr)
	}
}

// A repeated id of the loader of the rules leaves every file as it is, and goes to stderr with exit code 1.
func TestShellRepeatedIDs(t *testing.T) {
	repeat := "ai/rules/b.yaml A-1: id repeated, also in ai/rules/a.yaml"
	for _, check := range []bool{false, true} {
		port := &fakePort{files: map[string][]byte{"a.yaml": []byte(unformatted), "b.yaml": []byte(formatted)},
			repeated: []string{repeat}}
		res, err := run(context.Background(), port, Request{Check: check})
		if err != nil || port.written != nil || res.Written != nil {
			t.Fatalf("check %v: %+v %v", check, res, err)
		}
		code, stdout, stderr := printed(res)
		if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "✗ "+repeat+"\n") {
			t.Fatalf("check %v, print: %d %q %q", check, code, stdout, stderr)
		}
	}
}

// Without the folder, or without yaml files in it, there is nothing to format, with or without --check.
func TestShellEmpty(t *testing.T) {
	for _, check := range []bool{false, true} {
		res, err := run(context.Background(), &fakePort{}, Request{Check: check})
		if err != nil || !res.Empty || res.Dir != "ai/rules" {
			t.Fatalf("got %+v %v", res, err)
		}
		if code, stdout, _ := printed(res); code != 0 || stdout != "ok: no rules to format in ai/rules\n" {
			t.Fatalf("print: %d %q", code, stdout)
		}
	}
}

// A polix.yaml that cannot be loaded, or a path that cannot be the folder of the AI, stops the shell before
// any file is read; a workflow with another problem, as an invalid runner, is formatted. A folder that cannot be
// read stops it too.
func TestShellErrors(t *testing.T) {
	boom := errors.New("polix.yaml: yaml: broken")
	port := &fakePort{wfErr: boom, files: map[string][]byte{"a.yaml": []byte(unformatted)}}
	if _, err := run(context.Background(), port, Request{}); !errors.Is(err, boom) || port.read != nil ||
		port.written != nil {
		t.Fatalf("workflow: %v, read %v", err, port.read)
	}
	port = &fakePort{wf: workflow.Workflow{Path: "../x"},
		files: map[string][]byte{"a.yaml": []byte(unformatted)}}
	if _, err := run(context.Background(), port, Request{}); err == nil ||
		!strings.Contains(err.Error(), `path "../x" must be below the root`) || port.read != nil ||
		port.written != nil {
		t.Fatalf("path: %v, read %v", err, port.read)
	}
	port = &fakePort{wf: workflow.Workflow{Rules: workflow.Rules{Run: []workflow.Runner{{Glob: "*"}}}},
		files: map[string][]byte{"a.yaml": []byte(unformatted)}}
	if res, err := run(context.Background(), port, Request{}); err != nil || len(port.written) != 1 ||
		len(res.Written) != 1 {
		t.Fatalf("an invalid runner: %+v %v", res, err)
	}
	port = &fakePort{readErr: boom}
	if _, err := run(context.Background(), port, Request{}); !errors.Is(err, boom) || port.written != nil {
		t.Fatalf("read: %v", err)
	}
}

// The command takes no argument, and --dir names the repository whose rules it formats.
func TestCommand(t *testing.T) {
	root := testenv.GitRepo(t)
	file := filepath.Join(root, "ai", "rules", "a.yaml")
	testenv.Write(t, file, unformatted)
	fmtRules := func(args ...string) (int, string, string) {
		cmd := cli.Root("polix", "", "", "")
		cmd.AddCommand(Command(ruleload.Command(), workflowload.Command()))
		var out, errb bytes.Buffer
		code := cli.Run(cmd, append([]string{"fmt"}, args...), strings.NewReader(""), &out, &errb)
		return code, out.String(), errb.String()
	}
	if code, _, _ := fmtRules("x", "--dir", root); code == 0 {
		t.Fatal("an argument: want a failure")
	}
	if code, stdout, stderr := fmtRules("--dir", root); code != 0 || stdout != "ok: formatted ai/rules/a.yaml\n" {
		t.Fatalf("fmt: %d %q %q", code, stdout, stderr)
	}
	if got := testenv.Read(t, file); got != formatted {
		t.Fatalf("file: %q", got)
	}
	if code, _, stderr := fmtRules("--dir", t.TempDir()); code != 1 || stderr == "" {
		t.Fatalf("outside a repository: %d %q", code, stderr)
	}
}

// With the loaders that cmd/polix builds, a file that is yaml but does not decode as rules stops the loader before
// it can rule out a repeated id: the command reports its error, writes no file and exits with 1, with or without
// --check. A file that is not yaml is reported once, though both the format and the loader fail on it.
func TestCommandLoaderStops(t *testing.T) {
	root := testenv.GitRepo(t)
	rules := filepath.Join(root, "ai", "rules")
	src := map[string]string{"a.yaml": unformatted, "b.yaml": formatted,
		"c.yaml": "rules:\n  - id: C-1\n    description: c\n    scope: \"*.go\"\n"}
	for name, data := range src {
		testenv.Write(t, filepath.Join(rules, name), data)
	}
	fmtRules := func(args ...string) (int, string, string) {
		cmd := cli.Root("polix", "", "", "")
		cmd.AddCommand(Command(ruleload.Command(), workflowload.Command()))
		var out, errb bytes.Buffer
		code := cli.Run(cmd, append([]string{"fmt", "--dir", root}, args...), strings.NewReader(""), &out, &errb)
		return code, out.String(), errb.String()
	}
	for _, args := range [][]string{nil, {"--check"}} {
		code, stdout, stderr := fmtRules(args...)
		if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "✗ ai/rules/c.yaml: yaml: ") {
			t.Fatalf("%v: %d %q %q", args, code, stdout, stderr)
		}
		for name, data := range src {
			if got := testenv.Read(t, filepath.Join(rules, name)); got != data {
				t.Fatalf("%v: %s written: %q", args, name, got)
			}
		}
	}
	testenv.Write(t, filepath.Join(rules, "c.yaml"), "rules: [\n")
	code, _, stderr := fmtRules()
	if code != 1 || strings.Count(stderr, "ai/rules/c.yaml: ") != 1 {
		t.Fatalf("a file that is not yaml: %d %q", code, stderr)
	}
}
