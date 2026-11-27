# pair-to-gemini

> **Coding agents:** read [AGENTS.md](AGENTS.md) instead. It is the operating
> manual: preflight, commands, exit codes, and what to do on each error.

[中文说明](README.zh-CN.md)

A small macOS command-line tool that sends a prompt to **Gemini Desktop**
(`Gemini.app`) and returns the reply as JSON. It works through the macOS
Accessibility API, in the background: it does not take focus, raise the
window, or touch the clipboard.

Use it when you want a second opinion from Gemini, such as a plan review, a
spec review, or a code review, from a script or from a coding agent on
another machine.

```
your script / agent ──► pair-to-gemini ──► Accessibility API ──► Gemini.app
remote host ──ssh──► p2g-remote ──► Unix socket ──► LaunchAgent ──┘
```

## Before you use it

This is **not** an official Google integration. Automating the app may fall
outside Google's terms of service, and an account can be rate-limited or
suspended. Decide for yourself whether to use it, and with which account. Do
not send confidential material or credentials through it. The software comes
with no warranty (MIT).

## Install

You need macOS, Gemini Desktop signed in with Task mode available, and Go 1.22+
with the Xcode Command Line Tools.

```bash
git clone <this repo> pair-to-gemini
cd pair-to-gemini
scripts/install.sh        # guided; or scripts/build.sh for the binary only
```

This installs `~/.local/bin/pair-to-gemini` and creates the data directory
`~/.local/share/pair-to-gemini`. Make sure `~/.local/bin` is on your `PATH`.

Then grant Accessibility:

1. Open **System Settings → Privacy & Security → Accessibility**.
2. Add `~/.local/bin/pair-to-gemini`. If you run it from a terminal and it
   still is not trusted, add the terminal app too.
3. Check with `pair-to-gemini doctor`. It should report `axTrusted=true`.

**Rebuilding resets the grant.** macOS ties the grant to the binary's
signature, and every unsigned build has a new one. If you rebuild often, run
`scripts/setup-signing.sh` once. It creates a local signing certificate in
your login keychain, and later builds keep the grant. You can undo it with
`scripts/setup-signing.sh --disable`.

Tested with Gemini Desktop 1.150.x on macOS 27.2.

## Use

Ask a question and print only the answer:

```bash
pair-to-gemini send --text "Summarize this plan in three bullets: ..."
```

Send a file and get JSON back:

```bash
pair-to-gemini send --stdin < review.md
```

Each `send` opens a **new** Gemini conversation. To continue one, bind its
title to a short name (a *pair*) and send to that name:

```bash
title=$(pair-to-gemini send --stdin < plan.md | jq -r .geminiTitle)
pair-to-gemini pair add --id review --gemini-title "$title"
pair-to-gemini send --pair review --message "Now focus on the risks."
```

To submit now and collect the reply later:

```bash
pair-to-gemini send --wait=false --message "..."    # prints the taskId
pair-to-gemini task wait <taskId>
```

Every reply is archived, and `task read <taskId>` prints it again.

### Commands

| Command | What it does |
|---|---|
| `doctor [--json]` | Checks Gemini.app, Accessibility, and the screen lock |
| `chats` | Lists the conversations in Gemini's sidebar |
| `send [--pair <id> \| --current] <prompt>` | Sends a prompt. The prompt comes from `--message`, `--file`, `--stdin`, or a positional argument |
| `task wait / read / status <taskId>` | Resumes, rereads, or inspects a task |
| `pair list / add / init / remove` | Manages named conversations |
| `serve`, `client` | The SSH proxy (see below) |

Useful `send` options: `--timeout <sec>` (default 300), `--text`,
`--wait=false`, `--framed` (stricter reply extraction using marker tags).

### Things to know

- **Gemini owns the titles.** It names a conversation after the first reply and
  may rename it later. If that happens, `--pair` fails with
  `conversation_not_found`; bind the new title with `pair add`. Keep titles
  unique.
- **The prompt gets one extra line.** The tool adds a neutral `[task <id>]`
  first line so it can find its own message in the conversation. Nothing else
  is added.
- **Two at a time.** Several tasks can generate in parallel, two by default
  (`P2G_MAX_CONCURRENCY`). Sending is sequential: each send waits briefly for
  Gemini to name the conversation.
- **Replies may start with a summary line.** In Task mode Gemini often puts a
  short result title above the answer ("Confirmed the calculation."). It is
  part of what Gemini shows, so it stays in `message`.
- **Keep the Mac unlocked** while tasks run. Gemini can stay hidden or
  minimized.
- **`--current`** sends to whichever conversation is open, without checking
  which one it is. Use it only when you are at the Mac yourself.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Done; the reply is in `message` |
| `1` | Still generating; run `task wait <taskId>` |
| `2` | Could not start: bad arguments or a precondition (see the `error` field) |
| `3` | Gemini is waiting for you: a confirmation card or a dialog. Handle it in the app, then `task wait` |
| `4` | The proxy could not run the command (socket or daemon problem) |

`doctor` always exits `0`; read its `ok` field instead.

## Use from another machine (SSH)

An SSH session cannot use the Accessibility API, so a LaunchAgent runs
`pair-to-gemini serve` inside your login session. SSH only talks to its Unix
socket. Nothing listens on the network.

On the Mac:

```bash
scripts/install-agent.sh
pair-to-gemini client -- doctor --json     # should report "ok": true
```

On the other machine, copy `scripts/p2g-remote` into your `PATH` and add an SSH
alias for the Mac:

```
# ~/.ssh/config
Host mac
  HostName <mac-host-or-ip>
  User <mac-user>
  ServerAliveInterval 30
  ServerAliveCountMax 120
```

```bash
export P2G_SSH_HOST=mac
p2g-remote doctor --json
p2g-remote send --pair review --stdin < prompt.md
```

`p2g-remote` passes stdin, stdout, stderr, and the exit code through. SSH
itself exits `255` when it cannot connect.

The socket lives at `~/.local/share/pair-to-gemini/proxy.sock` with mode
`0600`, and the proxy only runs this same binary. Anyone who can log in to the
Mac as you can use it, so limit SSH access accordingly.

## Troubleshooting

| You see | Do this |
|---|---|
| `ax_not_trusted` | Grant Accessibility to the binary (and your terminal) |
| `axTrusted: false` from the proxy | Re-add the binary, then `launchctl kickstart -k gui/$(id -u)/com.pairtogemini.proxy` |
| `gemini_not_running` | Open Gemini.app |
| `screen_locked` | Unlock the Mac |
| `concurrency_limit` | Two tasks are already generating; retry later |
| `dispatch_failed:conversation_not_found` | Run `chats`, then `pair add` with the exact title |
| `dispatch_failed:title_verification_failed` | The title changed; rebind with `pair add` |
| `dispatch_failed:conversation_busy_or_needs_attention` | Wait for the reply, or clear the card or dialog in Gemini |
| `"submitUnconfirmed": true` | The send could not be confirmed. Check Gemini before resending; `task wait` if the prompt is there |
| Reply includes the prompt or old text | Try `--framed`; set `P2G_DEBUG_CHUNKS=1` to see what was read |
| `bind: invalid argument` | Socket path too long (macOS limit is about 104 bytes); use a shorter `--socket` |
| `client: connect: no such file or directory` | The proxy is not running: `launchctl print gui/$UID/com.pairtogemini.proxy` |

## Uninstall

```bash
launchctl bootout gui/$(id -u)/com.pairtogemini.proxy
rm -f ~/Library/LaunchAgents/com.pairtogemini.proxy.plist ~/.local/bin/pair-to-gemini
rm -rf ~/.local/share/pair-to-gemini
```

Then remove the binary from the Accessibility list. If you enabled signing,
delete `pair-to-gemini local` in Keychain Access.

## Limitations

- macOS only.
- Text only: no attachments, audio, or images.
- Best-effort UI automation. A Gemini update can break it. Replies already
  received stay archived.

## Credits and license

A Go rewrite of [gemini-task-bridge](https://github.com/xsj0413abc-ui/gemini-task-bridge)
by [@xsj0413abc-ui](https://github.com/xsj0413abc-ui), which established the idea
of binding a local name to a Gemini Desktop Task conversation. See
[NOTICE](NOTICE). MIT licensed; see [LICENSE](LICENSE).
