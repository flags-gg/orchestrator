# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the **Orchestrator** service for the flags.gg feature flag system. It serves as the central HTTP API that manages projects, agents, environments, flags, users, companies, and statistics. The service handles both library-based agent requests and frontend client requests.

## Development Commands

### Building & Running
```bash
go build -o bin/orchestrator ./cmd/orchestrator
./bin/orchestrator
```

### Testing
```bash
# Run all tests
go test ./...

# Run specific test
go test -v ./internal/pricing -run TestName

# Run tests with coverage
go test -v -cover ./...
```

The project uses testcontainers for integration tests (see `internal/pricing/pricing_test.go` for an example).

### Linting
```bash
go vet ./...
go fmt ./...
```

## Architecture

### Service Entry Point
- `cmd/orchestrator/service.go`: Main entry point that builds configuration and starts the service
- `internal/service.go`: HTTP server setup with all route definitions and middleware stack

### Configuration System
Uses `github.com/keloran/go-config` with environment-based configuration. The service requires:
- Database (PostgreSQL via pgx)
- Keycloak for authentication (with Clerk as alternative)
- InfluxDB for statistics
- Bugfixes logging
- Clerk for user management
- Resend for notifications
- Stripe for payments

Configuration is built with `ConfigBuilder.NewConfigNoVault()` and supports Railway deployment (checks `ON_RAILWAY` and `PORT` env vars).

### Module Structure
Each domain follows a consistent pattern with an `http.go` file for handlers and a data access file (`postgres.go`, `influx.go`, or `keycloak.go`):

- **agent**: Manages agents (SDK instances) that consume feature flags
- **company**: Company management, user associations, invitations, and upgrades
- **environment**: Environment management for agents (dev, staging, prod, etc.)
- **flags**: Core feature flag functionality
  - `agent.go`: Agent-facing flag retrieval (used by SDKs)
  - `client.go`: Client-facing flag retrieval (used by frontend)
  - `http.go`: HTTP handlers for CRUD operations
- **general**: Webhook handlers (Stripe, Keycloak events) and miscellaneous endpoints
- **pricing**: Pricing tiers and limits
- **project**: Top-level project management
- **secretmenu**: Secret menu feature with sequence, state, and style management
- **stats**: Statistics collection and retrieval (uses InfluxDB and PostgreSQL)
- **user**: User management, notifications, and profile updates

### Authentication Flow
Located in `internal/auth.go`. The `Auth` middleware:
1. First validates as a user via Clerk (`ValidateUser`)
2. Falls back to agent validation for SDK requests (`ValidateAgent`)
3. In development mode, bypasses all checks
4. Agent validation only applies to the `/flags` endpoint

### API Structure
The service uses Go 1.22+ HTTP routing with method prefixes:
- Projects: `/project/{projectId}`, `/projects`
- Agents: `/agent/{agentId}`, `/project/{projectId}/agents`
- Environments: `/environment/{environmentId}`, `/agent/{agentId}/environments`
- Flags:
  - `/flags` - Agent/SDK requests (authenticated via headers)
  - `/environment/{environmentId}/flags` - Client/frontend requests
  - `/flag/{flagId}` - CRUD operations
- Stats: `/stats/company`, `/stats/project/{projectId}`, `/stats/agent/{agentId}`
- Company: `/company`, `/company/users`, `/company/invite`

### Database Access Pattern
All modules use a consistent pattern:
1. Get pgx client from config: `s.Config.Database.GetPGXClient(s.Context)`
2. Defer close with error handling
3. Use parameterized queries with pgx
4. Handle `pgx.ErrNoRows` for missing data

### Key Headers
The service expects these headers for routing and authentication:
- `x-agent-id`: Agent identifier
- `x-project-id`: Project identifier
- `x-environment-id`: Environment identifier
- `x-company-id`: Company identifier
- `x-user-subject`: User subject from Clerk
- `x-flags-timestamp`: Timestamp for flag requests

### Middleware Stack
Applied in this order (see `internal/service.go:135-154`):
1. Logger (Error level)
2. RequestID
3. Recoverer
4. Auth (custom user/agent validation)
5. CORS (configurable origins based on environment)
6. LowerCaseHeaders

### Agent vs Client Flags
Two distinct flag retrieval paths:
- **Agent path** (`flags/agent.go`): Used by SDKs, includes interval control and secret menu configuration
- **Client path** (`flags/client.go`): Used by frontend applications, returns simpler flag list

### Statistics System
Dual storage approach:
- PostgreSQL: Structured data and relationships
- InfluxDB: Time-series metrics for high-volume stats

### Development Mode
When `Development: true` in config:
- All authentication checks are bypassed
- Additional CORS origins allowed (localhost:3000, localhost:5173)
- Bruno API client can be used without authentication

## Dependencies
- Go 1.24+
- PostgreSQL (via pgx/v5)
- InfluxDB v2
- Clerk for authentication
- Stripe for payments
- Resend for emails
- Keycloak support (legacy)
