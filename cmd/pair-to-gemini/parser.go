package main

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

const (
	SentinelStartPrefix = "[[TASK_START:"
	SentinelDonePrefix  = "[[TASK_DONE:"
	SentinelSuffix      = "]]"
)

var (
	newChatTitles = map[string]bool{
		"新对话": true, "New chat": true, "New Chat": true, "": true,
	}
	sidebarStaticLabels = map[string]bool{
		"库": true, "图片": true, "已排定": true, "每日简报": true, "学生": true, "更多": true,
		"笔记本": true, "新建笔记本": true, "项目": true, "排程": true, "技能": true,
		"Library": true, "Images": true, "Scheduled": true, "Daily brief": true,
		"Students": true, "More": true, "Notebooks": true, "New notebook": true,
		"Projects": true, "Schedules": true, "Skills": true,
	}
	chromeTexts = map[string]bool{
		"Gemini 是 AI，可能会出错。":                  true,
		"Gemini is AI and can make mistakes.": true,
		"问问 Gemini":                           true, "分配任务": true, "描述任务": true,
		"Ask Gemini": true, "Assign a task": true, "Describe a task": true, "Message Gemini": true,
		"附加": true, "共享屏幕": true, "使用麦克风": true, "Speak to Window": true,
		"对话": true, "任务": true, "Chat": true, "Task": true,
		"Folders": true, "文件夹": true, "拷贝": true, "Copy": true, "Copy response": true,
		"提交": true, "Submit": true, "Send": true,
		"停止回答": true, "停止": true, "Stop": true, "Stop response": true,
		"What’s next?": true, "What's next?": true, "接下来做什么？": true,
		"Tag me in! What’s next?": true, "Tag me in! What's next?": true,
		"Gemini":      true,
		"Gemini said": true, "Gemini 说": true,
		"Show thinking": true, "显示思路": true, "Hide thinking": true, "隐藏思路": true,
		"Thinking it through": true, "Complete": true,
		"Rename this chat": true, "重命名此对话": true,
		"Export to Docs": true, "Send feedback": true, "Good response": true, "Bad response": true,
		"Reply": true, "Try again": true, "Completed": true, "已完成": true, "In progress": true, "进行中": true,
	}
	confirmationCardIDs = []string{
		"ActionCardRemyConfirmationCard",
		"ActionCardRemyElicitationCard",
		"AgentWaitingForInputRow",
		"InlineConfirmationSlotView",
		"tab_stop_Continue",
	}
	badgeMap = map[string]string{
		"进行中":             "in_progress",
		"In progress":     "in_progress",
		"需要注意":            "needs_attention",
		"Needs attention": "needs_attention",
		"准备就绪":            "ready",
		"Ready":           "ready",
	}
	quotedBadgeZhRe = regexp.MustCompile(`^“(.+)”，(需要注意|进行中|准备就绪|Needs attention|In progress|Ready)$`)
	quotedBadgeEnRe = regexp.MustCompile(`^"(.+)",\s*(Needs attention|In progress|Ready|需要注意|进行中|准备就绪)$`)
	plainBadgeRe    = regexp.MustCompile(`^(.+?)[，,]\s*(Needs attention|In progress|Ready|需要注意|进行中|准备就绪)$`)
	otherDoneRe     = regexp.MustCompile(`\[\[TASK_DONE:[^\]]+\]\]`)
	pairIDRe        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	taskIDRe        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
)

type SidebarItem struct {
	Raw    string `json:"raw"`
	Title  string `json:"title"`
	Pinned bool   `json:"pinned"`
	Badge  string `json:"badge,omitempty"`
	Source string `json:"source,omitempty"`
}

func NormalizeWindowTitle(raw string) string {
	t := strings.TrimSpace(raw)
	if t == "Gemini" {
		return ""
	}
	for _, p := range []string{"Gemini - ", "Gemini — ", "Gemini – ", "Gemini: "} {
		if strings.HasPrefix(t, p) {
			return strings.TrimSpace(strings.TrimPrefix(t, p))
		}
	}
	return t
}

func ParseSidebarRowLabel(raw string) SidebarItem {
	text := strings.TrimSpace(raw)
	pinned := false
	for _, suf := range []string{"，已固定", ", Pinned"} {
		if strings.HasSuffix(text, suf) {
			pinned = true
			text = strings.TrimSpace(strings.TrimSuffix(text, suf))
			break
		}
	}

	title := text
	badge := ""
	if m := quotedBadgeZhRe.FindStringSubmatch(text); m != nil {
		title = strings.TrimSpace(m[1])
		badge = badgeMap[m[2]]
	} else if m := quotedBadgeEnRe.FindStringSubmatch(text); m != nil {
		title = strings.TrimSpace(m[1])
		badge = badgeMap[m[2]]
	} else if m := plainBadgeRe.FindStringSubmatch(text); m != nil {
		title = strings.TrimSpace(m[1])
		badge = badgeMap[m[2]]
	}

	if !pinned {
		for _, suf := range []string{"，已固定", ", Pinned"} {
			if strings.HasSuffix(title, suf) {
				pinned = true
				title = strings.TrimSpace(strings.TrimSuffix(title, suf))
				break
			}
		}
	}
	for _, suf := range []string{", On this Mac", "，在此 Mac 上"} {
		if strings.HasSuffix(title, suf) {
			title = strings.TrimSpace(strings.TrimSuffix(title, suf))
			break
		}
	}
	return SidebarItem{
		Raw:    raw,
		Title:  title,
		Pinned: pinned,
		Badge:  badge,
	}
}

// BuildDispatchPrompt prefixes a neutral task reference. It is the only
// addition to the user's text in the default mode; nothing about the tool is
// mentioned.
func BuildDispatchPrompt(taskID, message string) string {
	return "[task " + taskID + "]\n\n" + strings.TrimSpace(message)
}

func BuildFramedPrompt(taskID, message string) string {
	startTag := SentinelStartPrefix + taskID + SentinelSuffix
	return startTag + "\n" +
		"[task " + taskID + "]\n" +
		"When finished, end your response with a standalone line: " +
		SentinelDonePrefix + taskID + SentinelSuffix + "\n" +
		"--- TASK ---\n" +
		strings.TrimRight(message, " \t\r\n") + "\n"
}

func ExtractGeminiReply(chunks []string, baseline []string, taskID string) (hasSentinel bool, cleaned string) {
	startMarker := SentinelStartPrefix + taskID + SentinelSuffix
	doneMarker := SentinelDonePrefix + taskID + SentinelSuffix

	var nonEmpty []string
	for _, c := range chunks {
		if strings.TrimSpace(c) != "" {
			nonEmpty = append(nonEmpty, c)
		}
	}
	var base []string
	for _, c := range baseline {
		if strings.TrimSpace(c) != "" {
			base = append(base, c)
		}
	}

	lastStartIdx := -1
	for i, ch := range nonEmpty {
		if strings.Contains(ch, startMarker) {
			lastStartIdx = i
		}
	}
	cutIdx := 0
	if lastStartIdx != -1 {
		cutIdx = lastStartIdx + 1
	}

	for i, ch := range nonEmpty {
		for _, m := range otherDoneRe.FindAllString(ch, -1) {
			if m != doneMarker && i+1 > cutIdx {
				cutIdx = i + 1
			}
		}
	}

	if lastStartIdx == -1 && len(base) > 0 {
		common := 0
		limit := len(base)
		if len(nonEmpty) < limit {
			limit = len(nonEmpty)
		}
		for common < limit && nonEmpty[common] == base[common] {
			common++
		}
		if common > cutIdx {
			cutIdx = common
		}
	}

	if cutIdx > len(nonEmpty) {
		cutIdx = len(nonEmpty)
	}
	rawReply := strings.Join(nonEmpty[cutIdx:], "\n")
	hasSentinel = strings.Contains(rawReply, doneMarker)

	var lines []string
	skipEcho := false
	for _, line := range strings.Split(rawReply, "\n") {
		s := strings.TrimSpace(line)
		if strings.Contains(s, startMarker) {
			skipEcho = true
			continue
		}
		if skipEcho {
			if strings.Contains(s, "--- TASK ---") {
				skipEcho = false
			}
			continue
		}
		if s == doneMarker {
			continue
		}
		if strings.Contains(s, doneMarker) {
			s = strings.TrimSpace(strings.ReplaceAll(s, doneMarker, ""))
			if s == "" {
				continue
			}
			line = s
		}
		if chromeTexts[s] {
			continue
		}
		lines = append(lines, line)
	}
	cleaned = strings.TrimSpace(strings.Join(lines, "\n"))
	return hasSentinel, cleaned
}

func ExtractPlainReply(chunks, baseline []string, prompt string) string {
	var nonEmpty []string
	for _, c := range chunks {
		if strings.TrimSpace(c) != "" {
			nonEmpty = append(nonEmpty, c)
		}
	}
	var base []string
	for _, c := range baseline {
		if strings.TrimSpace(c) != "" {
			base = append(base, c)
		}
	}

	// The echoed "[task <id>]" line is the most reliable anchor: everything
	// after it belongs to this task. Without it, skip past the baseline.
	fresh := nonEmpty[baselineEnd(nonEmpty, base):]
	tag := taskTag(prompt)
	if tag != "" {
		for i, ch := range nonEmpty {
			if strings.Contains(ch, tag) {
				fresh = nonEmpty[i+1:]
				break
			}
		}
	}

	// Drop the echoed user message. It is rendered as its own chunk (or a few)
	// right after the baseline; only look near the start so a long quotation in
	// the answer cannot be mistaken for the echo.
	start := 0
	normPrompt := normalizePromptText(prompt)
	// The tag line can be rendered as its own chunk, leaving only the body.
	normBody := ""
	if tag != "" {
		normBody = normalizePromptText(strings.TrimPrefix(strings.TrimSpace(prompt), tag))
	}
	if normPrompt != "" {
		scan := len(fresh)
		if scan > 5 {
			scan = 5
		}
		for i := 0; i < scan; i++ {
			if isPromptEcho(fresh[i], normPrompt) || (normBody != "" && isPromptEcho(fresh[i], normBody)) {
				start = i + 1
			}
		}
	}

	var lines []string
	for _, ch := range fresh[start:] {
		s := strings.TrimSpace(ch)
		if s == "" || chromeTexts[s] {
			continue
		}
		lines = append(lines, ch)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// taskTag returns the "[task <id>]" first line that BuildDispatchPrompt adds.
func taskTag(prompt string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(prompt), "\n")
	first = strings.TrimSpace(first)
	if strings.HasPrefix(first, "[task ") && strings.HasSuffix(first, "]") {
		return first
	}
	return ""
}

// baselineEnd returns the index just past the baseline inside chunks. The
// baseline is the tail of the conversation before dispatch, so it is matched
// as a contiguous run rather than as a prefix.
func baselineEnd(chunks, base []string) int {
	if len(base) == 0 {
		return 0
	}
	for k := 0; k+len(base) <= len(chunks); k++ {
		match := true
		for j := range base {
			if chunks[k+j] != base[j] {
				match = false
				break
			}
		}
		if match {
			return k + len(base)
		}
	}
	// The view changed too much to align; fall back to the common prefix.
	cut := 0
	for cut < len(base) && cut < len(chunks) && chunks[cut] == base[cut] {
		cut++
	}
	return cut
}

func chunksContain(chunks []string, s string) bool {
	for _, c := range chunks {
		if strings.Contains(c, s) {
			return true
		}
	}
	return false
}

func normalizePromptText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func isPromptEcho(chunk, normPrompt string) bool {
	c := normalizePromptText(chunk)
	if c == "" {
		return false
	}
	if c == normPrompt {
		return true
	}
	if len(c) >= 40 && strings.Contains(normPrompt, c) {
		return true
	}
	return len(normPrompt) >= 40 && strings.Contains(c, normPrompt)
}

func Sha256Text(s string) (string, int) {
	b := []byte(s)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), len(b)
}
