# pair-to-gemini — agent manual

You can use `pair-to-gemini` to get a second opinion from Gemini Desktop:
plans, specs, diffs, design comparisons, explanations. It is a chat channel.
It cannot read your repository, run commands, or edit files. Paste what Gemini
needs into the prompt, and treat the reply as a review comment, not a fact.

On the Mac, call `pair-to-gemini`. On any other host, call `p2g-remote` with
`P2G_SSH_HOST` set; the arguments, output, and exit codes are the same.

## 1. Preflight

```bash
pair-to-gemini doctor --json
```

Continue only if `"ok": true`. Otherwise tell the human which of these is
false: `axTrusted`, `geminiRunning`, or `screenLocked` (must be false). Do not
try to fix Accessibility or unlock the screen yourself.

## 2. Pick the target

| Situation | Command |
|---|---|
| One-off question | `send` (opens a new conversation) |
| Next round of the same review | `send --pair <id>` |
| The human is at the Mac and asked for the open chat | `send --current` |

To create a pair from a finished one-off task, bind the `geminiTitle` it
returned:

```bash
pair-to-gemini pair add --id <id> --gemini-title "<geminiTitle>"
```

Pick a short `<id>` that names the review (`auth-review`, `plan-v2`). Use one
pair per review, not one per round.

## 3. Write the prompt

Put it in a file and send it with `--stdin`. Include:

- the role ("You are reviewing a database migration plan")
- the material, pasted in full; trim unrelated files
- the questions, as a short list
- the output format ("Markdown, no preamble, cite section numbers")
- "Read-only: do not run commands or modify files."

Leave out secrets, tokens, customer data, and internal hostnames. Do not add
tool markers or automation instructions; the tool adds a `[task <id>]` line by
itself.

## 4. Send and read the result

```bash
pair-to-gemini send --timeout 300 --stdin < prompt.md
pair-to-gemini send --pair <id> --timeout 300 --stdin < follow-up.md
```

The command prints one JSON object on stdout (errors go to stderr as
`{"ok":false,"error":"..."}`). On success:

```json
{"ok": true, "completed": true, "taskId": "20260101-120000-ab12cd",
 "pairId": "", "geminiTitle": "Plan review", "status": "sent",
 "message": "...", "messageBytes": 1234, "messageSha256": "..."}
```

Use `message`. Save `taskId` and `geminiTitle` so you can resume or continue.
In Task mode the first line of `message` is often a short result title that
Gemini adds above the answer; ignore it when you need the bare answer.

## 5. Act on the exit code

| Exit | Meaning | Your next step |
|---|---|---|
| `0` | Reply received | Read `message` |
| `1` | Still generating (`wait_timeout`) | `task wait <taskId> --timeout 300`. **Do not resend.** |
| `2` | Not started (`error` names the cause) | See the table below |
| `3` | Gemini needs a human (`needs_attention`) | Tell the human the `taskId` and `confirmationCard`. After they handle it, `task wait <taskId>`. **Do not resend.** |
| `4` | Proxy failure: socket or daemon | Check the proxy (`p2g-remote pair list`); the task may or may not exist |
| `255` | SSH failed (`p2g-remote` only) | Check the connection; if a send may have started, check with the human before resending |

If the result has `"submitUnconfirmed": true`, the prompt may or may not have
reached Gemini. Do not resend. Run `task wait <taskId>`. If that times out
too, ask the human to look at the app.

Common exit-2 errors:

| `error` | Action |
|---|---|
| `ax_not_trusted`, `gemini_not_running`, `screen_locked` | Tell the human; stop |
| `concurrency_limit` | Two tasks are generating; wait a minute, then send again |
| `unknown_pair` | Run `pair list`; create the pair or fix the id |
| `dispatch_failed:conversation_not_found` | Title changed. Run `chats`, then `pair add` with the new title |
| `dispatch_failed:title_verification_failed` | Same as above |
| `dispatch_failed:conversation_busy_or_needs_attention` | That chat is still busy or waiting on the human; wait, or ask the human |
| `specify_exactly_one_nonempty_message_source` | Pass exactly one of `--message`, `--file`, `--stdin`, or a positional prompt |

Exit 2 means nothing was sent, so sending again after fixing the cause is
safe. For exit 1, exit 3, and `submitUnconfirmed`, the task exists; resume it
with `task wait`.

## 6. Parallel work

At most two tasks generate at once by default. To run several reviews, submit
each with `--wait=false`, then collect them:

```bash
id1=$(pair-to-gemini send --wait=false --stdin < a.md | jq -r .taskId)
id2=$(pair-to-gemini send --wait=false --stdin < b.md | jq -r .taskId)
pair-to-gemini task wait "$id1"
pair-to-gemini task wait "$id2"
```

A dispatcher that cannot get a slot waits up to 90 seconds, then returns
`concurrency_limit`.

## 7. Other commands

```bash
pair-to-gemini task read <taskId>     # reprint an archived reply
pair-to-gemini task status <taskId>   # raw task record
pair-to-gemini pair list
pair-to-gemini chats                  # titles in Gemini's sidebar
```

Replies are archived on the Mac, so a reply survives a dropped SSH session;
read it with `task read`.

## Rules

- Use only the CLI. Do not drive the Gemini UI any other way.
- Never approve or dismiss a confirmation card yourself.
- Never resend after exit 1, exit 3, or `submitUnconfirmed`.
- Verify Gemini's claims against the source before acting on them.
- Tell the human the `taskId` and the outcome of each task.

---

## Working on this repository

- Build `scripts/build.sh`; test `go test ./...`; format `gofmt`.
- macOS only (`cgo` + Accessibility). Do not add other platforms without a
  design.
- Any change to prompt framing or reply extraction needs a test
  (`parser_test.go`).
- `P2G_DEBUG_CHUNKS=1` dumps the accessibility chunks read while waiting.
- The proxy stays a Unix socket (`0600`, in a `0700` directory). Do not add
  network listeners.
- Signing is opt-in (`scripts/setup-signing.sh`); `build.sh` signs only when
  it is enabled.
- Do not commit, push, tag, or publish without explicit human approval.
