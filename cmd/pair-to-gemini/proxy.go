package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The proxy lets a trusted process (a LaunchAgent in the user's GUI session)
// execute pair-to-gemini commands on behalf of an untrusted caller, typically
// an SSH session. The transport is a Unix domain socket, so nothing listens on
// a network port and only the same user can reach it.

const maxRequestBytes = 32 << 20 // 32 MiB

// exitTransport means the command never ran: the socket, the proxy, or the
// child process failed. It is distinct from exit 1 ("still generating").
const exitTransport = 4

type proxyRequest struct {
	Args  []string `json:"args"`
	Stdin string   `json:"stdin,omitempty"`
}

type proxyResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
	Error    string `json:"error,omitempty"`
}

// proxyExec runs one request. It is a variable so tests can replace it.
var proxyExec = execSelf

func defaultSocketPath(root string) string {
	return filepath.Join(root, "proxy.sock")
}

func cmdServe(root string, args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	socket := fs.String("socket", defaultSocketPath(root), "Unix socket path")
	if stop, code := parseFlags(fs, args); stop {
		return code
	}
	if err := serveSocket(root, *socket); err != nil {
		fmt.Fprintf(os.Stderr, "pair-to-gemini serve: %v\n", err)
		return 1
	}
	return 0
}

func serveSocket(root, socketPath string) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return err
	}
	ln, err := listenUnix(socketPath)
	if err != nil {
		return err
	}
	defer ln.Close()

	fmt.Fprintf(os.Stderr, "pair-to-gemini serve: listening on %s (pid %d)\n", socketPath, os.Getpid())
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		// Each request runs as its own process; the CLI's UI lock makes the
		// actual UI steps time-slice, so connections may overlap.
		go func(c net.Conn) {
			defer c.Close()
			handleConn(c, root)
		}(conn)
	}
}

func listenUnix(socketPath string) (net.Listener, error) {
	if c, err := net.DialTimeout("unix", socketPath, 300*time.Millisecond); err == nil {
		c.Close()
		return nil, fmt.Errorf("another server is already listening on %s", socketPath)
	}
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

func handleConn(conn net.Conn, root string) {
	dec := json.NewDecoder(io.LimitReader(conn, maxRequestBytes))
	var req proxyRequest
	if err := dec.Decode(&req); err != nil {
		writeResponse(conn, proxyResponse{ExitCode: 2, Error: "invalid_request: " + err.Error()})
		return
	}
	writeResponse(conn, proxyExec(root, req))
}

func writeResponse(w io.Writer, resp proxyResponse) {
	enc := json.NewEncoder(w)
	_ = enc.Encode(resp)
}

// execSelf runs the same binary with the requested arguments. Running the real
// CLI keeps a single implementation of every command; the child inherits the
// parent's TCC identity because it is the same code signature.
func execSelf(root string, req proxyRequest) proxyResponse {
	args := req.Args
	if !(len(args) >= 2 && args[0] == "--root") {
		args = append([]string{"--root", root}, args...)
	}
	if len(args) < 3 {
		return proxyResponse{ExitCode: 2, Error: "empty command"}
	}
	// Check the command after any --root prefix, or a caller could make the
	// proxy run itself.
	switch args[2] {
	case "serve", "client":
		return proxyResponse{ExitCode: 2, Error: "refusing to proxy serve or client"}
	}

	exe, err := os.Executable()
	if err != nil {
		return proxyResponse{ExitCode: exitTransport, Error: err.Error()}
	}

	cmd := exec.Command(exe, args...)
	cmd.Env = os.Environ()
	cmd.Stdin = strings.NewReader(req.Stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			return proxyResponse{ExitCode: exitTransport, Error: runErr.Error()}
		}
	}
	return proxyResponse{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code}
}

func cmdClient(root string, args []string) int {
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	socket := fs.String("socket", defaultSocketPath(root), "Unix socket path")
	timeout := fs.Float64("timeout", 0, "overall timeout in seconds (0 = none)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pair-to-gemini client [--socket <path>] [--timeout <sec>] -- <command> [args...]")
	}
	if stop, code := parseFlags(fs, args); stop {
		return code
	}
	target := fs.Args()
	if len(target) == 0 {
		fs.Usage()
		return 2
	}

	stdinText, err := readPipedStdin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pair-to-gemini client: read stdin: %v\n", err)
		return exitTransport
	}

	var deadline time.Duration
	if *timeout > 0 {
		deadline = time.Duration(*timeout * float64(time.Second))
	}
	resp, err := callProxy(*socket, proxyRequest{Args: target, Stdin: stdinText}, deadline)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pair-to-gemini client: %v\n", err)
		return exitTransport
	}

	if resp.Stdout != "" {
		_, _ = io.WriteString(os.Stdout, resp.Stdout)
	}
	if resp.Stderr != "" {
		_, _ = io.WriteString(os.Stderr, resp.Stderr)
	}
	if resp.Error != "" {
		fmt.Fprintf(os.Stderr, "pair-to-gemini client: %s\n", resp.Error)
	}
	if resp.ExitCode == 0 && resp.Error != "" {
		return exitTransport
	}
	return resp.ExitCode
}

func callProxy(socketPath string, req proxyRequest, timeout time.Duration) (proxyResponse, error) {
	var resp proxyResponse
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.Dial("unix", socketPath)
	if err != nil {
		return resp, err
	}
	defer conn.Close()
	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return resp, err
	}
	err = json.NewDecoder(conn).Decode(&resp)
	return resp, err
}

// readPipedStdin reads stdin only when it is not a terminal, so an interactive
// `client` call never waits for input it will not receive.
func readPipedStdin() (string, error) {
	st, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if st.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}
	b, err := io.ReadAll(io.LimitReader(os.Stdin, maxRequestBytes))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
