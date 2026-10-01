---
name: commit
description: Create a git commit with the project's message format (type, title, Why, What). Use whenever the user asks to commit, make a commit, or save changes to git, or says "/commit".
---

Commit only what the user has already staged.

**Never alter the repository state** (index, working tree, branches, history). The user decides what goes in the commit. The only git commands allowed are read-only ones (`git diff --cached`, `git status`, `git log`) and a plain `git commit` with the message.

1. Run `git diff --cached` to read the staged changes. Ignore unstaged and untracked files.
2. If nothing is staged, stop and tell the user to stage their changes.
3. If the staged changes mix unrelated purposes, point it out and ask whether to continue or let them restage.
4. Write the message in English with this exact format, based only on the staged diff, and run `git commit` with a heredoc:

```text
<type>: <title>

Why: <the problem or need that motivates the change>

What:
- <concrete change and its effect>
- <concrete change and its effect>
```

- `type` is one of `feat`, `fix`, `chore`, `refactor`.
- `title`: imperative, lowercase, no trailing period, at most 72 characters (`add order cancellation endpoint`).
- `Why` explains the problem, not the solution. One or two sentences.
- `What` lists what actually changed and how it affects the system, one bullet per change. Keep each bullet short.

5. Do not push unless the user asks.

Example:

```text
feat: add order cancellation endpoint

Why: users could not cancel a pending order, so mistaken orders stayed on the book.

What:
- Add DELETE /orders/{id}, which moves the order to CANCELLED
- Return 409 when the order is already in a final state
- Document the endpoint in docs/api.md
```
