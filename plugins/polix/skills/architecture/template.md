# Architecture of <scope>

<The date as YYYY-MM-DD.
The base: `git rev-parse --short HEAD`, the branch and whether the working tree had uncommitted changes.
The scope and the dimensions measured, and whether the scores hold only for the scope.>

## Summary

<One line per dimension measured: `<dimension>: <score>/5`, or `unscored`, and its reason.
Then the agent traps and the unmodeled cases found, and how many have `no mechanical guard`.
Then the three recommendations that matter most, each with the solution recommended.>

## Metrics

<Graph: the modules and edges, every cycle, and the modules with the highest fan-in, fan-out and instability.
External reach: each external dependency and the modules that import it.
History: the commits sampled, the hotspots, the co-change pairs without an import, the spread and the fix
concentration.
Each metric with the command that produced it, or `approximate`.>

## Dimensions

<One subsection per dimension measured, in the order of the rubric:

### <Dimension>: <score>/5

<Why it meets that level and not the next, the metrics it rests on and each finding at `file:line`.>

Weakness: <the main weakness, at `file:line`: what holds it below 5, or at 5, what would bring it down first>.

<In conformance, also the challenge of the declared architecture: each constraint that protects nothing, costs more
than it protects, contradicts another or is bypassed, with the constraint or rule it names.>>

## Agent traps

<One subsection per trap, ranked: confirmed by the history first, then `no mechanical guard`, then the rest:

### T<n>. <title> (<kind>)

Where: <`file:line`>.
Scenario: <the change an agent would make, what breaks and why no check fails>.
Confirmed by: <the commit, or none>.
Guard: <the first viable guard of the rubric, or `no mechanical guard` and the note it needs>.>

<Or "none" with the kinds walked.>

## Unmodeled cases

<One subsection per case, ranked as the traps:

### U<n>. <title> (<family>)

Case: <what happens and when>.
Owner: <the module that should handle it and why none does today, at `file:line`>.
Guard: <the first viable guard of the rubric, or `no mechanical guard` and the note it needs>.>

<Or "none" with the families walked.>

## Strengths

<Only what a recommendation could undo and must be kept, each with its evidence at `file:line`.>

## Recommendations

<One subsection per recommendation, sorted by priority, then by dimensions improved, then by effort:

### R<n>. <title> (<priority>)

Dimensions: <the dimensions it improves>.
Evidence: <the findings at `file:line` and the metrics>.
Impact: <what it costs today and what it unlocks>.
Effort: <low, medium or high>.

<For a high one, two or three solutions:

- **A. <idea>**: <how it works>.
  Pros: <what it gains>. Cons: <what it costs>. Effort: <low, medium or high>. Risk: <low, medium or high>.

Recommended: <the letter>, because <reason>.
A sketch of the recommended one, before and after, in the language of the project.>

<For any other one: Direction: <the change to make, in one or two lines>.>>

## Coverage

<The modules read whole, those read only by their surface, and those left out and why.>

## Pending

<What could not be measured and why: a missing tool, a short history, a part too large to read.>
