package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const Version = "0.3.0"

type Pair struct {
	PairID      string `json:"pairId"`
	Label       string `json:"label"`
	GeminiTitle string `json:"geminiTitle"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type PairsFile struct {
	SchemaVersion int             `json:"schemaVersion"`
	Pairs         map[string]Pair `json:"pairs"`
}

type TaskRecord struct {
	SchemaVersion    int      `json:"schemaVersion"`
	TaskID           string   `json:"taskId"`
	PairID           string   `json:"pairId"`
	GeminiTitle      string   `json:"geminiTitle"`
	Status           string   `json:"status"` // in_flight, needs_attention, sent, failed
	Framed           bool     `json:"framed,omitempty"`
	Prompt           string   `json:"prompt,omitempty"`
	ConfirmationCard string   `json:"confirmationCard,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	BaselineChunks   []string `json:"baselineChunks,omitempty"`
	Message          string   `json:"message,omitempty"`
	MessageBytes     int      `json:"messageBytes,omitempty"`
	MessageSha256    string   `json:"messageSha256,omitempty"`
	CreatedAt        string   `json:"createdAt"`
	UpdatedAt        string   `json:"updatedAt"`
}

func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func randHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func makeTaskID() string {
	return time.Now().Format("20060102-150405") + "-" + randHex(3)
}

func defaultRoot() string {
	if v := strings.TrimSpace(os.Getenv("PAIR_TO_GEMINI_ROOT")); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local/share/pair-to-gemini")
}

func ensureDirs(root string) error {
	for _, sub := range []string{"", "tasks", "locks"} {
		d := filepath.Join(root, sub)
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func withFlock(lockPath string, fn func() error) error {
	fd, err := syscall.Open(lockPath, syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	return fn()
}

func atomicWriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := filepath.Join(filepath.Dir(path), fmt.Sprintf("._%s.%s.tmp", filepath.Base(path), randHex(6)))
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, path)
}

func loadPairs(root string) (PairsFile, error) {
	p := filepath.Join(root, "pairs.json")
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return PairsFile{SchemaVersion: 2, Pairs: map[string]Pair{}}, nil
	}
	if err != nil {
		return PairsFile{}, err
	}
	var pf PairsFile
	if err := json.Unmarshal(raw, &pf); err != nil {
		return PairsFile{}, err
	}
	if pf.Pairs == nil {
		pf.Pairs = map[string]Pair{}
	}
	if pf.SchemaVersion == 0 {
		pf.SchemaVersion = 2
	}
	return pf, nil
}

func savePairs(root string, pf PairsFile) error {
	pf.SchemaVersion = 2
	return atomicWriteJSON(filepath.Join(root, "pairs.json"), pf)
}

func taskPath(root, taskID string) string {
	return filepath.Join(root, "tasks", taskID+".json")
}

func loadTask(root, taskID string) (TaskRecord, error) {
	if !taskIDRe.MatchString(taskID) {
		return TaskRecord{}, errors.New("invalid_task_id")
	}
	raw, err := os.ReadFile(taskPath(root, taskID))
	if errors.Is(err, os.ErrNotExist) {
		return TaskRecord{}, errors.New("task_not_found")
	}
	if err != nil {
		return TaskRecord{}, err
	}
	var tr TaskRecord
	if err := json.Unmarshal(raw, &tr); err != nil {
		return TaskRecord{}, err
	}
	return tr, nil
}

func saveTask(root string, tr TaskRecord) error {
	tr.SchemaVersion = 2
	tr.UpdatedAt = nowISO()
	return atomicWriteJSON(taskPath(root, tr.TaskID), tr)
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func failErr(code string, exit int) int {
	b, _ := json.Marshal(map[string]any{"ok": false, "error": code})
	fmt.Fprintln(os.Stderr, string(b))
	return exit
}

// parseFlags reports whether the caller should return. Asking for help is a
// successful invocation: the flag package prints the usage and returns
// flag.ErrHelp, which is not bad input.
func parseFlags(fs *flag.FlagSet, args []string) (stop bool, code int) {
	err := fs.Parse(args)
	if err == nil {
		return false, 0
	}
	if errors.Is(err, flag.ErrHelp) {
		return true, 0
	}
	return true, failErr("invalid_args", 2)
}

// parseFlagsArgs is parseFlags for subcommands that need a positional argument.
func parseFlagsArgs(fs *flag.FlagSet, args []string, need int) (stop bool, code int) {
	if stop, code := parseFlags(fs, args); stop {
		return stop, code
	}
	if fs.NArg() < need {
		return true, failErr("invalid_args", 2)
	}
	return false, 0
}

func waitTaskCompletion(root string, tr TaskRecord, timeoutSec float64, returnOnAttention bool) (int, map[string]any) {
	if timeoutSec < 1 {
		timeoutSec = 1
	}
	deadline := time.Now().Add(time.Duration(timeoutSec * float64(time.Second)))
	uiLock := filepath.Join(root, "locks", "gemini_ui.lock")

	hasTitle := strings.TrimSpace(tr.GeminiTitle) != ""
	observedTitle := tr.GeminiTitle

	var stableLen = -1
	var stableCount = 0
	var debugLast = "\x00"
	dispatchedAt, _ := time.Parse("2006-01-02T15:04:05.000Z", tr.CreatedAt)

	for time.Now().Before(deadline) {
		if isScreenLocked() {
			time.Sleep(time.Second)
			continue
		}
		pid := findGeminiPID()
		if pid == 0 {
			time.Sleep(time.Second)
			continue
		}

		var snap PollSnapshot
		var badge string
		got := false
		badgeReady := false
		// Each step holds the UI lock briefly: read a badge, or read one
		// completed conversation. Concurrent tasks interleave between steps.
		_ = withFlock(uiLock, func() error {
			// The sidebar scan costs about two seconds; skip it when this
			// task's chat is already the active window and read it directly.
			if hasTitle && NormalizeWindowTitle(getWindowTitle(pid)) != tr.GeminiTitle {
				badge = getConversationBadge(pid, tr.GeminiTitle)
				// "ready" means finished. No badge means the chat is active
				// or its badge was cleared after being viewed; open it and
				// rely on the stability check below.
				if badge == "ready" || badge == "" {
					if ok, _ := openConversation(pid, tr.GeminiTitle); ok {
						snap = pollConversation(pid)
						got = true
						badgeReady = badge == "ready"
					}
				}
			}
			if !got {
				live := NormalizeWindowTitle(getWindowTitle(pid))
				// Read the active window only when it is this task's chat.
				if !hasTitle || live == tr.GeminiTitle {
					snap = pollConversation(pid)
					got = snap.HasWindow
				}
			}
			return nil
		})
		// Without a title the active window may belong to another task; only
		// accept it when it shows this task's own "[task <id>]" line.
		if !got || (!hasTitle && !chunksContain(snap.Chunks, "[task "+tr.TaskID+"]")) {
			time.Sleep(time.Second)
			continue
		}

		if observedTitle == "" && plausibleTitle(snap.WindowTitle) {
			observedTitle = snap.WindowTitle
		}

		if badge == "needs_attention" || snap.Modal || snap.ConfirmationCard != "" {
			tr.Status = "needs_attention"
			tr.ConfirmationCard = snap.ConfirmationCard
			tr.GeminiTitle = observedTitle
			_ = saveTask(root, tr)
			if returnOnAttention {
				reason := "manual_confirmation_required"
				if snap.Modal && snap.ConfirmationCard == "" {
					reason = "dialog_open"
				}
				return 3, map[string]any{
					"ok":               false,
					"completed":        false,
					"taskId":           tr.TaskID,
					"pairId":           tr.PairID,
					"geminiTitle":      observedTitle,
					"status":           "needs_attention",
					"confirmationCard": snap.ConfirmationCard,
					"reason":           reason,
				}
			}
			time.Sleep(1500 * time.Millisecond)
			continue
		}

		if (hasTitle && !badgeReady && snap.WindowTitle != tr.GeminiTitle) || badge == "in_progress" || snap.Busy {
			stableCount = 0
			time.Sleep(1200 * time.Millisecond)
			continue
		}

		if !badgeReady && !dispatchedAt.IsZero() && time.Since(dispatchedAt) < 3*time.Second {
			time.Sleep(800 * time.Millisecond)
			continue
		}

		var hasSentinel bool
		var reply string
		if tr.Framed {
			hasSentinel, reply = ExtractGeminiReply(snap.Chunks, tr.BaselineChunks, tr.TaskID)
		} else {
			reply = ExtractPlainReply(snap.Chunks, tr.BaselineChunks, tr.Prompt)
		}
		if reply == "" {
			time.Sleep(1000 * time.Millisecond)
			continue
		}

		if os.Getenv("P2G_DEBUG_CHUNKS") != "" && reply != debugLast {
			debugLast = reply
			fmt.Fprintf(os.Stderr, "pair-to-gemini: chunks (%d), reply=%q\n", len(snap.Chunks), reply)
			for i, c := range snap.Chunks {
				fmt.Fprintf(os.Stderr, "  %02d %q\n", i, c)
			}
		}

		if len(reply) == stableLen {
			stableCount++
		} else {
			stableLen = len(reply)
			stableCount = 1
		}

		// A ready badge means generation finished; one read is enough.
		if (badgeReady && stableCount >= 1) || (hasSentinel && stableCount >= 1) || stableCount >= 3 {
			if tr.GeminiTitle == "" && plausibleTitle(observedTitle) {
				tr.GeminiTitle = observedTitle
			}
			sha, nBytes := Sha256Text(reply)
			tr.Status = "sent"
			tr.ConfirmationCard = ""
			tr.Message = reply
			tr.MessageBytes = nBytes
			tr.MessageSha256 = sha
			_ = saveTask(root, tr)
			return 0, map[string]any{
				"ok":            true,
				"completed":     true,
				"taskId":        tr.TaskID,
				"pairId":        tr.PairID,
				"geminiTitle":   tr.GeminiTitle,
				"status":        "sent",
				"message":       reply,
				"messageBytes":  nBytes,
				"messageSha256": sha,
			}
		}
		time.Sleep(1000 * time.Millisecond)
	}

	// Only a title seen in this task's own snapshot is kept; the live window
	// may show another task's chat.
	tr.GeminiTitle = observedTitle
	if !plausibleTitle(tr.GeminiTitle) {
		tr.GeminiTitle = ""
	}
	_ = saveTask(root, tr)
	return 1, map[string]any{
		"ok":          false,
		"completed":   false,
		"taskId":      tr.TaskID,
		"pairId":      tr.PairID,
		"geminiTitle": tr.GeminiTitle,
		"status":      tr.Status,
		"reason":      "wait_timeout",
	}
}

func cmdDoctor(root string, asJSON bool) int {
	_ = ensureDirs(root)
	pf, err := loadPairs(root)
	if err != nil {
		return failErr("corrupt_pairs_json", 2)
	}
	pid := findGeminiPID()
	trusted := axIsTrusted()
	locked := isScreenLocked()
	winTitle := ""
	if trusted && pid > 0 {
		winTitle = getWindowTitle(pid)
	}
	ver := geminiAppVersion()
	rep := map[string]any{
		"ok":                trusted && !locked && pid > 0,
		"version":           Version,
		"root":              root,
		"pairCount":         len(pf.Pairs),
		"axTrusted":         trusted,
		"screenLocked":      locked,
		"geminiRunning":     pid > 0,
		"geminiPid":         pid,
		"geminiAppVersion":  ver,
		"geminiWindowTitle": winTitle,
	}
	if asJSON {
		printJSON(rep)
	} else {
		fmt.Printf("pair-to-gemini %s\n", Version)
		fmt.Printf("Gemini.app: running=%v pid=%d ver=%s window=%q\n", pid > 0, pid, ver, winTitle)
		fmt.Printf("Permissions: axTrusted=%v screenLocked=%v | Pairs=%d\n", trusted, locked, len(pf.Pairs))
	}
	return 0
}

func cmdChats() int {
	if !axIsTrusted() {
		return failErr("ax_not_trusted", 2)
	}
	pid := findGeminiPID()
	if pid == 0 {
		return failErr("gemini_not_running", 2)
	}
	hasWin, curTitle, items := listConversations(pid)
	printJSON(map[string]any{
		"hasWindow":     hasWin,
		"currentTitle":  curTitle,
		"conversations": items,
	})
	return 0
}

func cmdPair(root string, args []string) int {
	if len(args) == 0 {
		return failErr("missing_pair_subcommand", 2)
	}
	if a := args[0]; a == "-h" || a == "--help" || a == "help" {
		usage()
		return 0
	}
	if err := ensureDirs(root); err != nil {
		return failErr("io_error", 2)
	}
	lockP := filepath.Join(root, "locks", "pairs.lock")
	sub := args[0]

	switch sub {
	case "list":
		pf, err := loadPairs(root)
		if err != nil {
			return failErr("corrupt_pairs_json", 2)
		}
		keys := make([]string, 0, len(pf.Pairs))
		for k := range pf.Pairs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]Pair, 0, len(keys))
		for _, k := range keys {
			out = append(out, pf.Pairs[k])
		}
		printJSON(map[string]any{"ok": true, "pairs": out})
		return 0

	case "add":
		fs := flag.NewFlagSet("pair add", flag.ContinueOnError)
		id := fs.String("id", "", "pair id")
		title := fs.String("gemini-title", "", "Gemini conversation title")
		label := fs.String("label", "", "human label")
		if stop, code := parseFlags(fs, args[1:]); stop {
			return code
		}
		if !pairIDRe.MatchString(*id) || strings.TrimSpace(*title) == "" {
			return failErr("invalid_pair_id_or_title", 2)
		}
		saved, err := bindPair(root, *id, *label, strings.TrimSpace(*title))
		if err != nil {
			return failErr(err.Error(), 2)
		}
		printJSON(map[string]any{"ok": true, "pair": saved})
		return 0

	case "init":
		fs := flag.NewFlagSet("pair init", flag.ContinueOnError)
		id := fs.String("id", "", "pair id")
		title := fs.String("gemini-title", "", "target Gemini Task chat title")
		label := fs.String("label", "", "human label")
		ws := fs.String("workspace", "", "optional bookmarked workspace path")
		initPrompt := fs.String("init-prompt", "", "custom init prompt")
		waitSec := fs.Float64("wait-sec", 40, "seconds to wait")
		if stop, code := parseFlags(fs, args[1:]); stop {
			return code
		}
		if !pairIDRe.MatchString(*id) || strings.TrimSpace(*title) == "" {
			return failErr("invalid_pair_id_or_title", 2)
		}
		if !axIsTrusted() {
			return failErr("ax_not_trusted", 2)
		}
		pid := findGeminiPID()
		if pid == 0 {
			return failErr("gemini_not_running", 2)
		}
		tTrim := strings.TrimSpace(*title)
		pfCheck, err := loadPairs(root)
		if err != nil {
			return failErr("corrupt_pairs_json", 2)
		}
		if titleTaken(pfCheck, *id, tTrim) {
			return failErr("duplicate_gemini_title_in_pairs", 2)
		}
		wsAbs := ""
		if strings.TrimSpace(*ws) != "" {
			p, err := filepath.Abs(*ws)
			if err != nil {
				return failErr("invalid_workspace", 2)
			}
			wsAbs = p
		}
		prompt := *initPrompt
		if prompt == "" {
			prompt = "Reply with READY if you can see this message. Do not modify files or run commands."
		}
		var actualTitle, titleMode, failReason string
		_ = withFlock(filepath.Join(root, "locks", "gemini_ui.lock"), func() error {
			actualTitle, titleMode, failReason = createTaskConversation(pid, tTrim, prompt, wsAbs, *waitSec)
			return nil
		})
		if failReason != "" {
			return failErr("gemini_init_failed:"+failReason, 2)
		}
		// Rename can fail, leaving Gemini's own title; bindPair checks
		// that title for duplicates too.
		saved, err := bindPair(root, *id, *label, actualTitle)
		if err != nil {
			return failErr(err.Error(), 2)
		}
		printJSON(map[string]any{"ok": true, "titleMode": titleMode, "pair": saved})
		return 0

	case "remove":
		fs := flag.NewFlagSet("pair remove", flag.ContinueOnError)
		if stop, code := parseFlagsArgs(fs, args[1:], 1); stop {
			return code
		}
		targetID := fs.Arg(0)
		err := withFlock(lockP, func() error {
			pf, err := loadPairs(root)
			if err != nil {
				return err
			}
			if _, ok := pf.Pairs[targetID]; !ok {
				return errors.New("unknown_pair_id")
			}
			delete(pf.Pairs, targetID)
			return savePairs(root, pf)
		})
		if err != nil {
			return failErr(err.Error(), 2)
		}
		printJSON(map[string]any{"ok": true, "removed": targetID})
		return 0
	}
	return failErr("unknown_pair_subcommand", 2)
}

func maxConcurrency() int {
	v := strings.TrimSpace(os.Getenv("P2G_MAX_CONCURRENCY"))
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 2
	}
	return n
}

func titleTaken(pf PairsFile, id, title string) bool {
	for k, existing := range pf.Pairs {
		if k != id && existing.GeminiTitle == title {
			return true
		}
	}
	return false
}

// bindPair creates or updates a pair under the pairs lock.
func bindPair(root, id, label, title string) (Pair, error) {
	var saved Pair
	err := withFlock(filepath.Join(root, "locks", "pairs.lock"), func() error {
		pf, err := loadPairs(root)
		if err != nil {
			return err
		}
		if titleTaken(pf, id, title) {
			return errors.New("duplicate_gemini_title_in_pairs")
		}
		now := nowISO()
		created := now
		if prev, ok := pf.Pairs[id]; ok && prev.CreatedAt != "" {
			created = prev.CreatedAt
		}
		if label == "" {
			label = id
		}
		saved = Pair{PairID: id, Label: label, GeminiTitle: title, CreatedAt: created, UpdatedAt: now}
		pf.Pairs[id] = saved
		return savePairs(root, pf)
	})
	return saved, err
}

func cmdSend(root string, args []string) int {
	if err := ensureDirs(root); err != nil {
		return failErr("io_error", 2)
	}
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	pairID := fs.String("pair", "", "reuse an existing pair's conversation")
	currentFlag := fs.Bool("current", false, "send to the conversation that is already open")
	msgFlag := fs.String("message", "", "prompt text")
	fileFlag := fs.String("file", "", "prompt file path")
	stdinFlag := fs.Bool("stdin", false, "read prompt from stdin")
	customTaskID := fs.String("task-id", "", "optional custom task id")
	waitFlag := fs.Bool("wait", true, "wait for Gemini reply")
	retAtt := fs.Bool("return-on-attention", true, "return exit 3 if Gemini shows confirmation card")
	timeout := fs.Float64("timeout", 300, "wait timeout seconds")
	textOut := fs.Bool("text", false, "print only the reply text")
	// Optional: keep the legacy framed prompt with sentinel tags for stricter
	// reply extraction. Plain prompts are the default so the chat looks natural.
	framed := fs.Bool("framed", false, "wrap the prompt with paired sentinel tags")
	if stop, code := parseFlags(fs, args); stop {
		return code
	}

	var body string
	sources := 0
	if *msgFlag != "" {
		sources++
		body = *msgFlag
	}
	if *fileFlag != "" {
		sources++
		b, err := os.ReadFile(*fileFlag)
		if err != nil {
			return failErr("file_read_error", 2)
		}
		body = string(b)
	}
	if *stdinFlag {
		sources++
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return failErr("stdin_read_error", 2)
		}
		body = string(b)
	}
	// Convenience: `send "question"` uses the positional text as the prompt.
	if sources == 0 && fs.NArg() > 0 {
		sources++
		body = strings.Join(fs.Args(), " ")
	}
	if sources != 1 || strings.TrimSpace(body) == "" {
		return failErr("specify_exactly_one_nonempty_message_source", 2)
	}

	pair := Pair{}
	hasPair := strings.TrimSpace(*pairID) != ""
	useCurrent := *currentFlag
	if hasPair && useCurrent {
		return failErr("pair_and_current_are_mutually_exclusive", 2)
	}
	if hasPair {
		pf, err := loadPairs(root)
		if err != nil {
			return failErr("corrupt_pairs_json", 2)
		}
		var found bool
		pair, found = pf.Pairs[*pairID]
		if !found {
			return failErr("unknown_pair", 2)
		}
	}
	if !axIsTrusted() {
		return failErr("ax_not_trusted", 2)
	}
	if isScreenLocked() {
		return failErr("screen_locked", 2)
	}
	pid := findGeminiPID()
	if pid == 0 {
		return failErr("gemini_not_running", 2)
	}

	tid := *customTaskID
	if tid == "" {
		tid = makeTaskID()
	}
	if !taskIDRe.MatchString(tid) {
		return failErr("invalid_task_id", 2)
	}

	var baseline []string
	var subOk bool
	var subReason string
	currentTitle := ""
	capturedTitle := ""
	outbound := BuildDispatchPrompt(tid, body)
	if *framed {
		outbound = BuildFramedPrompt(tid, body)
	}
	uiLock := filepath.Join(root, "locks", "gemini_ui.lock")
	dispatchDeadline := time.Now().Add(90 * time.Second)
	for {
		subOk = false
		_ = withFlock(uiLock, func() error {
			if busyConversationCount(pid) >= maxConcurrency() {
				subReason = "concurrency_limit"
				return nil
			}
			switch {
			case hasPair:
				openOk, openReason := openConversation(pid, pair.GeminiTitle)
				if !openOk {
					subReason = openReason
					return nil
				}
			case useCurrent:
				if !ensureMainWindowSilent(pid) {
					subReason = "no_main_window"
					return nil
				}
			default:
				if ok, reason := startNewConversation(pid); !ok {
					subReason = reason
					return nil
				}
			}
			snap := pollConversation(pid)
			if hasPair && snap.WindowTitle != pair.GeminiTitle {
				subReason = "title_verification_failed"
				return nil
			}
			if snap.Busy || snap.ConfirmationCard != "" || snap.Modal {
				subReason = "conversation_busy_or_needs_attention"
				return nil
			}
			if useCurrent {
				currentTitle = snap.WindowTitle
			}
			if len(snap.Chunks) > 60 {
				baseline = append([]string(nil), snap.Chunks[len(snap.Chunks)-60:]...)
			} else {
				baseline = append([]string(nil), snap.Chunks...)
			}
			subOk, subReason = writePromptAndSubmit(pid, outbound)
			if subOk && !hasPair && !plausibleTitle(currentTitle) {
				capturedTitle = captureStableTitle(pid)
			}
			return nil
		})
		if subOk || subReason != "concurrency_limit" {
			break
		}
		if time.Now().After(dispatchDeadline) {
			return failErr("concurrency_limit", 2)
		}
		time.Sleep(time.Second)
	}

	// The press may have gone through even when it could not be confirmed.
	// Record the task so the caller waits for it instead of resending.
	unconfirmed := !subOk && (subReason == "submit_press_uncertain" || subReason == "submit_unconfirmed")
	if !subOk && !unconfirmed {
		return failErr("dispatch_failed:"+subReason, 2)
	}
	recReason := ""
	if unconfirmed {
		recReason = subReason
	}

	title := pair.GeminiTitle
	if useCurrent {
		title = currentTitle
	}
	if capturedTitle != "" {
		title = capturedTitle
	}
	rec := TaskRecord{
		SchemaVersion:  2,
		TaskID:         tid,
		PairID:         pair.PairID,
		GeminiTitle:    title,
		Status:         "in_flight",
		Reason:         recReason,
		Framed:         *framed,
		Prompt:         outbound,
		BaselineChunks: baseline,
		CreatedAt:      nowISO(),
		UpdatedAt:      nowISO(),
	}
	if err := saveTask(root, rec); err != nil {
		return failErr("task_persist_failed", 2)
	}

	if !*waitFlag {
		out := map[string]any{
			"ok":          true,
			"completed":   false,
			"taskId":      tid,
			"pairId":      pair.PairID,
			"geminiTitle": title,
			"status":      "in_flight",
		}
		if rec.Reason != "" {
			out["reason"] = rec.Reason
		}
		printJSON(out)
		return 0
	}

	rc, out := waitTaskCompletion(root, rec, *timeout, *retAtt)
	if unconfirmed {
		out["submitUnconfirmed"] = true
	}
	if *textOut && *waitFlag {
		if rc == 0 {
			if msg, ok := out["message"].(string); ok {
				fmt.Println(msg)
			}
			return 0
		}
		reason, _ := out["reason"].(string)
		if reason == "" {
			reason = "request_failed"
		}
		fmt.Fprintln(os.Stderr, reason)
		return rc
	}
	printJSON(out)
	return rc
}

func cmdTask(root string, args []string) int {
	if len(args) == 0 {
		return failErr("missing_task_subcommand", 2)
	}
	if a := args[0]; a == "-h" || a == "--help" || a == "help" {
		usage()
		return 0
	}
	sub := args[0]
	switch sub {
	case "status":
		fs := flag.NewFlagSet("task status", flag.ContinueOnError)
		if stop, code := parseFlagsArgs(fs, args[1:], 1); stop {
			return code
		}
		tr, err := loadTask(root, fs.Arg(0))
		if err != nil {
			return failErr(err.Error(), 2)
		}
		printJSON(tr)
		return 0

	case "read":
		fs := flag.NewFlagSet("task read", flag.ContinueOnError)
		if stop, code := parseFlagsArgs(fs, args[1:], 1); stop {
			return code
		}
		tr, err := loadTask(root, fs.Arg(0))
		if err != nil {
			return failErr(err.Error(), 2)
		}
		if tr.Status != "sent" {
			return failErr("task_not_completed:"+tr.Status, 2)
		}
		sum, n := Sha256Text(tr.Message)
		if tr.MessageSha256 != "" && sum != tr.MessageSha256 {
			return failErr("archived_message_sha256_mismatch", 2)
		}
		printJSON(map[string]any{
			"ok":            true,
			"taskId":        tr.TaskID,
			"pairId":        tr.PairID,
			"geminiTitle":   tr.GeminiTitle,
			"status":        "sent",
			"message":       tr.Message,
			"messageBytes":  n,
			"messageSha256": sum,
		})
		return 0

	case "wait":
		// Support flags before or after positional task_id
		var flagArgs []string
		var taskID string
		rest := args[1:]
		for i := 0; i < len(rest); i++ {
			tok := rest[i]
			if strings.HasPrefix(tok, "-") {
				flagArgs = append(flagArgs, tok)
				if (tok == "--timeout" || tok == "-timeout") && i+1 < len(rest) {
					i++
					flagArgs = append(flagArgs, rest[i])
				}
			} else if taskID == "" {
				taskID = tok
			}
		}
		fs := flag.NewFlagSet("task wait", flag.ContinueOnError)
		timeout := fs.Float64("timeout", 300, "wait timeout seconds")
		retAtt := fs.Bool("return-on-attention", true, "return exit 3 on confirmation card")
		if stop, code := parseFlags(fs, flagArgs); stop {
			return code
		}
		if taskID == "" {
			return failErr("invalid_args", 2)
		}
		tr, err := loadTask(root, taskID)
		if err != nil {
			return failErr(err.Error(), 2)
		}
		if tr.Status == "sent" {
			printJSON(map[string]any{
				"ok":            true,
				"completed":     true,
				"taskId":        tr.TaskID,
				"pairId":        tr.PairID,
				"geminiTitle":   tr.GeminiTitle,
				"status":        "sent",
				"message":       tr.Message,
				"messageBytes":  tr.MessageBytes,
				"messageSha256": tr.MessageSha256,
			})
			return 0
		}
		if latest, err := loadTask(root, taskID); err == nil {
			tr = latest
		}
		rc, out := waitTaskCompletion(root, tr, *timeout, *retAtt)
		printJSON(out)
		return rc
	}
	return failErr("unknown_task_subcommand", 2)
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: pair-to-gemini [--root <dir>] <command> [options]

Commands:
  doctor [--json]                   Check Gemini.app, Accessibility, screen lock, and pairs
  chats                             List visible Gemini.app conversations
  pair list                         List configured 1-to-1 pairs
  pair init --id <id> --gemini-title <title> [--workspace <dir>] [--label <lbl>]
  pair add  --id <id> --gemini-title <title> [--label <lbl>]
  pair remove <id>
  send [--pair <id> | --current]
       (--message <text> | --file <path> | --stdin | "<prompt>") [--framed] [--text] [--wait=true] [--timeout 300]
  task status <taskId>
  task wait <taskId> [--timeout 300] [--return-on-attention]
  task read <taskId>
  serve [--socket <path>]           Run the local proxy daemon (macOS GUI session)
  client [--socket <path>] -- <command> [args...]
                                    Forward one command to a running serve
  agent install [--bin <path>]      Install the proxy as a LaunchAgent
  agent status                      Show the agent's plist, load state, and socket
  agent uninstall                   Remove the LaunchAgent (keeps the data directory)`)
}

func main() {
	root := defaultRoot()
	args := os.Args[1:]
	if len(args) >= 2 && args[0] == "--root" {
		root = args[1]
		args = args[2:]
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage()
		os.Exit(0)
	}
	cmd := args[0]
	rest := args[1:]
	switch cmd {
	case "doctor":
		asJSON := false
		for _, a := range rest {
			if a == "--json" {
				asJSON = true
			}
		}
		os.Exit(cmdDoctor(root, asJSON))
	case "chats":
		os.Exit(cmdChats())
	case "pair":
		os.Exit(cmdPair(root, rest))
	case "send":
		os.Exit(cmdSend(root, rest))
	case "task":
		os.Exit(cmdTask(root, rest))
	case "agent":
		os.Exit(cmdAgent(root, rest))
	case "serve":
		os.Exit(cmdServe(root, rest))
	case "client":
		os.Exit(cmdClient(root, rest))
	default:
		usage()
		os.Exit(2)
	}
}
