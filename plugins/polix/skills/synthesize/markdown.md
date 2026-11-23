# Markdown

The practices for the text the skill writes, in CommonMark and GitHub Flavored Markdown.

## Headings

- One `#` title per file, the first line after the frontmatter.
- Go down one level at a time: never `#` followed by `###`.
- Write ATX headings: the marks, one space and the text, with no closing marks and no final punctuation.
- Give each heading a text unique in the file, since links point to it by its anchor.
- Name the content in the heading in a few words: never a sentence nor a question.

## Blocks

- Leave one blank line before and after a heading, list, code block, table or quote.
- Never leave two blank lines in a row, and end the file with one newline.
- Start a new paragraph for a break: never two trailing spaces, a backslash nor `<br>`.
- Never leave trailing spaces nor hard tabs.
- Use raw HTML only for what Markdown cannot express.

## Lists

- Use one bullet marker in the whole file, `-` by default.
- Number a list only when the order of its items matters.
- Indent a nested list under the text of its parent item, the same width in the whole file.
- Write the items of one list in the same form: all sentences or all fragments, with the same punctuation.
- Start an item that defines something with its name in bold, then a colon: `- **Name**: what it is.`

## Code

- Fence code blocks with three backticks and name their language: `sh`, `go`, `yaml`, `json`, `text`.
- Never indent a block to make it code.
- Put in inline code every path, command, flag, identifier, field name, key and literal value.
- Never put prose in inline code nor in a code block.

## Emphasis

- Bold only a term, a name or a field: never a whole sentence nor a paragraph.
- Use italics sparingly, for a word used as a word or a title.
- Write `**` for bold and `_` for italics.

## Links and images

- Give every link a text that names its target: never "here", "this link" nor a bare URL.
- Link a file of the same repository by its relative path.
- Give every image an alt text that says what it shows.

## Tables

- Use a table only to compare items across several attributes; use a list otherwise.
- Give every table a header row, and keep each cell to a few words.
- Escape a `|` inside a cell as `\|`.

## Escaping

- Escape the characters Markdown would parse in prose: `*`, `_`, `#` at the start of a line, `<` and `[`.
