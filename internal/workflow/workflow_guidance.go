package workflow

// Guidance is the key `guidance` of polix.yaml: the built-in instructions polix injects into the agent when a
// session starts, each one turned on or off. They ask the agent to work in the forms the heuristics of polix
// recognise; they block nothing.
type Guidance struct {
	Commit bool     // run each commit as a git commit of its own, so pre_commit recognises it
	Edit   bool     // change files with the edit tool of the agent, so pre_edit and post_edit see them
	Keys   []string // the keys the file used, so Valid rejects unknown ones
}

// The texts of the guidance, agent neutral, as the agent reads them.
const (
	GuidanceCommit = "polix: run each commit as a git commit command of its own, not chained with &&, ; or | " +
		"nor wrapped in sh -c or bash -c, so polix recognises it and applies its pre_commit gates. " +
		"The message may go by -m, -F or a heredoc inside -m."
	GuidanceEdit = "polix: create and change the files of the repository with the edit tool of the agent, never " +
		"from the shell (sed -i, perl -i, > or >>, tee, heredocs or scripts that write files): polix applies " +
		"its edit rules and checks only to what goes through that tool."
)

// GuidanceKeys are the keys of guidance, in the order their texts are injected.
var GuidanceKeys = []string{"commit", "edit"}

// Valid tells why the guidance is not valid: an unknown key.
func (g Guidance) Valid() error {
	v := &validator{}
	v.guidance(g)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// Instructions returns the texts of the guidance turned on, commit first; none when every switch is off.
func (g Guidance) Instructions() []string {
	var out []string
	if g.Commit {
		out = append(out, GuidanceCommit)
	}
	if g.Edit {
		out = append(out, GuidanceEdit)
	}
	return out
}

func (v *validator) guidance(g Guidance) {
	v.unknownKeys(0, "guidance", g.Keys, GuidanceKeys)
}
