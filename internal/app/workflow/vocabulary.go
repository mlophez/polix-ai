package workflow

// Neutral events, the same for every agent.
const (
	Start         = "start"
	Prompt        = "prompt"
	Skill         = "skill"          // a skill starts, typed by the user or invoked by the agent through the tool Skill
	SubagentStart = "subagent_start" // a subagent starts, with a new context; the actor is its type
	PreEdit       = "pre_edit"
	PostEdit      = "post_edit"
	PreCommand    = "pre_command"
	PostCommand   = "post_command"
	PreCommit     = "pre_commit"
	PostCommit    = "post_commit" // a commit of the agent succeeded
	SubagentStop  = "subagent_stop"
	Stop          = "stop" // the main session ends its turn; a subagent ends with subagent_stop
)

// Verbs of an action: the action of an entry holds exactly one.
const (
	VerbRun    = "run"
	VerbInject = "inject"
	VerbBlock  = "block"
	VerbNotify = "notify"
)

// Events lists every neutral event in lifecycle order.
var Events = []string{Start, Prompt, Skill, SubagentStart, PreEdit, PostEdit, PreCommand, PostCommand, PreCommit,
	PostCommit, SubagentStop, Stop}

// Actions are the verbs an action can hold, exactly one per entry.
var Actions = []string{VerbRun, VerbInject, VerbBlock, VerbNotify}

// Conditions a `when` or `unless` can use.
var Conditions = []string{"actor", "skill", "started", "finished", "finished_since_commit", "agents_are_running", "branch", "var",
	"path", "changed", "file", "contains", "command", "writes", "exit", "not", "any", "changed_lines", "changed_files", "risk_score"}

// Measures are the conditions that measure the uncommitted diff, over the files of the nearest `changed`.
var Measures = []string{"changed_lines", "changed_files", "risk_score"}

// Builtins are the placeholders every string may use; a variable cannot take their names. commit and subject, the
// short hash and the subject of the commit just made, expand only on post_commit, and skill, the name of the skill
// that starts, only on skill. root, the absolute path of the repository root, holds on every event; abs_path, the
// edited path as an absolute path, on the edits; agent, session, actor and event, the context of the event, on every
// agent event; and exit_code, the exit code of a command, on post_command and post_commit. placeholderEvents limits
// each to its events.
var Builtins = []string{"path", "command", "branch", "changed_lines", "changed_files", "risk_score", "commit",
	"subject", "skill", "root", "abs_path", "agent", "session", "actor", "event", "exit_code"}

// AgentOnlyBuiltins are the placeholders of Builtins that only an event of an agent can give, never a hook of git:
// path, command and abs_path, of the edit or the command line of an agent, commit and subject, of a commit already
// made (post_commit), skill, of a skill that starts, and agent, session, actor, event and exit_code, of the event
// itself. The others, the root, the branch and the measures of the diff, git can compute.
var AgentOnlyBuiltins = []string{"path", "command", "commit", "subject", "skill", "abs_path", "agent", "session",
	"actor", "event", "exit_code"}

// conditionEvents limits the conditions that only make sense on some events; the rest hold on any event.
var conditionEvents = map[string][]string{
	"path":    {PreEdit, PostEdit},
	"command": {PreCommand, PostCommand, PreCommit, PostCommit},
	"writes":  {PreCommand, PostCommand, PreCommit, PostCommit},
	"exit":    {PostCommand},
	"skill":   {Skill},
}

// placeholderEvents limits the built-in placeholders that only expand on some agent events; the rest hold on every
// one. A placeholder written on another event would always be empty, so the validator rejects it.
var placeholderEvents = map[string][]string{
	"path":      {PreEdit, PostEdit},
	"command":   {PreCommand, PostCommand, PreCommit, PostCommit},
	"commit":    {PostCommit},
	"subject":   {PostCommit},
	"skill":     {Skill},
	"abs_path":  {PreEdit, PostEdit},
	"exit_code": {PostCommand, PostCommit},
}

// Informed are the events that already happened when polix hears of them: a block cannot stop them, and a failing
// run only reaches the AI as context. Neither agent can stop the start of a subagent; the start of a skill comes
// before it runs, so a block refuses it.
var Informed = []string{Start, SubagentStart, PostEdit, PostCommand, PostCommit}

// EventVerbs are the verbs each event of Events allows, in the order of Actions: what both agents support on it, so
// polix.yaml means the same in each. A block cannot stop the events that already happened (Informed), an inject
// cannot reach the agent on a stop, which takes no context, and a notify, a message for the user, works on every
// event.
var EventVerbs = map[string][]string{
	Start:         {VerbRun, VerbInject, VerbNotify},
	Prompt:        {VerbRun, VerbInject, VerbBlock, VerbNotify},
	Skill:         {VerbRun, VerbInject, VerbBlock, VerbNotify},
	SubagentStart: {VerbRun, VerbInject, VerbNotify},
	PreEdit:       {VerbRun, VerbInject, VerbBlock, VerbNotify},
	PostEdit:      {VerbRun, VerbInject, VerbNotify},
	PreCommand:    {VerbRun, VerbInject, VerbBlock, VerbNotify},
	PostCommand:   {VerbRun, VerbInject, VerbNotify},
	PreCommit:     {VerbRun, VerbInject, VerbBlock, VerbNotify},
	PostCommit:    {VerbRun, VerbInject, VerbNotify},
	SubagentStop:  {VerbRun, VerbBlock, VerbNotify},
	Stop:          {VerbRun, VerbBlock, VerbNotify},
}

// CommitMsg is the event of githooks git runs once the message of a commit is written (commit-msg); the other one,
// before the commit is made (pre-commit), takes the name of PreCommit.
const CommitMsg = "commit_msg"

// GithookEvents are the events of githooks, the gates git runs on every commit, apart from the events of the AI
// (Events), in the order git runs them.
var GithookEvents = []string{PreCommit, CommitMsg}

// GithookVerbs are the verbs a githooks entry can hold, in the order of Actions.
var GithookVerbs = []string{VerbRun, VerbBlock}

// GithookConditions are the conditions a `when` or `unless` of githooks can use, in the order polix config fmt
// writes them; message holds only on CommitMsg (githookOnly).
var GithookConditions = []string{"branch", "changed", "message"}

// GithookBuiltins are the placeholders the strings of githooks may use besides the variables of vars; message
// expands only on CommitMsg (githookOnly).
var GithookBuiltins = []string{"branch", "message"}

// GithookRunKeys and GithookBlockKeys are the keys of the long form of run and block in githooks, in the order of
// RunKeys and BlockKeys: githooks keep no log, so a run runs on every commit (no always) and a block blocks every
// time (no once).
var (
	GithookRunKeys   = []string{"command", "message"}
	GithookBlockKeys = []string{"message"}
)

// githookOnly names the only event of githooks a condition or a placeholder holds on, for those that do not hold
// on every one.
var githookOnly = map[string]string{"message": CommitMsg}

// refusals tells why an event does not allow a verb, with the alternative, as a format of the event; a verb without
// one is told only the verbs the event allows.
var refusals = map[string]string{
	VerbBlock:  "block cannot stop %s, which already happened: use inject, or a run whose failure reaches the AI",
	VerbInject: "inject does not reach the agent on %s, which cannot receive context",
}
