// Package setup installs what an agent needs to run polix, from the binary.
//
// For the user it installs the plugin polix (plugins/polix of the repository, embedded in the binary): its skills
// and, for Claude Code, its agents, which help in any repository. The plugin is written to a local marketplace,
// ~/.polix/marketplace/, and registered with each agent:
//   - Claude Code: `claude plugin marketplace add` and `claude plugin install polix@polix`, for the user. Claude Code
//     copies the plugin to its cache, so a rewrite of the folder needs `claude plugin update` to reach it.
//   - Codex: `codex plugin marketplace add` and `[plugins."polix@polix"] enabled = true` in ~/.codex/config.toml.
//
// For a repository it registers the hooks that call `polix hook`, which belong to the project like its polix.yaml
// and are committed with it: in <repo>/.claude/settings.json and <repo>/.codex/hooks.json, merged with every other
// setting and hook of the file, whose order is kept. Codex skips new or changed hooks until the user trusts them
// with /hooks.
//
// A new release brings its plugin and hooks with it: install rewrites what changed, removes what the release
// dropped and leaves the rest alone.
package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Agents polix installs for.
const (
	Claude = "claude"
	Codex  = "codex"
)

// HookTimeout is the timeout, in seconds, of every `polix hook` call: above the run budget of a call (10 minutes),
// so the agent never kills a gate in the middle and lets the action through.
const HookTimeout = 900

// updateTimeout bounds the session start check: an update downloads the binary.
const updateTimeout = 180

// UpdateCommand is the session start command that keeps polix up to date and reports the problems of the machine:
// polix update --hook ends with the doctor of the binary it leaves. It is one plain command, with no shell syntax,
// because it is committed in the hooks files of the repositories, which every release, and every shell an agent
// runs hooks with, must understand; what it does lives in the binary.
const UpdateCommand = "polix update --hook"

// legacyCommands are the commands earlier builds registered, recognized so install replaces them.
var legacyCommands = []string{"polix update --hook; polix doctor --hook"}

// claudeTools are the tools whose use polix governs in Claude Code.
var claudeTools = "Edit|Write|MultiEdit|NotebookEdit|Bash"

// Options says where to install.
type Options struct {
	// Home is the home folder of the user: the plugin goes below it.
	Home string
	// Project is the root of the repository whose hooks to register; empty registers none.
	Project string
	// Version is the version of polix, written in the manifests of the plugin.
	Version string
}

// HooksFile is the file of the project where the hooks of an agent are registered.
func (o Options) HooksFile(agent string) string {
	if agent == Codex {
		return filepath.Join(o.Project, ".codex", "hooks.json")
	}
	return filepath.Join(o.Project, ".claude", "settings.json")
}

// Change is what install or uninstall did with one path, or with the registration of the plugin in an agent.
type Change struct {
	Path   string
	Action string // created, updated, removed, registered, unregistered, or todo: a command to run by hand
}

// hookGroup and hookHandler are the format of hooks of Claude Code and Codex: event, groups with a matcher, handlers.
type hookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookHandler `json:"hooks"`
}

type hookHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// Hooks returns the hooks polix registers for an agent, by event.
func Hooks(agent string) map[string][]hookGroup {
	call := hookHandler{Type: "command", Command: "polix hook " + agent, Timeout: HookTimeout}
	one := func(matcher string) []hookGroup { return []hookGroup{{Matcher: matcher, Hooks: []hookHandler{call}}} }
	tools := claudeTools
	if agent == Codex {
		tools = "" // Codex names its tools differently (shell, apply_patch...): polix reads every call
	}
	hooks := map[string][]hookGroup{
		"SessionStart": append(one(""), hookGroup{Matcher: "startup",
			Hooks: []hookHandler{{Type: "command", Command: UpdateCommand, Timeout: updateTimeout}}}),
		"UserPromptSubmit": one(""),
		"PreToolUse":       one(tools),
		"PostToolUse":      one(tools),
		"SubagentStop":     one(""),
		"Stop":             one(""),
	}
	return hooks
}

// IsPolixCommand reports whether a hook command is one polix registers.
func IsPolixCommand(command string) bool {
	c := strings.TrimSpace(command)
	return strings.HasPrefix(c, "polix hook ") || IsUpdateCommand(c)
}

// IsUpdateCommand reports whether a hook command is the session start command of polix, of this release, of an
// earlier one or of a later one that adds flags: install replaces it instead of adding a second update next to it.
func IsUpdateCommand(command string) bool {
	c := strings.TrimSpace(command)
	return c == UpdateCommand || strings.HasPrefix(c, UpdateCommand+" ") || strings.HasPrefix(c, UpdateCommand+";")
}

// IsLegacyCommand reports whether a hook command is one an earlier build registered and install now replaces.
func IsLegacyCommand(command string) bool {
	return slices.Contains(legacyCommands, strings.TrimSpace(command))
}

// hookEvents are the events polix registers, in lifecycle order: the order new events are added to a file.
var hookEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStop", "Stop"}

// Install writes the plugin polix and registers it with each agent for the user and, when o.Project is set,
// registers the hooks of each agent in the project. It returns what changed.
func Install(o Options, agents []string) ([]Change, error) {
	if err := checkAgents(agents); err != nil {
		return nil, err
	}
	changes, err := WritePlugin(o)
	if err != nil {
		return changes, err
	}
	// An agent that refuses the plugin does not keep the hooks of the repository from being registered.
	var errs []error
	for _, a := range agents {
		ch, err := registerPlugin(o, a)
		changes, errs = append(changes, ch...), append(errs, err)
		if o.Project != "" {
			ch, err := RegisterHooks(o, a)
			changes, errs = append(changes, ch...), append(errs, err)
		}
	}
	return changes, errors.Join(errs...)
}

// Uninstall unregisters the plugin polix from each agent, removes its folder when no agent is left and, when
// o.Project is set, removes the hooks of polix of the project, keeping every other hook. It returns what changed.
func Uninstall(o Options, agents []string) ([]Change, error) {
	if err := checkAgents(agents); err != nil {
		return nil, err
	}
	var changes []Change
	for _, a := range agents {
		ch, err := unregisterPlugin(o, a)
		changes = append(changes, ch...)
		if err != nil {
			return changes, err
		}
		if o.Project != "" {
			ch, err := mergeHooks(o.HooksFile(a), nil)
			changes = append(changes, ch...)
			if err != nil {
				return changes, err
			}
		}
	}
	if slices.Contains(agents, Claude) && slices.Contains(agents, Codex) {
		ch, err := removePlugin(o)
		changes = append(changes, ch...)
		if err != nil {
			return changes, err
		}
	}
	return changes, nil
}

// RegisterHooks writes the hooks of polix for an agent in the hooks file of the project.
func RegisterHooks(o Options, agent string) ([]Change, error) {
	return mergeHooks(o.HooksFile(agent), Hooks(agent))
}

func checkAgents(agents []string) error {
	for _, a := range agents {
		if a != Claude && a != Codex {
			return fmt.Errorf("unknown agent %q (claude, codex)", a)
		}
	}
	return nil
}

// ReadHooks returns the hooks of a settings file of Claude Code or a hooks file of Codex, by event; a missing file
// has none.
func ReadHooks(file string) (map[string][]hookGroup, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Hooks map[string][]hookGroup `json:"hooks"`
	}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
	}
	return doc.Hooks, nil
}

// Registered reports whether a hooks file registers any hook of polix.
func Registered(file string) bool {
	hooks, _ := ReadHooks(file)
	for _, groups := range hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if IsPolixCommand(h.Command) {
					return true
				}
			}
		}
	}
	return false
}

// mergeHooks replaces the hooks of polix in a hooks file with want (nil removes them). Every other key, event and
// group of the file is kept in its order, and the groups of polix take the place of the ones they replace, so a
// file committed by a team only changes where polix changed. The file is not touched when nothing changes, and
// removed when polix was all it held.
func mergeHooks(file string, want map[string][]hookGroup) ([]Change, error) {
	old, err := os.ReadFile(file)
	existed := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if !existed && want == nil {
		return nil, nil
	}
	top, err := decodeObject(old)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	hi := -1
	var events []member
	for i, m := range top {
		if m.key == "hooks" {
			hi = i
			if events, err = decodeObject(m.val); err != nil {
				return nil, fmt.Errorf("%s: hooks: %w", file, err)
			}
		}
	}

	ours := func(ev string) []json.RawMessage {
		var out []json.RawMessage
		for _, g := range want[ev] {
			out = append(out, marshal(g))
		}
		return out
	}
	done := map[string]bool{}
	var merged []member
	for _, ev := range events {
		var groups []json.RawMessage
		if err := json.Unmarshal(ev.val, &groups); err != nil {
			return nil, fmt.Errorf("%s: hooks.%s: %w", file, ev.key, err)
		}
		var kept []json.RawMessage
		for _, g := range groups {
			if !isPolixGroup(g) {
				kept = append(kept, g)
			} else if !done[ev.key] {
				kept, done[ev.key] = append(kept, ours(ev.key)...), true
			}
		}
		if !done[ev.key] {
			kept, done[ev.key] = append(kept, ours(ev.key)...), true
		}
		if len(kept) == 0 && len(groups) > 0 {
			continue // only polix was there
		}
		merged = append(merged, member{ev.key, encodeArray(kept)})
	}
	for _, ev := range hookEvents {
		if !done[ev] && len(want[ev]) > 0 {
			merged = append(merged, member{ev, encodeArray(ours(ev))})
		}
	}

	next := slices.Clone(top)
	switch {
	case hi >= 0 && len(merged) == 0 && len(events) > 0:
		next = slices.Delete(next, hi, hi+1)
	case hi >= 0:
		next[hi].val = encodeObject(merged)
	case len(merged) > 0:
		next = append(next, member{"hooks", encodeObject(merged)})
	}
	data := pretty(encodeObject(next))
	if existed && bytes.Equal(data, pretty(encodeObject(top))) {
		return nil, nil
	}
	if want == nil && len(next) == 0 {
		if err := os.Remove(file); err != nil {
			return nil, err
		}
		return []Change{{file, "removed"}}, nil
	}
	if err := writeFile(file, data); err != nil {
		return nil, err
	}
	if existed {
		return []Change{{file, "updated"}}, nil
	}
	return []Change{{file, "created"}}, nil
}

// isPolixGroup reports whether every handler of a group of hooks is a command polix registers.
func isPolixGroup(raw json.RawMessage) bool {
	var g struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if json.Unmarshal(raw, &g) != nil || len(g.Hooks) == 0 {
		return false
	}
	for _, h := range g.Hooks {
		if !IsPolixCommand(h.Command) {
			return false
		}
	}
	return true
}

// member is a key of a JSON object with its value as written, so a file is rewritten in its own order.
type member struct {
	key string
	val json.RawMessage
}

// decodeObject reads a JSON object keeping the order of its keys; empty data is an empty object.
func decodeObject(data []byte) ([]member, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}
	var out []member
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		out = append(out, member{t.(string), v})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("data after the JSON object")
	}
	return out, nil
}

func encodeObject(ms []member) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range ms {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(marshal(m.key))
		b.WriteByte(':')
		b.Write(m.val)
	}
	b.WriteByte('}')
	return b.Bytes()
}

func encodeArray(items []json.RawMessage) json.RawMessage {
	b := []byte{'['}
	for i, it := range items {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, it...)
	}
	return append(b, ']')
}

// marshal encodes v without escaping <, > and &, which a shell command may hold.
func marshal(v any) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return bytes.TrimSpace(b.Bytes())
}

// pretty indents JSON with two spaces, as Claude Code and Codex write their files.
func pretty(raw []byte) []byte {
	var b bytes.Buffer
	_ = json.Indent(&b, raw, "", "  ")
	b.WriteByte('\n')
	return b.Bytes()
}

// syncDir makes dir hold exactly files (paths relative to it, with /): it writes the files that differ and removes
// the ones that are not wanted.
func syncDir(dir string, files map[string][]byte) ([]Change, error) {
	var changes []Change
	names := make([]string, 0, len(files))
	for rel := range files {
		names = append(names, rel)
	}
	sort.Strings(names)
	for _, rel := range names {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		old, err := os.ReadFile(p)
		switch {
		case err == nil && bytes.Equal(old, files[rel]):
			continue
		case err == nil:
			changes = append(changes, Change{p, "updated"})
		default:
			changes = append(changes, Change{p, "created"})
		}
		if err := writeFile(p, files[rel]); err != nil {
			return changes, err
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
			return changes, err
		}
		changes = append(changes, Change{p, "removed"})
		removeEmptyParents(filepath.Dir(p), dir)
	}
	return changes, nil
}

func removeEmptyParents(d, stop string) {
	for d != stop && strings.HasPrefix(d, stop) {
		if os.Remove(d) != nil {
			return
		}
		d = filepath.Dir(d)
	}
}

// writeFile writes through a temporary file and a rename, so an agent that starts meanwhile reads the old file
// or the new one, never half of it.
func writeFile(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), p)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
