# Flags ⇄ Tracklines integration: implementation guide

This is for whoever implements the Tracklines side. The Flags side is built and tested (`orchestrator/internal/integration`).

## The lifecycle

A ticket can say it **needs a feature flag**. From then on the flag follows the ticket across its board:

| Ticket event (Tracklines) | Flags call | Result in Flags |
|---|---|---|
| Ticket created with "needs a flag" | *nothing* | Only Tracklines stores the intent |
| Ticket put on a board (leaves the backlog) | `PUT /integration/tickets/{key}/flag` | Flag created **off**, in the agent's first (lowest-level) environment, e.g. `dev` |
| Ticket in progress | none; the ticket UI shows toggles | Developer toggles it with `PATCH …/flag` |
| Ticket moved to a "testing" column | `POST …/flag/promote` | Copied, **off**, into the next environment, e.g. `staging` |
| Ticket moved to a "dev complete" column | `POST …/flag/promote?to=production` | Copied, **off**, into every remaining environment, up to production |
| Someone decides the outcome | `DELETE …/flag` | Removed from every environment and archived in Flags (not destroyed) |

**Tracklines owns the mapping** from board columns to actions. Flags has no idea what a column is. It only exposes create / toggle / promote / archive, keyed by ticket.

## Auth

- A Flags company admin creates an **integration token** in the Flags dashboard (`POST /company/integration-token`). It looks like `flg_<64 hex>`, is shown once, and can be revoked.
- Send it on every call as `Authorization: Bearer flg_…`.
- A token sees only its own company's agents and flags. Anything else returns **404**, which is indistinguishable from "missing".
- Store the token encrypted, per Tracklines project (or organisation). Never send it to the browser: Tracklines' server calls Flags.

Base URL: `https://api.flags.gg` (no `/v1` prefix). In local development it's `http://localhost:8080`.

## What Tracklines needs to store

**Per project (or organisation): Flags connection settings**
- `flags_token`: encrypted.
- `flags_agent_id`: the default Flags agent (the application) that this project's flags go into. Let the user pick it from `GET /integration/agents` (see below).

**Per board: a column → action map**
- Each column is one of: `none` | `promote` | `promote_to_production` | `archive`.
- Sensible defaults: Testing/QA → `promote`, Dev complete → `promote_to_production`, everything else → `none`.
- Leave `archive` off by default. Deciding a flag is a deliberate act (see the "Remove flag" action below).
- "Put on a board" isn't a column action. It's the moment the ticket leaves the backlog onto any board column.

**Per ticket**
- `needs_flag`: boolean, set at creation or later.
- `flag_name`: optional. Default it to the ticket key lower-cased (`tl-123`), or a slug the user picks. It's what the code checks, so let the user edit it **before** the flag is created. It must match `^[\p{L}\p{N} _.\-]{1,100}$`.
- `flag_agent_id`: optional override of the project default.

Flags already keys everything by the ticket key, so Tracklines doesn't need to store any Flags ids.

## Endpoints

All endpoints:
- send and receive `application/json`;
- key on `{key}`, the ticket key (e.g. `TL-123`): 1–255 characters, no spaces or `/`;
- are **idempotent**, so retry freely on network errors, 5xx and timeouts.

### `PUT /integration/tickets/{key}/flag`: create

```json
{ "agent_id": "8c1f…", "name": "tl-123" }
```

- **201**: created. **200**: the ticket already had a flag; the existing one is returned unchanged, even if the name differs.
- **404**: unknown agent, or the agent belongs to another company.
- **409** with `{"error": "..."}`: either the agent has no environments, or a flag with that name already exists in the first environment. Show the error on the ticket and let the user rename.
- **400**: bad name or body.

Both successful responses return the **TicketFlag** body:

```json
{
  "ticket": "TL-123",
  "name": "tl-123",
  "agent_id": "8c1f…",
  "environments": [
    { "environment_id": "env-dev",  "name": "dev",     "level": 0, "present": true,  "enabled": false, "production": false, "rollout": 100, "rules": [] },
    { "environment_id": "env-stg",  "name": "staging", "level": 1, "present": false, "enabled": false, "production": false, "rollout": 100, "rules": [] },
    { "environment_id": "env-prod", "name": "prod",    "level": 2, "present": false, "enabled": false, "production": true,  "rollout": 100, "rules": [] }
  ]
}
```

- `environments` lists **every** environment of the agent, lowest level first.
- `present` says whether the flag has reached that environment yet. `production` marks the last environment.
- `rollout` (0–100) and `rules` decide who gets the flag while it's `enabled`: the first matching rule serves its value, otherwise the user's bucket must be under `rollout`. `100` with no rules means everyone (a plain on/off flag). Environments the flag hasn't reached read as `100` and `[]`.

### `GET /integration/agents`: settings picker

- **200** with `{"agents": [{ "project_id", "project_name", "agent_id", "agent_name", "environments": [{ "environment_id", "name", "level", "production" }] }]}`.
- Use it right after the token is pasted, both to check the token works and to let the user pick the default agent.

### `GET /integration/tickets/{key}/flag`: state

- **200** with a TicketFlag. **404** if the ticket has no flag.
- Use it to render the ticket's flag panel.

### `PATCH /integration/tickets/{key}/flag`: toggle, rollout, rules

```json
{ "environment_id": "env-dev", "enabled": true }
{ "environment_id": "env-dev", "rollout": 25 }
{ "environment_id": "env-dev", "rules": [{ "attribute": "key", "operator": "in", "values": ["u_8812"], "serve": true }] }
```

- Send any of `enabled`, `rollout` and `rules` (at least one); the others stay as they are. `rules` replaces the environment's rules, and `[]` clears them.
- `rollout` is 0–100. A rule has an `attribute` from the SDK's context (`key` is the targeting key), an `operator` (`in`, `not_in`, `contains`, `starts_with`, `ends_with`), 1–1000 `values` (matched ignoring case) and `serve` (on or off). At most 50 rules.
- **200** with the updated TicketFlag. **400** with `{"error": "..."}` when the change is out of bounds.
- **404** if the ticket has no flag, or the flag isn't in that environment yet.
- Only offer toggles for environments where `present` is true.

### `POST /integration/tickets/{key}/flag/promote[?to=production]`: promote

- With no query, the flag is copied into the next environment after the deepest one it's in. With `?to=production`, it's copied into all remaining environments.
- The new copies always start **off**.
- Already in production is a no-op, so it still returns **200** with the TicketFlag.
- **404**: the ticket has no flag. **409**: a flag with the same name already exists in a target environment; nothing changes.

### `DELETE /integration/tickets/{key}/flag`: archive

- **204**, even if there was nothing to remove.
- Afterwards, `GET` returns 404, and a new `PUT` creates a fresh flag.

## Recommended Tracklines behaviour

1. **On ticket moved** (including backlog → board), look up the destination:
   - backlog → any board column, with `needs_flag` → `PUT` (create);
   - destination column action `promote` → `POST …/promote`;
   - `promote_to_production` → `POST …/promote?to=production`;
   - `archive` → `DELETE`.
   - If a promote returns 404 because the flag was never created (e.g. `needs_flag` was ticked after the ticket reached the board), `PUT` first, then retry the promote.
2. **Moving a ticket backwards** (e.g. Testing → In progress) does **nothing**. Flags never demotes. A flag being present in staging is harmless because it's off.
3. **Make the calls async with retries**, from a queue or job, so a Flags outage never blocks a board move. All calls are idempotent, so at-least-once delivery is fine.
4. **Ticket flag panel**, visible when `needs_flag`:
   - flag name (editable until created);
   - one row per environment from `GET`, with an on/off switch where `present`, and "not yet" otherwise;
   - a "Remove flag" action (`DELETE`) with a confirm: "Code must no longer check this flag."
   - Toggling calls `PATCH` and re-renders from the response.
5. **Unticking `needs_flag` after creation** should offer "Remove flag" rather than silently deleting it.
6. **Ticket deleted, or resolved as won't-do / duplicate** → `DELETE`.
7. **Show errors on the ticket** (an activity or comment entry). The 409 name clash is the one a user can fix.

## For agents (MCP)

Flags also serves an MCP server at `/mcp` (streamable HTTP, stateless), using the **same integration token**:

```json
{ "mcpServers": { "flags": { "type": "http", "url": "https://api.flags.gg/mcp",
  "headers": { "Authorization": "Bearer flg_…" } } } }
```

| Tool | What it does |
|---|---|
| `list_agents` | Lists projects, agents and environments, to find `agent_id` and `environment_id` |
| `list_flags` | Lists all of an agent's flags (including ones not tied to a ticket), with per-environment state |
| `get_flag` | Same as `GET` |
| `create_flag` | Same as `PUT` |
| `set_flag` | Same as `PATCH`: on/off, rollout and/or rules |
| `promote_flag` | Same as `POST …/promote`; `to_production: true` promotes to production |
| `archive_flag` | Same as `DELETE` |

An agent working a ticket can create its flag through the MCP server, and the board automation stays consistent because both paths are keyed by the ticket. A Tracklines agent should still prefer the board automation, so it doesn't double up.

## Flags → TrackLines callbacks

Company-scoped outbound webhooks report `flag.created`, `flag.state_changed`, and `flag.archived`. Configure an endpoint in Company → Webhooks, verify its timestamped HMAC signature, deduplicate by event UUID, then re-read the ticket’s canonical flag state. Events contain `ticket_ref`, agent and environment IDs. See [Outbound flag webhooks](outbound-webhooks.md) for the payload, signature verification, retry policy and delivery log. Until TrackLines has a receiver configured, continue refreshing with `GET` when the ticket panel opens.
