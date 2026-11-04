package integration

import (
	"context"
	"errors"
	"net/http"

	"github.com/flags-gg/orchestrator/internal/auth"
	"github.com/flags-gg/orchestrator/internal/flags"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ticketIn struct {
	Ticket string `json:"ticket" jsonschema:"ticket key the flag belongs to, e.g. TL-123"`
}
type agentIn struct {
	AgentId string `json:"agent_id" jsonschema:"agent id from list_agents"`
}
type createIn struct {
	Ticket  string `json:"ticket" jsonschema:"ticket key the flag belongs to, e.g. TL-123; any unique reference works"`
	AgentId string `json:"agent_id" jsonschema:"agent id from list_agents"`
	Name    string `json:"name" jsonschema:"flag name the code checks: letters, digits, space, - _ . (max 100)"`
}
type setIn struct {
	Ticket        string `json:"ticket" jsonschema:"ticket key the flag belongs to"`
	EnvironmentId string `json:"environment_id" jsonschema:"environment id the flag is in"`
	Enabled       bool   `json:"enabled" jsonschema:"true to turn the flag on"`
}
type promoteIn struct {
	Ticket       string `json:"ticket" jsonschema:"ticket key the flag belongs to"`
	ToProduction bool   `json:"to_production,omitempty" jsonschema:"promote through every remaining environment instead of one step"`
}
type agentsOut struct {
	Agents []AgentInfo `json:"agents"`
}
type archivedOut struct {
	Ticket   string `json:"ticket"`
	Archived bool   `json:"archived"`
}

func checkTicket(t string) error {
	if !validTicket(t) {
		return errors.New("ticket must be 1-255 characters with no spaces or slashes")
	}
	return nil
}

// mcpServer exposes the integration API as MCP tools for one company.
func (s *System) mcpServer(companyId string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "flags", Title: "Flags", Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: "Feature flags follow a ticket: create_flag puts it, off, in the agent's first environment; " +
			"set_flag toggles it in one environment; promote_flag copies it (off) to the next environment, or to production; " +
			"archive_flag removes it everywhere once the feature is decided. Use list_agents to find agent and environment ids.",
	})

	mcp.AddTool(srv, &mcp.Tool{Name: "list_agents", Description: "List projects, their agents and each agent's environments (lowest level first; production is the highest)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, agentsOut, error) {
			agents, err := s.ListAgents(ctx, companyId)
			return nil, agentsOut{Agents: agents}, err
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "list_flags", Description: "List an agent's flags with their state in each environment."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in agentIn) (*mcp.CallToolResult, *AgentFlags, error) {
			af, err := s.GetAgentFlags(ctx, companyId, in.AgentId)
			return nil, af, err
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "get_flag", Description: "Get a ticket's flag and its state in every environment."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ticketIn) (*mcp.CallToolResult, *TicketFlag, error) {
			if err := checkTicket(in.Ticket); err != nil {
				return nil, nil, err
			}
			tf, err := s.GetTicketFlag(ctx, companyId, in.Ticket)
			return nil, tf, err
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "create_flag", Description: "Create a ticket's flag, off, in the agent's first environment. Returns the existing flag if the ticket already has one."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in createIn) (*mcp.CallToolResult, *TicketFlag, error) {
			if err := checkTicket(in.Ticket); err != nil {
				return nil, nil, err
			}
			if in.AgentId == "" || !flags.ValidFlagName(in.Name) {
				return nil, nil, errors.New("agent_id is required and name must be 1-100 letters, digits, spaces, - _ .")
			}
			tf, _, err := s.CreateTicketFlag(ctx, companyId, in.Ticket, in.AgentId, in.Name)
			return nil, tf, err
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "set_flag", Description: "Turn a ticket's flag on or off in one environment it has been promoted to."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in setIn) (*mcp.CallToolResult, *TicketFlag, error) {
			if err := checkTicket(in.Ticket); err != nil {
				return nil, nil, err
			}
			if err := s.SetTicketFlag(ctx, companyId, in.Ticket, in.EnvironmentId, in.Enabled); err != nil {
				return nil, nil, err
			}
			tf, err := s.GetTicketFlag(ctx, companyId, in.Ticket)
			return nil, tf, err
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "promote_flag", Description: "Copy a ticket's flag, off, into the next environment (or every remaining one with to_production). No-op if already in production."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in promoteIn) (*mcp.CallToolResult, *TicketFlag, error) {
			if err := checkTicket(in.Ticket); err != nil {
				return nil, nil, err
			}
			tf, err := s.PromoteTicketFlag(ctx, companyId, in.Ticket, in.ToProduction)
			return nil, tf, err
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "archive_flag", Description: "Remove a ticket's flag from every environment (it is archived, not destroyed). Code must no longer depend on it."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ticketIn) (*mcp.CallToolResult, archivedOut, error) {
			if err := checkTicket(in.Ticket); err != nil {
				return nil, archivedOut{}, err
			}
			err := s.ArchiveTicketFlag(ctx, companyId, in.Ticket)
			return nil, archivedOut{Ticket: in.Ticket, Archived: err == nil}, err
		})
	return srv
}

// MCPHandler serves /mcp (streamable HTTP, stateless) for the integration token's company.
func (s *System) MCPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer(auth.CompanyID(r))
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
}
