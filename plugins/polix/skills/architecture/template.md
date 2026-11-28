# Architecture of <scope>

<The date as YYYY-MM-DD.
The base: `git rev-parse --short HEAD`, the branch and whether the working tree had uncommitted changes.
The scope and the dimensions measured, and whether the scores hold only for the scope.>

## Summary

<One line per dimension measured: `<dimension>: <score>/5`, or `unscored`, and its reason.
Then the three recommendations that matter most, each with the solution recommended.>

## Metrics

<Graph: the modules and edges, every cycle, and the modules with the highest fan-in, fan-out and instability.
External reach: each external dependency and the modules that import it.
History: the commits sampled, the hotspots, the co-change pairs without an import and the spread.
Each metric with the command that produced it, or `approximate`.>

## Dimensions

<One subsection per dimension measured, in the order of the rubric:

### <Dimension>: <score>/5

<Why it meets that level and not the next, the metrics it rests on and each finding at `file:line`.>>

## Strengths

<What the architecture does well and must be kept, each with its evidence at `file:line`.>

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
