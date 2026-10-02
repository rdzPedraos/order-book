---
name: commit
description: Create a git commit with the project's message format (type, title, Why, What), written only from the staged diff. Use whenever the user asks to commit, make a commit, or save changes to git, or says "/commit".
---

Commit only what the user has already staged, and describe only what the staged diff shows.

**Never alter the repository state** (index, working tree, branches, history). The user decides what goes in the commit. The only git commands allowed are read-only ones (`git diff --cached`, `git status`, `git log`, `git show`) and a plain `git commit` with the message.

## Source of truth: the staged diff, not the conversation

The message describes the staged changes, nothing else. The conversation history, plans, OpenSpec changes, and what "we decided" or "will implement" are **not** sources for the message:

- Do not describe work that is not in the diff: discussed changes, future phases, files that will exist later.
- Do not describe a document as if it were code. If the diff edits a doc, spec or standard, the commit documents or specifies something; it does not add, replace or implement it.
- If the chat and the diff disagree, the diff wins.
- Do not borrow the `Why` from the chat. Take it from what the diff itself shows (what the removed lines said and the added lines say instead). If the reason is not visible in the diff, ask the user for a one-line reason before committing.

## Steps

1. Run `git diff --cached --stat` and `git diff --cached`. Ignore unstaged and untracked files.
2. If nothing is staged, stop and tell the user to stage their changes.
3. Classify each staged file by kind: code, tests, config/infra, docs/specs/standards. Note what each hunk does (added, removed, renamed, reworded).
4. If the staged changes mix unrelated purposes, point it out and ask whether to continue or let them restage.
5. Write the message in English with the format below.
6. **Self-check before committing.** For every `What` bullet, name the file and hunk it comes from. Delete any bullet you cannot point to. Check that the type and verbs match the kind of files (step 3). Check that nothing in the message comes only from the conversation.
7. Run `git commit` with a heredoc.
8. Run `git log -1 --format='%h%n%n%B'` and paste its output verbatim in a `text` code block in your final reply, so the user sees the exact message that was committed. Tool output is not shown to the user, so it must go in the reply.
9. Do not push unless the user asks. Never amend unless the user asks. If they ask to fix only the message, use `git commit --amend --only` (no paths), so that changes staged after the commit do not get absorbed.

## Format

```text
<type>: <title>

Why: <the problem or need that motivates the change>

What:
- <concrete change and its effect>
- <concrete change and its effect>
```

- `type` is one of `feat`, `fix`, `chore`, `refactor`:
  - `feat` / `fix`: code that adds or fixes behavior.
  - `refactor`: code that changes structure without changing behavior.
  - `chore`: docs, specs, standards, config and tooling.
- `title`: imperative, lowercase, no trailing period, at most 72 characters (`add order cancellation endpoint`).
- `Why`: the problem, not the solution. One or two sentences, grounded in the diff.
- `What`: one bullet per change actually present in the diff, with its effect. Keep each bullet short. For docs, use verbs like "Document", "Describe", "State", "Specify".

## Examples

Code change:

```text
feat: add order cancellation endpoint

Why: users could not cancel a pending order, so mistaken orders stayed on the book.

What:
- Add DELETE /orders/{id}, which moves the order to CANCELLED
- Return 409 when the order is already in a final state
- Document the endpoint in docs/api.md
```

Docs-only change (the diff edits only `.claude/standards/architecture.md`):

```text
chore: document single root go module in architecture standard

Why: the architecture standard described a go.work workspace with one Go module per service.

What:
- Replace go.work and the per-service modules in the monorepo tree with one root go.mod/go.sum
- State that services import shared packages by full path from the module root
- State that isolation between services relies on Go's internal/ rule
```

Wrong for that same docs-only diff, because it describes code that is not in the commit: "Replace go.work with a single go.mod", "Build each service with go build ./services/<x>".
