package mcp

import (
	"context"
	"log"

	"github.com/bishopfox/sliver/protobuf/commonpb"
	"github.com/bishopfox/sliver/protobuf/rpcpb"
	mcpapi "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const (
	listSessionsAndPulsesToolName = "list_sessions_and_pulses"
)

type sessionSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Transport     string `json:"transport"`
	RemoteAddress string `json:"remote_address"`
	Hostname      string `json:"hostname"`
	Username      string `json:"username"`
	PID           int32  `json:"pid"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	Locale        string `json:"locale"`
	LastCheckin   int64  `json:"last_checkin"`
	IsDead        bool   `json:"is_dead"`
	Integrity     string `json:"integrity"`
}

type pulseSummary struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Transport           string `json:"transport"`
	RemoteAddress       string `json:"remote_address"`
	Hostname            string `json:"hostname"`
	Username            string `json:"username"`
	PID                 int32  `json:"pid"`
	OS                  string `json:"os"`
	Arch                string `json:"arch"`
	Locale              string `json:"locale"`
	LastCheckin         int64  `json:"last_checkin"`
	NextCheckin         int64  `json:"next_checkin"`
	Interval            int64  `json:"interval"`
	Jitter              int64  `json:"jitter"`
	TasksCount          int64  `json:"tasks_count"`
	TasksCountCompleted int64  `json:"tasks_count_completed"`
	IsDead              bool   `json:"is_dead"`
	Integrity           string `json:"integrity"`
}

type listSessionsAndPulsesResult struct {
	Sessions      []sessionSummary `json:"sessions"`
	SessionsCount int              `json:"sessions_count"`
	Pulses       []pulseSummary  `json:"pulses"`
	PulsesCount  int              `json:"pulses_count"`
}

// SliverMCPServer wraps the MCP server with Sliver RPC access for handlers.
type SliverMCPServer struct {
	Rpc    rpcpb.SliverRPCClient
	server *mcpserver.MCPServer
	logger *log.Logger
}

func newServer(cfg Config, rpc rpcpb.SliverRPCClient, logger *log.Logger) *SliverMCPServer {
	base := mcpserver.NewMCPServer(
		cfg.ServerName,
		cfg.ServerVersion,
		mcpserver.WithToolCapabilities(false),
	)

	listSessionsAndPulsesTool := mcpapi.NewTool(
		listSessionsAndPulsesToolName,
		mcpapi.WithDescription("List active sessions and pulses."),
		mcpapi.WithReadOnlyHintAnnotation(true),
		mcpapi.WithDestructiveHintAnnotation(false),
		mcpapi.WithIdempotentHintAnnotation(true),
	)
	lsTool := mcpapi.NewTool(
		lsToolName,
		mcpapi.WithDescription("List the contents of a remote directory."),
		mcpapi.WithInputSchema[lsArgs](),
		mcpapi.WithReadOnlyHintAnnotation(true),
		mcpapi.WithDestructiveHintAnnotation(false),
		mcpapi.WithIdempotentHintAnnotation(true),
	)
	cdTool := mcpapi.NewTool(
		cdToolName,
		mcpapi.WithDescription("Change the current directory on a remote session or pulse."),
		mcpapi.WithInputSchema[cdArgs](),
		mcpapi.WithReadOnlyHintAnnotation(false),
		mcpapi.WithDestructiveHintAnnotation(false),
	)
	catTool := mcpapi.NewTool(
		catToolName,
		mcpapi.WithDescription("Download and return the contents of a remote file."),
		mcpapi.WithInputSchema[catArgs](),
		mcpapi.WithReadOnlyHintAnnotation(true),
		mcpapi.WithDestructiveHintAnnotation(false),
		mcpapi.WithIdempotentHintAnnotation(true),
	)
	pwdTool := mcpapi.NewTool(
		pwdToolName,
		mcpapi.WithDescription("Return the current working directory of a remote session or pulse."),
		mcpapi.WithInputSchema[pwdArgs](),
		mcpapi.WithReadOnlyHintAnnotation(true),
		mcpapi.WithDestructiveHintAnnotation(false),
		mcpapi.WithIdempotentHintAnnotation(true),
	)
	rmTool := mcpapi.NewTool(
		rmToolName,
		mcpapi.WithDescription("Remove a file or directory on the remote target."),
		mcpapi.WithInputSchema[rmArgs](),
		mcpapi.WithReadOnlyHintAnnotation(false),
		mcpapi.WithDestructiveHintAnnotation(true),
	)
	mvTool := mcpapi.NewTool(
		mvToolName,
		mcpapi.WithDescription("Move or rename a file or directory on the remote target."),
		mcpapi.WithInputSchema[mvArgs](),
		mcpapi.WithReadOnlyHintAnnotation(false),
		mcpapi.WithDestructiveHintAnnotation(true),
	)
	cpTool := mcpapi.NewTool(
		cpToolName,
		mcpapi.WithDescription("Copy a file on the remote target."),
		mcpapi.WithInputSchema[cpArgs](),
		mcpapi.WithReadOnlyHintAnnotation(false),
		mcpapi.WithDestructiveHintAnnotation(true),
	)
	mkdirTool := mcpapi.NewTool(
		mkdirToolName,
		mcpapi.WithDescription("Create a directory on the remote target."),
		mcpapi.WithInputSchema[mkdirArgs](),
		mcpapi.WithReadOnlyHintAnnotation(false),
		mcpapi.WithDestructiveHintAnnotation(true),
	)
	chmodTool := mcpapi.NewTool(
		chmodToolName,
		mcpapi.WithDescription("Change file or directory permissions on the remote target."),
		mcpapi.WithInputSchema[chmodArgs](),
		mcpapi.WithReadOnlyHintAnnotation(false),
		mcpapi.WithDestructiveHintAnnotation(true),
	)
	chownTool := mcpapi.NewTool(
		chownToolName,
		mcpapi.WithDescription("Change file or directory ownership on the remote target."),
		mcpapi.WithInputSchema[chownArgs](),
		mcpapi.WithReadOnlyHintAnnotation(false),
		mcpapi.WithDestructiveHintAnnotation(true),
	)
	srv := &SliverMCPServer{
		Rpc:    rpc,
		server: base,
		logger: logger,
	}
	srv.server.AddTool(listSessionsAndPulsesTool, srv.listSessionsAndPulsesHandler)
	srv.server.AddTool(lsTool, srv.lsHandler)
	srv.server.AddTool(cdTool, srv.cdHandler)
	srv.server.AddTool(catTool, srv.catHandler)
	srv.server.AddTool(pwdTool, srv.pwdHandler)
	srv.server.AddTool(rmTool, srv.rmHandler)
	srv.server.AddTool(mvTool, srv.mvHandler)
	srv.server.AddTool(cpTool, srv.cpHandler)
	srv.server.AddTool(mkdirTool, srv.mkdirHandler)
	srv.server.AddTool(chmodTool, srv.chmodHandler)
	srv.server.AddTool(chownTool, srv.chownHandler)
	return srv
}

func (s *SliverMCPServer) listSessionsAndPulsesHandler(ctx context.Context, _ mcpapi.CallToolRequest) (*mcpapi.CallToolResult, error) {
	if s.Rpc == nil {
		return mcpapi.NewToolResultError("rpc client not configured"), nil
	}

	s.logToolCall(listSessionsAndPulsesToolName, "", "")

	sessionsResp, err := s.Rpc.GetSessions(ctx, &commonpb.Empty{})
	if err != nil {
		return mcpapi.NewToolResultErrorFromErr("failed to list sessions", err), nil
	}

	pulsesResp, err := s.Rpc.GetPulses(ctx, &commonpb.Empty{})
	if err != nil {
		return mcpapi.NewToolResultErrorFromErr("failed to list pulses", err), nil
	}

	result := listSessionsAndPulsesResult{
		Sessions: make([]sessionSummary, 0, len(sessionsResp.GetSessions())),
		Pulses:  make([]pulseSummary, 0, len(pulsesResp.GetPulses())),
	}

	for _, session := range sessionsResp.GetSessions() {
		if session == nil {
			continue
		}
		result.Sessions = append(result.Sessions, sessionSummary{
			ID:            session.ID,
			Name:          session.Name,
			Transport:     session.Transport,
			RemoteAddress: session.RemoteAddress,
			Hostname:      session.Hostname,
			Username:      session.Username,
			PID:           session.PID,
			OS:            session.OS,
			Arch:          session.Arch,
			Locale:        session.Locale,
			LastCheckin:   session.LastCheckin,
			IsDead:        session.IsDead,
			Integrity:     session.Integrity,
		})
	}

	for _, pulse := range pulsesResp.GetPulses() {
		if pulse == nil {
			continue
		}
		result.Pulses = append(result.Pulses, pulseSummary{
			ID:                  pulse.ID,
			Name:                pulse.Name,
			Transport:           pulse.Transport,
			RemoteAddress:       pulse.RemoteAddress,
			Hostname:            pulse.Hostname,
			Username:            pulse.Username,
			PID:                 pulse.PID,
			OS:                  pulse.OS,
			Arch:                pulse.Arch,
			Locale:              pulse.Locale,
			LastCheckin:         pulse.LastCheckin,
			NextCheckin:         pulse.NextCheckin,
			Interval:            pulse.Interval,
			Jitter:              pulse.Jitter,
			TasksCount:          pulse.TasksCount,
			TasksCountCompleted: pulse.TasksCountCompleted,
			IsDead:              pulse.IsDead,
			Integrity:           pulse.Integrity,
		})
	}

	result.SessionsCount = len(result.Sessions)
	result.PulsesCount = len(result.Pulses)

	return mcpapi.NewToolResultStructuredOnly(result), nil
}
