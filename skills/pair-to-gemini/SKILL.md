---
name: pair-to-gemini
description: Send a read-only plan, spec, or code review prompt to Google Gemini Desktop (Gemini.app) on a Mac and read the reply. Use when a second opinion from Gemini is wanted; from Linux use the p2g-remote SSH wrapper.
---

# pair-to-gemini

Sends a prompt to Gemini Desktop and returns the reply as JSON. Use it for a
second opinion on plans, specs, and diffs. Gemini cannot see the repository,
so paste the material into the prompt. Treat the reply as a review comment.

`AGENTS.md` in this repository is the full manual and wins on any conflict.

- On the Mac: `pair-to-gemini` (installed to `~/.local/bin`).
- Elsewhere: `P2G_SSH_HOST=mac p2g-remote ...`, same arguments and exit codes.
  Never run the Darwin binary on Linux.

## Steps

1. **Preflight.** `pair-to-gemini doctor --json`. Continue only if `"ok": true`;
   otherwise report which of `axTrusted`, `geminiRunning`, and `screenLocked`
   is wrong.
2. **Target.** A plain `send` opens a new conversation. For the next round of
   the same review, bind it once with
   `pair add --id <id> --gemini-title "<geminiTitle from a send>"`, then use
   `send --pair <id>`. Use `--current` only when the human asks for it.
3. **Prompt.** Write it to a file: role, the material in full, the questions,
   the output format, and "read-only: do not run commands or modify files".
   No secrets. The tool adds a `[task <id>]` first line by itself.
4. **Send.** `pair-to-gemini send --timeout 300 --stdin < prompt.md`. Read
   `message`; keep `taskId` and `geminiTitle`.

## Exit codes

| Exit | Next step |
|---|---|
| `0` | Read `message` |
| `1` | Still generating: `task wait <taskId> --timeout 300`. Do not resend |
| `2` | Nothing was sent. Fix `error`; on `concurrency_limit`, retry later |
| `3` | A confirmation card or dialog needs the human. Report `taskId`, then `task wait`. Do not resend |
| `4` | Proxy or socket failure. Check `p2g-remote pair list` |
| `255` | SSH failed (`p2g-remote` only) |

`"submitUnconfirmed": true` means the send may or may not have arrived:
`task wait`, never resend.

## Parallel reviews

Two tasks generate at once by default (`P2G_MAX_CONCURRENCY`). Submit each with
`send --wait=false`, then `task wait <taskId>` for each one. `task read
<taskId>` reprints an archived reply, even after a dropped SSH session.

## Rules

- Use only the CLI; never approve confirmation cards yourself.
- Verify Gemini's claims before acting on them.
- Tell the human the `taskId` and the outcome.
