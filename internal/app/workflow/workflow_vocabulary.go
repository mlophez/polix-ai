package workflow

// Neutral events, the same for every agent.
const (
	Start         = "start"
	Prompt        = "prompt"
	SubagentStart = "subagent_start" // a subagent starts, with a new context; the actor is its type
	PreEdit       = "pre_edit"
	PostEdit      = "post_edit"
	PreCommand    = "pre_command"
	PostCommand   = "post_command"
	PreCommit     = "pre_commit"
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
var Events = []string{Start, Prompt, SubagentStart, PreEdit, PostEdit, PreCommand, PostCommand, PreCommit,
	SubagentStop, Stop}

// Actions are the verbs an action can hold, exactly one per entry.
var Actions = []string{VerbRun, VerbInject, VerbBlock, VerbNotify}

// Conditions a `when` or `unless` can use.
var Conditions = []string{"actor", "finished", "agents_are_running", "branch", "var", "path", "changed", "file",
	"contains", "command", "writes", "exit", "not", "any", "changed_lines", "changed_files", "risk_score"}

// Measures are the conditions that measure the uncommitted diff, over the files of the nearest `changed`.
var Measures = []string{"changed_lines", "changed_files", "risk_score"}

// Builtins are the placeholders every string may use; a variable cannot take their names.
var Builtins = []string{"path", "command", "branch", "changed_lines", "changed_files", "risk_score"}

// conditionEvents limits the conditions that only make sense on some events; the rest hold on any event.
var conditionEvents = map[string][]string{
	"path":    {PreEdit, PostEdit},
	"command": {PreCommand, PostCommand, PreCommit},
	"writes":  {PreCommand, PostCommand, PreCommit},
	"exit":    {PostCommand},
}

// Informed are the events that already happened when polix hears of them: a block cannot stop them, and a failing
// run only reaches the AI as context. Neither agent can stop the start of a subagent.
var Informed = []string{Start, SubagentStart, PostEdit, PostCommand}

// EventVerbs are the verbs each event of Events allows, in the order of Actions: what both agents support on it, so
// polix.yaml means the same in each. A block cannot stop the events that already happened (Informed), an inject
// cannot reach the agent on a stop, which takes no context, and a notify, a message for the user, works on every
// event.
var EventVerbs = map[string][]string{
	Start:         {VerbRun, VerbInject, VerbNotify},
	Prompt:        {VerbRun, VerbInject, VerbBlock, VerbNotify},
	SubagentStart: {VerbRun, VerbInject, VerbNotify},
	PreEdit:       {VerbRun, VerbInject, VerbBlock, VerbNotify},
	PostEdit:      {VerbRun, VerbInject, VerbNotify},
	PreCommand:    {VerbRun, VerbInject, VerbBlock, VerbNotify},
	PostCommand:   {VerbRun, VerbInject, VerbNotify},
	PreCommit:     {VerbRun, VerbInject, VerbBlock, VerbNotify},
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
