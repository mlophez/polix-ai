package diagnose

import (
	"fmt"
	"path/filepath"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

// memory checks that the auto memory of Claude Code is off, as polix install leaves it in the settings of the
// project, so no note of an earlier session steers a session away from the rules of polix. It decides as Claude
// Code does, the first that sets it wins: the variable setup.AutoMemoryVariable (1 off, 0 on, any other value
// nothing), then the local settings, then the ones of the project, then the ones of the user; without any of them
// it is on. One finding, a warning when it is on naming what turns it on, which never fails the report; none when
// no settings register polix or one of them cannot be read or is not JSON, which the area hooks reports.
func memory(i setup.Inspection) []setup.Finding {
	failures, _, sources := claudeSettings(i)
	if len(failures) > 0 || len(sources) == 0 {
		return nil
	}
	switch i.AutoMemoryEnv {
	case "1":
		return memoryOff()
	case "0":
		return foundFix(setup.LevelWarn, "memory", setup.AutoMemoryVariable+"=0 turns auto memory of Claude Code on", "unset it")
	}
	project := i.Setup.HooksFile(setup.Claude)
	local := filepath.Join(filepath.Dir(project), "settings.local.json")
	user := filepath.Join(i.Setup.Home, ".claude", "settings.json")
	switch i.ClaudeHooks[local].AutoMemory {
	case "false":
		return memoryOff()
	case "true":
		return foundFix(setup.LevelWarn, "memory", local+" turns auto memory of Claude Code on",
			fmt.Sprintf("remove %s from it", setup.AutoMemoryKey))
	}
	value := i.ClaudeHooks[project].AutoMemory
	if value == "" && i.Setup.Home != "" {
		value = i.ClaudeHooks[user].AutoMemory
	}
	if value == "false" {
		return memoryOff()
	}
	return foundFix(setup.LevelWarn, "memory", "auto memory of Claude Code is on", "run polix install to turn it off in "+project)
}

// memoryOff is the finding of the auto memory of Claude Code off.
func memoryOff() []setup.Finding {
	return found(setup.LevelOK, "memory", "auto memory of Claude Code is off")
}
