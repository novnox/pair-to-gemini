package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withFakeExec(t *testing.T, fn func(root string, req proxyRequest) proxyResponse) {
	t.Helper()
	old := proxyExec
	proxyExec = fn
	t.Cleanup(func() { proxyExec = old })
}

func TestHandleConnRoundTrip(t *testing.T) {
	withFakeExec(t, func(root string, req proxyRequest) proxyResponse {
		return proxyResponse{
			Stdout:   root + "|" + strings.Join(req.Args, ",") + "|" + req.Stdin,
			ExitCode: 3,
		}
	})

	client, server := net.Pipe()
	defer client.Close()
	go func() {
		handleConn(server, "/tmp/root")
		server.Close()
	}()

	if err := json.NewEncoder(client).Encode(proxyRequest{
		Args:  []string{"send", "--pair", "default"},
		Stdin: "hello",
	}); err != nil {
		t.Fatalf("encode request: %v", err)
	}
	var resp proxyResponse
	if err := json.NewDecoder(client).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", resp.ExitCode)
	}
	want := "/tmp/root|send,--pair,default|hello"
	if resp.Stdout != want {
		t.Fatalf("stdout = %q, want %q", resp.Stdout, want)
	}
}

func TestHandleConnRejectsInvalidJSON(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		handleConn(server, "/tmp/root")
		server.Close()
	}()

	if _, err := client.Write([]byte("not json\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	var resp proxyResponse
	if err := json.NewDecoder(client).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ExitCode != 2 || !strings.HasPrefix(resp.Error, "invalid_request") {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

// unixSocketPath returns a short path: macOS limits sun_path to 104 bytes.
func unixSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "p2g")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "p.sock")
}

func TestServeSocketPermissionsAndClient(t *testing.T) {
	withFakeExec(t, func(root string, req proxyRequest) proxyResponse {
		return proxyResponse{Stdout: strings.Join(req.Args, " "), ExitCode: 0}
	})

	socket := unixSocketPath(t)
	ln, err := listenUnix(socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	st, err := os.Stat(socket)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket mode = %o, want 600", perm)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		handleConn(conn, "/tmp/root")
		conn.Close()
	}()

	resp, err := callProxy(socket, proxyRequest{Args: []string{"doctor", "--json"}}, 5*time.Second)
	if err != nil {
		t.Fatalf("call proxy: %v", err)
	}
	if resp.ExitCode != 0 || resp.Stdout != "doctor --json" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	<-done
}

func TestListenUnixRefusesLiveSocket(t *testing.T) {
	socket := unixSocketPath(t)
	first, err := listenUnix(socket)
	if err != nil {
		t.Fatalf("first listen: %v", err)
	}
	defer first.Close()

	if _, err := listenUnix(socket); err == nil {
		t.Fatal("second listen succeeded, want error for a live socket")
	}
}

func TestExecSelfRejectsRecursion(t *testing.T) {
	for _, args := range [][]string{
		{"serve"},
		{"client", "--", "doctor"},
		// A --root prefix must not hide the command from the check.
		{"--root", "/tmp/other", "serve"},
		{"--root", "/tmp/other", "client", "--", "doctor"},
	} {
		resp := execSelf("/tmp/root", proxyRequest{Args: args})
		if resp.ExitCode != 2 || !strings.Contains(resp.Error, "refusing") {
			t.Fatalf("%v: unexpected response %+v", args, resp)
		}
	}
	if resp := execSelf("/tmp/root", proxyRequest{}); resp.ExitCode != 2 {
		t.Fatalf("empty args: unexpected response %+v", resp)
	}
}

func TestClientTransportErrorExitCode(t *testing.T) {
	if rc := cmdClient(t.TempDir(), []string{"--socket", unixSocketPath(t), "--", "doctor"}); rc != exitTransport {
		t.Fatalf("rc = %d, want %d", rc, exitTransport)
	}
}
