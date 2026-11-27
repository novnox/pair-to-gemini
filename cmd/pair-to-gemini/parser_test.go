package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseSidebarRowLabel(t *testing.T) {
	cases := []struct {
		raw    string
		title  string
		pinned bool
		badge  string
	}{
		{`"Pair to Gemini", Needs attention, Pinned`, "Pair to Gemini", true, "needs_attention"},
		{`“架构方案评审”，需要注意，已固定`, "架构方案评审", true, "needs_attention"},
		{`Codex 会话质量优化评估, Ready`, "Codex 会话质量优化评估", false, "ready"},
		{`My Task, In progress`, "My Task", false, "in_progress"},
		{`Repo Walkthrough, On this Mac`, "Repo Walkthrough", false, ""},
	}
	for _, tc := range cases {
		got := ParseSidebarRowLabel(tc.raw)
		if got.Title != tc.title || got.Pinned != tc.pinned || got.Badge != tc.badge {
			t.Fatalf("ParseSidebarRowLabel(%q) = %+v, want title=%q pinned=%v badge=%q",
				tc.raw, got, tc.title, tc.pinned, tc.badge)
		}
	}
}

func TestNormalizeWindowTitle(t *testing.T) {
	if got := NormalizeWindowTitle("Gemini — Pair to Gemini"); got != "Pair to Gemini" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeWindowTitle("Gemini"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestExtractGeminiReplyPromptEchoAndPriorTurnImmunity(t *testing.T) {
	tidOld := "20261127-100000-aaaaaa"
	tidNew := "20261127-100500-bbbbbb"
	prompt := BuildFramedPrompt(tidNew, "What is 6 * 7?")

	// Prompt echo alone must NOT count as done or reply
	chunksEchoOnly := []string{
		"Old answer\n" + SentinelDonePrefix + tidOld + SentinelSuffix,
		prompt,
	}
	hasSent, rep := ExtractGeminiReply(chunksEchoOnly, nil, tidNew)
	if hasSent || rep != "" {
		t.Fatalf("expected empty on echo only, got hasSent=%v rep=%q", hasSent, rep)
	}

	// Real answer with chrome noise and sentinel
	chunksDone := append(chunksEchoOnly,
		"Gemini said",
		"Show thinking",
		"6 * 7 equals 42.\n"+SentinelDonePrefix+tidNew+SentinelSuffix,
		"Copy response",
	)
	hasSent2, rep2 := ExtractGeminiReply(chunksDone, nil, tidNew)
	if !hasSent2 || rep2 != "6 * 7 equals 42." {
		t.Fatalf("unexpected extraction: hasSent=%v rep=%q", hasSent2, rep2)
	}
}

func TestExtractPlainReply(t *testing.T) {
	prompt := "用一句话回答：1+1 等于几？"
	chunks := []string{
		"旧问题",
		"旧回答",
		prompt,
		"Thinking it through",
		"1+1 等于 2。",
		"Complete",
	}
	got := ExtractPlainReply(chunks, []string{"旧问题", "旧回答"}, prompt)
	if got != "1+1 等于 2。" {
		t.Fatalf("got %q", got)
	}
}

func TestSendModeValidation(t *testing.T) {
	root := t.TempDir()
	if rc := cmdSend(root, []string{"--pair", "x", "--current", "--message", "hi"}); rc != 2 {
		t.Fatalf("pair+current rc = %d, want 2", rc)
	}
}

func TestBuildDispatchPrompt(t *testing.T) {
	got := BuildDispatchPrompt("20260101-120000-ab12cd", "  hello\n")
	want := "[task 20260101-120000-ab12cd]\n\nhello"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if strings.Contains(got, "Pair-to-Gemini") || strings.Contains(got, "pair-to-gemini") {
		t.Fatalf("dispatch prompt leaks the tool name: %q", got)
	}
}

func TestPairsAndTaskPersistence(t *testing.T) {
	root := t.TempDir()
	if rc := cmdPair(root, []string{"add", "--id", "rev", "--gemini-title", "Review Chat"}); rc != 0 {
		t.Fatalf("pair add failed: %d", rc)
	}
	// Duplicate title under different pair id must fail
	if rc := cmdPair(root, []string{"add", "--id", "rev2", "--gemini-title", "Review Chat"}); rc == 0 {
		t.Fatalf("expected duplicate title rejection")
	}

	sum, n := Sha256Text("hello world")
	rec := TaskRecord{
		TaskID:        "20261127-120000-112233",
		PairID:        "rev",
		GeminiTitle:   "Review Chat",
		Status:        "sent",
		Message:       "hello world",
		MessageBytes:  n,
		MessageSha256: sum,
		CreatedAt:     nowISO(),
	}
	if err := saveTask(root, rec); err != nil {
		t.Fatalf("saveTask: %v", err)
	}
	if rc := cmdTask(root, []string{"read", rec.TaskID}); rc != 0 {
		t.Fatalf("task read failed: %d", rc)
	}
}

// The baseline holds only the last 60 chunks of a long conversation; the reply
// must still exclude the older history.
func TestExtractPlainReplyLongConversation(t *testing.T) {
	var hist []string
	for i := 0; i < 80; i++ {
		hist = append(hist, fmt.Sprintf("old chunk %d", i))
	}
	base := hist[len(hist)-60:]
	prompt := BuildDispatchPrompt("20260101-120000-ab12cd", "follow up")

	withEcho := append(append([]string(nil), hist...), prompt, "new answer")
	if got := ExtractPlainReply(withEcho, base, prompt); got != "new answer" {
		t.Fatalf("with echo: got %q", got)
	}
	noEcho := append(append([]string(nil), hist...), "new answer")
	if got := ExtractPlainReply(noEcho, base, prompt); got != "new answer" {
		t.Fatalf("without echo: got %q", got)
	}
}

// The echoed task line may arrive as its own short chunk.
func TestExtractPlainReplyAnchorsOnTaskTag(t *testing.T) {
	prompt := BuildDispatchPrompt("20260101-120000-ab12cd", "What is 6 * 7?")
	chunks := []string{"unrelated", "[task 20260101-120000-ab12cd]", "What is 6 * 7?", "42."}
	if got := ExtractPlainReply(chunks, nil, prompt); got != "42." {
		t.Fatalf("got %q", got)
	}
}
