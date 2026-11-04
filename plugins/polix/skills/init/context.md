# <name of the project>

<!-- Fixed structure, general context only.
Never add, remove, rename or reorder the # and ## headings of this file, nor add ### subsections.
Every section holds only bullets, one fact per item, with exact values.
Every item is a short summary on a single line of at most 120 characters, never wrapped; the detail belongs in docs/.
It describes how the project is; the rules on how to write its code go to ai/rules/.
Under 100 lines.
Only the init skill or a human changes this file: any other skill or agent reports the change it needs instead. -->

## Purpose

<!-- One to three items: what the project is, what it does and who uses it. -->

## Stack

<!-- One item per area (languages and frameworks, infrastructure, tooling, CI), as `- **<Area>**: <technologies>`.
Only the names of the technologies the project uses; versions, files, configuration, scripts and wiring belong in docs/.
Example: `- **Infrastructure**: Terraform and Terragrunt, on AWS.` -->

## Commands

<!-- One item per command, as ``- **<Action>**: `<command>` ``, plus its prerequisites (profile, variable) if any.
Commands run from the repository root unless they change folder.
Install, build, run, test, single test, lint, format, and any other the flow needs (e.g. apply, deploy). -->

## Layout

<!-- Keep the fixed items that apply: ai/ items when the folder exists, polix.yaml when the file exists.
Add one item per other top-level folder, in alphabetical order, as `- **<folder>/**: <what it holds>`. -->

- **ai/**: instructions for the AI only; without it the repository is a plain project.
- **ai/plans/**: one plan per file, written by the `plan` skill.
- **ai/rules/**: rules on how to write the code, one `yaml` file per topic.
- **polix.yaml**: the workflow of the agents, enforced by polix through the hooks.

## Testing

<!-- One item per kind of test (unit, integration, e2e, policy...), as `- **<Kind>**: <framework>, in <folder>`.
Only the name of the framework and the folder; commands belong in `Commands`.
Fixtures, mocks, coverage, configuration and what each test checks belong in docs/.
Example: `- **Unit**: pytest, in tests/unit/.` -->

## Rules

<!-- Keep the item when ai/rules/ exists; otherwise write `Not applicable.` -->

- The rules for the AI are in `ai/rules/*.yaml`.

## Documentation

<!-- One or two items: where the documentation of the project lives and how it is published, if it is.
Example: `- The documentation of the project is in docs/, published as a website with Hugo and Docsy.` -->
