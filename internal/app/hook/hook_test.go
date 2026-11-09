package hook

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// A call needs a known agent and events polix evaluates.
func TestHookValid(t *testing.T) {
	stop := agent.Event{Name: workflow.Stop, Agent: agent.Claude}
	if err := (Hook{Agent: agent.Agent{Name: agent.Claude}, Events: []agent.Event{stop}}).Valid(); err != nil {
		t.Fatal(err)
	}
	if (Hook{Agent: agent.Agent{Name: "x"}}).Valid() == nil {
		t.Fatal("an unknown agent is valid")
	}
	if (Hook{Agent: agent.Agent{Name: agent.Claude}, Events: []agent.Event{{Name: "x"}}}).Valid() == nil {
		t.Fatal("an unknown event is valid")
	}
}

// The repository is looked for from the dir asked, else the cwd of the call, else the working directory.
func TestHookFolder(t *testing.T) {
	h := Hook{Events: []agent.Event{{Cwd: "/repo/src"}}}
	if h.Folder("/other") != "/other" || h.Folder("") != "/repo/src" || (Hook{}).Folder("") != "" {
		t.Fatal("Folder")
	}
}
