# Question template

Ask every question to the user as text, one per message, with this template, and wait for the answer.
Write every word in the language of the session, the labels too; only the names of the code, commands and
literals keep their spelling.
Never use `AskUserQuestion` nor any dialog of the agent: the template works the same in every agent.

```markdown
`[Q<n> OF <total>]` `[WHAT|HOW]` **<TITLE OF THE DECISION>**
> 
> <What you are deciding and why it matters, in one or two sentences that read without the rest of the chat.>
> <Optional: the fact it rests on, with its `file:line`, in one line.>

- `[A]` **<recommended option>** · (_Recommended_)

> <What the option is and how the change behaves with it: what the user sees, what the code does, in two or three
> sentences.>
> 
> - **Pro:** <what it gains>
> - **Con:** <what it costs>
> - **Cost:** effort <low|medium|high> · risk <low|medium|high> · tests <the tests it needs, or none>

- `[B]` **<option>**

> <What the option is and how the change behaves with it, in two or three sentences.>
> 
> - **Pro:** <what it gains>
> - **Con:** <what it costs>
> - **Cost:** effort <low|medium|high> · risk <low|medium|high> · tests <the tests it needs, or none>

`→` **A**, <the reason of the recommendation in a few words>. Reply `yes`, a letter or your own answer.
```

## Title

- The title line has the number in brackets and inline code, then the kind, then the title in bold capitals.
- Right under it, with no blank line, a quote (`>`) opens with a line `>` and holds what you decide and the fact.
- Number the questions in order; `<total>` is the estimate of the frontier, prefixed with `~` while it may grow.
- The kind is `[WHAT]` for a decision of the what and `[HOW]` for one of the how (see `What and how` in SKILL.md).
- A confirmation, such as the brief, the next step or the approval of a plan, has no number nor kind: its title
  line is `` `[CONFIRM]` **<TITLE>** ``, and its text says what is just above it.

## Options

- Two to four options, lettered from A; the recommended one goes first, marked `· (_Recommended_)`.
- Every option title is an item of a list (`- `) with its letter in brackets and inline code (`[A]`), and the
  option in bold; never a header.
- Under its title, a blank line and a quote (`>`): the explanation, a line `>`, and the `Pro`, `Con` and `Cost`.
  Never indent with spaces, which makes a code block.
- The explanation says what the option is and how the change behaves with it, concrete enough to choose without
  reading the code.
- One `Pro` and one `Con` line; join two ideas with `;` rather than add a line.
  A con may be "none" only on a confirmation, and a confirmation drops `Cost`.
- `Cost` rates effort and risk against the other options, and names the tests the option needs.

## Answer

- The line `→`, plain text after the last option, names the recommended letter, the reason in a few words and how
  to reply.
- The answer `yes` takes the recommended option, a letter takes that option, and any other text is a valid answer:
  record it as given and ask about it only when ambiguous.
- Keep it short: every line holds one idea, and the whole question fits in one screen.
- An unanswered question is asked again; never fill it with a default.
