# pair-to-gemini

> **编程 agent 请直接读 [AGENTS.md](AGENTS.md)。** 那里是操作手册：
> 预检、命令、退出码、每种错误该怎么处理。

[English](README.md)

一个 macOS 命令行小工具：把提示词发给 **Gemini 桌面版**（`Gemini.app`），
再把回复以 JSON 返回。它通过 macOS 辅助功能（Accessibility）API 在后台工作，
不抢焦点、不把窗口提到前台，也不碰剪贴板。

适合从脚本或另一台机器上的编程 agent 里，让 Gemini 给第二意见，比如
评审方案、规格或代码。

```
脚本 / agent ──► pair-to-gemini ──► Accessibility API ──► Gemini.app
远端主机 ──ssh──► p2g-remote ──► Unix socket ──► LaunchAgent ──┘
```

## 使用前须知

这**不是** Google 官方集成。自动化操作 App 可能不符合 Google 的服务条款，
账号可能被限流或封禁。是否使用、用哪个账号，请自行判断。不要通过它发送
机密材料或凭据。软件按 MIT 许可提供，不附任何担保。

## 安装

需要：macOS；已登录、可使用任务（Task）模式的 Gemini 桌面版；Go 1.22+ 和
Xcode Command Line Tools。

```bash
git clone <本仓库> pair-to-gemini
cd pair-to-gemini
scripts/build.sh          # 编译并安装到 ~/.local/bin/pair-to-gemini
```

安装后得到 `~/.local/bin/pair-to-gemini`，并创建数据目录
`~/.local/share/pair-to-gemini`。确认 `~/.local/bin` 在 `PATH` 里。

然后授予辅助功能权限：

1. 打开 **系统设置 → 隐私与安全性 → 辅助功能**。
2. 添加 `~/.local/bin/pair-to-gemini`。如果在终端里运行仍未获得授权，把终端
   App 也加进去。
3. 运行 `pair-to-gemini doctor` 检查，应显示 `axTrusted=true`。

**重新编译会让授权失效。** macOS 按二进制签名记录授权，而每次未签名编译出的
签名都不同。如果经常重新编译，运行一次 `scripts/build.sh --enable-signing`。它会
在登录钥匙串里创建一个本地签名证书，之后的编译都能保留授权。撤销方法是运行
`scripts/build.sh --disable-signing`。

已在 macOS 27.2 + Gemini 桌面版 1.150.x 上测试。

**给 coding agent：** 把 `skills/pair-to-gemini/` 复制到你的 agent skills
目录，或直接读 `AGENTS.md`。那份 skill 是这份手册的简版。

## 使用

提一个问题，只输出答案：

```bash
pair-to-gemini send --text "用三点总结这个方案：..."
```

发送一个文件，返回 JSON：

```bash
pair-to-gemini send --stdin < review.md
```

每次 `send` 都会新开一个 Gemini 会话。要接着某个会话继续，先把它的标题绑定到
一个短名字（*pair*），再发到这个名字：

```bash
title=$(pair-to-gemini send --stdin < plan.md | jq -r .geminiTitle)
pair-to-gemini pair add --id review --gemini-title "$title"
pair-to-gemini send --pair review --message "现在重点看风险。"
```

先提交、稍后再取回复：

```bash
pair-to-gemini send --wait=false --message "..."    # 输出 taskId
pair-to-gemini task wait <taskId>
```

所有回复都会存档，`task read <taskId>` 可以再次打印。

### 命令

| 命令 | 作用 |
|---|---|
| `doctor [--json]` | 检查 Gemini.app、辅助功能权限和锁屏状态 |
| `chats` | 列出 Gemini 侧边栏里的会话 |
| `send [--pair <id> \| --current] <提示词>` | 发送提示词，来源可以是 `--message`、`--file`、`--stdin` 或位置参数 |
| `task wait / read / status <taskId>` | 继续等待、重读或查看任务 |
| `pair list / add / init / remove` | 管理命名会话 |
| `serve`、`client` | SSH 代理（见下文） |

`send` 的常用选项：`--timeout <秒>`（默认 300）、`--text`、`--wait=false`、
`--framed`（用标记标签做更严格的回复提取）。

### 需要知道的几点

- **标题由 Gemini 决定。** 它在第一次回复后给会话命名，之后也可能改名。改名后
  `--pair` 会报 `conversation_not_found`，用 `pair add` 重新绑定即可。标题要保持唯一。
- **提示词会多出一行。** 工具会在开头加一行中性的 `[task <id>]`，用来在会话里找到
  自己发的消息。除此之外不加任何内容。
- **默认同时两个。** 多个任务可以并行生成，默认上限 2（`P2G_MAX_CONCURRENCY`）。
  发送是逐个进行的：每次发送都要稍等 Gemini 给会话命名。
- **回复开头可能有一行摘要。** 任务模式下 Gemini 常在答案上方加一行简短的结果标题
  （如"完成了基础加法运算任务。"）。这是 Gemini 显示的内容，所以会保留在 `message` 里。
- **任务运行期间保持 Mac 解锁。** Gemini 可以隐藏或最小化。
- **`--current`** 发到当前打开的会话，不检查是哪一个。只在你本人守在 Mac 前时使用。

## 退出码

| 码 | 含义 |
|---|---|
| `0` | 完成，回复在 `message` 里 |
| `1` | 仍在生成，运行 `task wait <taskId>` |
| `2` | 未能开始：参数错误或前置条件不满足（看 `error` 字段） |
| `3` | Gemini 在等人处理：确认卡片或对话框。在 App 里处理后再 `task wait` |
| `4` | 代理无法执行命令（socket 或守护进程问题） |

`doctor` 总是返回 `0`，要看它的 `ok` 字段。

## 从另一台机器使用（SSH）

SSH 会话不能使用辅助功能 API，所以由一个 LaunchAgent 在你的登录会话里运行
`pair-to-gemini serve`。SSH 只和它的 Unix socket 通信，不监听任何网络端口。

在 Mac 上：

```bash
pair-to-gemini agent install
pair-to-gemini client -- doctor --json     # 应返回 "ok": true
pair-to-gemini agent status                # plist、加载状态、socket
```

在另一台机器上，把 `scripts/p2g-remote` 复制到 `PATH`，并给 Mac 配一个 SSH 别名：

```
# ~/.ssh/config
Host mac
  HostName <mac 主机名或 IP>
  User <mac 用户名>
  ServerAliveInterval 30
  ServerAliveCountMax 120
```

```bash
export P2G_SSH_HOST=mac
p2g-remote doctor --json
p2g-remote send --pair review --stdin < prompt.md
```

`p2g-remote` 会原样转发 stdin、stdout、stderr 和退出码。SSH 本身连不上时返回 `255`。

socket 位于 `~/.local/share/pair-to-gemini/proxy.sock`，权限 `0600`，代理只会运行
同一个二进制。凡是能以你的身份登录这台 Mac 的人都能使用它，请相应地限制 SSH 访问。

## 故障排查

| 现象 | 处理 |
|---|---|
| `ax_not_trusted` | 给二进制（以及你用的终端）授予辅助功能权限 |
| 代理报 `axTrusted: false` | 在辅助功能里重新添加二进制，再运行 `pair-to-gemini agent install` 重启它 |
| `gemini_not_running` | 打开 Gemini.app |
| `screen_locked` | 解锁 Mac |
| `concurrency_limit` | 已有两个任务在生成，稍后重试 |
| `dispatch_failed:conversation_not_found` | 运行 `chats`，再用准确的标题 `pair add` |
| `dispatch_failed:title_verification_failed` | 标题变了，用 `pair add` 重新绑定 |
| `dispatch_failed:conversation_busy_or_needs_attention` | 等回复结束，或在 Gemini 里处理卡片 / 对话框 |
| `"submitUnconfirmed": true` | 无法确认是否发出。重发前先看 Gemini；如果消息已在会话里，就 `task wait` |
| 回复里带了提示词或旧内容 | 试试 `--framed`；设 `P2G_DEBUG_CHUNKS=1` 查看实际读到的内容 |
| `bind: invalid argument` | socket 路径太长（macOS 上限约 104 字节），用更短的 `--socket` |
| `client: connect: no such file or directory` | 代理没在运行：`pair-to-gemini agent status` |

## 卸载

```bash
pair-to-gemini agent uninstall
rm -f ~/.local/bin/pair-to-gemini
rm -rf ~/.local/share/pair-to-gemini
```

然后从辅助功能列表里移除这个二进制。如果启用过签名，在钥匙串访问里删除
`pair-to-gemini local`。

## 限制

- 只支持 macOS。
- 只处理文本，不支持附件、音频和图片。
- UI 自动化是尽力而为，Gemini 更新可能导致失效。已收到的回复仍保留在存档里。

## 致谢与许可

本项目是 [@xsj0413abc-ui](https://github.com/xsj0413abc-ui) 的
[gemini-task-bridge](https://github.com/xsj0413abc-ui/gemini-task-bridge) 的 Go 重写版。
「把本地名字绑定到 Gemini 桌面版任务会话」这一思路来自原项目。详见 [NOTICE](NOTICE)。
MIT 许可，见 [LICENSE](LICENSE)。
