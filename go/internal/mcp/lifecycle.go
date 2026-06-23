// SPDX-FileCopyrightText: GitHub, Inc.
// SPDX-License-Identifier: MIT

package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GitHubSecurityLab/seclab-taskflow-agent/go/internal/envutil"
)

// Tool is a backend-neutral description of an MCP tool with a namespace-
// prefixed name.
type Tool struct {
	Name        string
	Description string
	InputSchema any
}

// Server is a connected MCP server, exposing namespace-prefixed tools and an
// optional interactive confirmation gate.
type Server struct {
	Name      string
	Namespace string
	session   *mcpsdk.ClientSession
	confirms  []string
	headless  bool
	proc      *process
}

// Pool owns a set of connected MCP servers for the lifetime of a task.
type Pool struct {
	Servers []*Server
}

// ServerPrompts returns the per-server prompt strings for system-prompt
// construction.
func (p *Pool) ServerPrompts(params []ResolvedParams) []string {
	prompts := make([]string, 0, len(params))
	for _, rp := range params {
		if rp.ServerPrompt != "" {
			prompts = append(prompts, rp.ServerPrompt)
		}
	}
	return prompts
}

// Connect builds and connects a server pool from resolved parameters. On any
// failure, already-connected servers are closed before returning.
func Connect(ctx context.Context, params []ResolvedParams) (*Pool, error) {
	pool := &Pool{}
	for _, rp := range params {
		srv, err := connectOne(ctx, rp)
		if err != nil {
			pool.Close(ctx)
			return nil, fmt.Errorf("connecting toolbox %q: %w", rp.Toolbox, err)
		}
		pool.Servers = append(pool.Servers, srv)
	}
	return pool, nil
}

func connectOne(ctx context.Context, rp ResolvedParams) (*Server, error) {
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "seclab-taskflow-agent", Version: "go"}, nil)

	var transport mcpsdk.Transport
	var proc *process

	switch rp.Kind {
	case "stdio":
		cmd := exec.Command(rp.Command, rp.Args...)
		cmd.Env = mergeEnv(rp.Env)
		transport = &mcpsdk.CommandTransport{Command: cmd}

	case "streamable":
		if rp.Command != "" {
			p, err := startProcess(rp)
			if err != nil {
				return nil, err
			}
			proc = p
		}
		transport = &mcpsdk.StreamableClientTransport{
			Endpoint:   rp.URL,
			HTTPClient: httpClientWithHeaders(rp.Headers),
		}

	case "sse":
		return nil, fmt.Errorf("sse transport is not supported in the Go agent yet")

	default:
		return nil, fmt.Errorf("unsupported MCP transport %q", rp.Kind)
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		if proc != nil {
			proc.stop()
		}
		return nil, err
	}

	return &Server{
		Name:      rp.Toolbox,
		Namespace: CompressName(rp.Toolbox),
		session:   session,
		confirms:  rp.Confirm,
		headless:  false, // set by caller via SetHeadless
		proc:      proc,
	}, nil
}

// SetHeadless toggles confirmation prompting for every server in the pool.
func (p *Pool) SetHeadless(headless bool) {
	for _, s := range p.Servers {
		s.headless = headless
	}
}

// Close cleans up every server (and any local processes) in reverse order.
func (p *Pool) Close(ctx context.Context) {
	for i := len(p.Servers) - 1; i >= 0; i-- {
		p.Servers[i].close(ctx)
	}
	p.Servers = nil
}

func (s *Server) close(_ context.Context) {
	if s.session != nil {
		_ = s.session.Close()
	}
	if s.proc != nil {
		s.proc.stop()
	}
}

// ListTools returns the server's tools with namespace-prefixed names.
func (s *Server) ListTools(ctx context.Context) ([]Tool, error) {
	res, err := s.session.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		return nil, err
	}
	tools := make([]Tool, 0, len(res.Tools))
	for _, t := range res.Tools {
		tools = append(tools, Tool{
			Name:        addNamespace(s.Namespace, t.Name),
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return tools, nil
}

// HandlesTool reports whether a namespaced tool name belongs to this server.
func (s *Server) HandlesTool(namespacedName string) bool {
	return strings.HasPrefix(namespacedName, s.Namespace)
}

// BareToolName returns the tool name with this server's namespace prefix
// removed, for matching against blocked-tool lists.
func (s *Server) BareToolName(namespacedName string) string {
	return stripNamespace(s.Namespace, namespacedName)
}

// CallTool invokes a namespaced tool, stripping the prefix and optionally
// requesting interactive confirmation. It returns the concatenated text
// content of the result.
func (s *Server) CallTool(ctx context.Context, namespacedName string, args map[string]any) (string, error) {
	bare := stripNamespace(s.Namespace, namespacedName)
	if !s.headless && contains(s.confirms, bare) {
		if !confirmToolCall(bare, args) {
			return "Tool call not allowed.", nil
		}
	}
	res, err := s.session.CallTool(ctx, &mcpsdk.CallToolParams{Name: bare, Arguments: args})
	if err != nil {
		return "", err
	}
	return extractText(res), nil
}

func extractText(res *mcpsdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func mergeEnv(extra map[string]string) []string {
	env := envutil.FilteredEnviron()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// process manages a locally-launched streamable MCP server subprocess and a
// readiness probe on its URL. It ports the Python “StreamableMCPThread“.
type process struct {
	cmd *exec.Cmd
}

func startProcess(rp ResolvedParams) (*process, error) {
	exe, err := exec.LookPath(rp.Command)
	if err != nil {
		return nil, fmt.Errorf("could not resolve path to %s: %w", rp.Command, err)
	}
	cmd := exec.Command(exe, rp.Args...)
	cmd.Env = mergeEnv(rp.Env)
	// Drain output so the pipe buffer never blocks the child.
	if stdout, e := cmd.StdoutPipe(); e == nil {
		go drain(stdout)
	}
	if stderr, e := cmd.StderrPipe(); e == nil {
		go drain(stderr)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &process{cmd: cmd}
	if err := waitForURL(rp.URL, 30*time.Second); err != nil {
		p.stop()
		return nil, err
	}
	return p, nil
}

func (p *process) stop() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_, _ = p.cmd.Process.Wait()
	}
}

func drain(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		// Discard; the MCP server mirrors its own output to its logs.
	}
}

// waitForURL polls the host:port of rawURL until a TCP connection succeeds or
// the timeout elapses.
func waitForURL(rawURL string, timeout time.Duration) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	host := u.Host
	if host == "" {
		return fmt.Errorf("invalid streamable MCP url: %q", rawURL)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", host, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for streamable MCP server at %s", host)
}
