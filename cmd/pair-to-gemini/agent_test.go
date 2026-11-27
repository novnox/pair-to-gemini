package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderAgentPlist(t *testing.T) {
	got := renderAgentPlist("/Users/a&b/.local/bin/pair-to-gemini", "/Users/a&b/.local/share/pair-to-gemini", "/Users/a&b/root/proxy.sock")

	for _, want := range []string{
		"<string>" + agentLabel + "</string>",
		"<string>/Users/a&amp;b/.local/bin/pair-to-gemini</string>",
		"<string>/Users/a&amp;b/root/proxy.sock</string>",
		"<string>serve</string>",
		"<key>RunAtLoad</key>",
		"<key>KeepAlive</key>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist missing %q", want)
		}
	}
	if strings.Contains(got, "a&b") {
		t.Error("paths were not XML-escaped")
	}
	if !strings.HasPrefix(got, "<?xml") {
		t.Error("plist does not start with an XML declaration")
	}
}

func TestAgentPlistPath(t *testing.T) {
	if got := agentPlistPath(); filepath.Base(got) != agentLabel+".plist" {
		t.Fatalf("plist path = %q", got)
	}
}

func TestAgentBinaryReusable(t *testing.T) {
	if !agentBinaryReusable("/Users/someone/.local/bin/pair-to-gemini") {
		t.Error("installed binary should be reusable")
	}
	if agentBinaryReusable(filepath.Join(t.TempDir(), "pair-to-gemini")) {
		t.Error("binary in a temp dir should be rejected")
	}
	if agentBinaryReusable("/var/folders/xy/T/go-build123/b001/exe/pair-to-gemini") {
		t.Error("go run / go test binary should be rejected")
	}
	if agentBinaryReusable("/tmp/pair-to-gemini") {
		t.Error("binary in /tmp should be rejected")
	}
	if agentBinaryReusable("") {
		t.Error("empty path should be rejected")
	}
}
