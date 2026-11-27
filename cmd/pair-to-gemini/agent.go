package main

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The proxy runs as a per-user LaunchAgent. SSH sessions cannot reach the
// Accessibility API, but a process in the GUI session can.
const agentLabel = "com.pairtogemini.proxy"

func agentPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist")
}

func agentLogPath(root string) string {
	return filepath.Join(root, "logs", "proxy.log")
}

func agentDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func escapeXML(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// renderAgentPlist builds the property list. Every path is escaped because a
// home directory can contain & or <.
func renderAgentPlist(bin, root, socket string) string {
	logPath := agentLogPath(root)
	esc := escapeXML
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>serve</string>
		<string>--socket</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>WorkingDirectory</key>
	<string>%s</string>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, agentLabel, esc(bin), esc(socket), esc(root), esc(logPath), esc(logPath))
}

// agentBinaryReusable rejects binaries that will not exist when launchd starts
// the agent again: `go run` and `go test` build into a cache or temp directory.
func agentBinaryReusable(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	for _, unstable := range []string{
		os.TempDir(),
		"/tmp/",
		"/private/tmp/",
		"/var/folders/",
		"/private/var/folders/",
		"/go-build",
	} {
		if unstable != "" && strings.HasPrefix(path, unstable) {
			return false
		}
	}
	return true
}

func agentInstall(root, bin string) int {
	if !agentBinaryReusable(bin) {
		return failErr("unstable_binary_path", 2)
	}
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		return failErr("missing_binary", 2)
	}
	if err := ensureDirs(root); err != nil {
		return failErr("io_error", 2)
	}
	logPath := agentLogPath(root)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return failErr("io_error", 2)
	}
	socket := defaultSocketPath(root)
	plist := agentPlistPath()
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return failErr("io_error", 2)
	}
	if err := os.WriteFile(plist, []byte(renderAgentPlist(bin, root, socket)), 0o600); err != nil {
		return failErr("io_error", 2)
	}
	domain := agentDomain()
	// Reinstalling over a loaded agent fails unless it is booted out first, and
	// launchd needs a moment before the label can be bootstrapped again.
	if exec.Command("launchctl", "print", domain+"/"+agentLabel).Run() == nil {
		_ = exec.Command("launchctl", "bootout", domain+"/"+agentLabel).Run()
		time.Sleep(700 * time.Millisecond)
	}
	var lastOut []byte
	bootstrapped := false
	for attempt := 0; attempt < 5 && !bootstrapped; attempt++ {
		out, err := exec.Command("launchctl", "bootstrap", domain, plist).CombinedOutput()
		if err == nil {
			bootstrapped = true
			break
		}
		lastOut = out
		time.Sleep(400 * time.Millisecond)
	}
	if !bootstrapped {
		fmt.Fprintln(os.Stderr, strings.TrimSpace(string(lastOut)))
		return failErr("agent_bootstrap_failed", 2)
	}
	_ = exec.Command("launchctl", "kickstart", "-k", domain+"/"+agentLabel).Run()

	printJSON(map[string]any{
		"ok":     true,
		"label":  agentLabel,
		"plist":  plist,
		"bin":    bin,
		"socket": socket,
		"log":    logPath,
		"next":   "grant Accessibility to " + bin + ", then check with: " + bin + " client -- doctor --json",
	})
	return 0
}

func agentStatus(root string) int {
	plist := agentPlistPath()
	_, plistErr := os.Stat(plist)
	socket := defaultSocketPath(root)
	_, sockErr := os.Stat(socket)
	loaded := exec.Command("launchctl", "print", agentDomain()+"/"+agentLabel).Run() == nil
	printJSON(map[string]any{
		"ok":            true,
		"label":         agentLabel,
		"plist":         plist,
		"installed":     plistErr == nil,
		"loaded":        loaded,
		"socket":        socket,
		"socketPresent": sockErr == nil,
	})
	return 0
}

func agentUninstall() int {
	plist := agentPlistPath()
	_ = exec.Command("launchctl", "bootout", agentDomain()+"/"+agentLabel).Run()
	_ = os.Remove(plist)
	printJSON(map[string]any{
		"ok":       true,
		"label":    agentLabel,
		"plist":    plist,
		"removed":  true,
		"dataKept": defaultRoot(),
	})
	return 0
}

func cmdAgent(root string, args []string) int {
	if len(args) == 0 {
		return failErr("missing_agent_subcommand", 2)
	}
	if a := args[0]; a == "-h" || a == "--help" || a == "help" {
		usage()
		return 0
	}
	switch args[0] {
	case "install":
		fs := flag.NewFlagSet("agent install", flag.ContinueOnError)
		bin := fs.String("bin", "", "binary the agent should run (default: this executable)")
		if stop, code := parseFlags(fs, args[1:]); stop {
			return code
		}
		if *bin == "" {
			exe, err := os.Executable()
			if err != nil {
				return failErr("io_error", 2)
			}
			*bin = exe
		}
		return agentInstall(root, *bin)
	case "status":
		return agentStatus(root)
	case "uninstall":
		return agentUninstall()
	default:
		return failErr("unknown_agent_subcommand", 2)
	}
}
