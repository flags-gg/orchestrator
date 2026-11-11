# Outbound flag webhooks

Company → Webhooks in the dashboard manages up to 20 HTTPS endpoints per company. Each endpoint subscribes to one or more events:

| Event | Trigger |
| --- | --- |
| `flag.created` | A flag is added to an environment, including promotion into a new environment. |
| `flag.state_changed` | Enabled state, rollout, targeting rules, or name changes. Identical updates emit nothing. |
| `flag.archived` | A flag is removed from an environment. Archiving across several environments produces an event for each. |

Subscriptions apply to the company's flags across projects and environments. They include dashboard, REST, integration and MCP changes. Parent environment/agent switches do not change the flag row and do not produce these events. Rule changes are signaled but rules are not included in the payload; read the current flag state when needed.

## Configure an endpoint

Open Company → Webhooks, enter a name and public HTTPS URL on port 443, then select events. Save the signing secret shown after creation: it is not returned by list or log requests. To replace a secret or URL, create a replacement endpoint, update the receiver, then delete the old endpoint. The secret must remain available to the delivery worker in the database; restrict database access and encrypt database storage/backups.

Pause stops new events and delivery attempts for that endpoint; resume also continues queued deliveries. Event subscription changes apply to future events. Deleting an endpoint cancels its queue and deletes its delivery history. An attempt already in progress may finish after pause/delete.

The API rejects private literal addresses and checks DNS immediately before each connection, including retries. It pins the connection to a checked public IP, uses no HTTP proxy, and does not follow redirects. A hostname resolving to any private or reserved address fails delivery.

## Request contract

Requests are `POST` with `Content-Type: application/json` and these headers:

- `X-Flags-Event-ID`: stable event UUID, shared by retries and by endpoints receiving the same event.
- `X-Flags-Timestamp`: Unix seconds for this attempt.
- `X-Flags-Signature`: `v1=` followed by a lowercase hexadecimal HMAC-SHA256.

The signing key is the complete secret, including its `whsec_` prefix. The signed bytes are the timestamp header, a literal dot, and the **exact raw HTTP request body**. Do not parse and re-encode the JSON before verification. Use constant-time comparison. Reject timestamps more than five minutes in the past or future and deduplicate the event UUID after successful processing. The timestamp and signature are refreshed on each retry.

Example body:

```json
{
  "id": "fd0b1908-7b7b-4295-9a90-b374484858f5",
  "type": "flag.state_changed",
  "created_at": "2026-11-11T10:00:00Z",
  "company_id": "company-public-id",
  "data": {
    "flag_id": "123",
    "name": "new-checkout",
    "ticket_ref": "FLA-104",
    "project_id": "project-public-id",
    "agent_id": "agent-public-id",
    "environment_id": "environment-public-id",
    "enabled": true,
    "rollout": 100
  }
}
```

`ticket_ref` is null for flags not linked to a ticket. IDs for project, agent, environment and company are public identifiers. `flag_id` is a string containing the numeric flag row ID. The payload has the changed row's state; archive events carry the state before removal. It describes the stored flag, not a particular user's evaluation.

Example Go signature verification (after limiting and reading the body):

```go
stamp := r.Header.Get("X-Flags-Timestamp")
seconds, err := strconv.ParseInt(stamp, 10, 64)
if err != nil || time.Since(time.Unix(seconds, 0)) > 5*time.Minute || time.Until(time.Unix(seconds, 0)) > 5*time.Minute {
    http.Error(w, "invalid timestamp", http.StatusUnauthorized)
    return
}
mac := hmac.New(sha256.New, []byte(secret))
mac.Write([]byte(stamp + "."))
mac.Write(rawBody)
expected := "v1=" + hex.EncodeToString(mac.Sum(nil))
if !hmac.Equal([]byte(expected), []byte(r.Header.Get("X-Flags-Signature"))) {
    http.Error(w, "invalid signature", http.StatusUnauthorized)
    return
}
```

## Delivery and retries

Return any 2xx status within ten seconds to acknowledge a delivery. Network failures, timeouts, redirects, and other status codes are retried. There are at most eight attempts per delivery cycle: the initial attempt, followed by delays of 1 minute, 5 minutes, 15 minutes, 1 hour, 4 hours, 12 hours and 24 hours. The worker polls every two seconds; queue load can add delay.

Queue rows commit in the same transaction as the flag change. Rollbacks send nothing. Workers resume after restarts; leases and row locks prevent multiple replicas from claiming the same live attempt. If a receiver processed a request before the worker crashed, it can receive that event again. Delivery is at least once within the retry budget and **ordering is not guaranteed**. For TrackLines badges, use the verified event to refresh the ticket's canonical flag state rather than applying old events blindly.

The dashboard log shows event UUID, payload, endpoint, status, attempt count, latest HTTP status/error, and next scheduled attempt. Failed deliveries on enabled endpoints can be queued again manually, restarting their eight-attempt budget with the same event UUID. Delivered and failed records are removed after 30 days; pending records remain until delivery, failure, or endpoint deletion. Response bodies are not stored.

## Company API

All configuration and log endpoints require a verified Clerk user session (`Authorization: Bearer <session JWT>`) and are scoped to that user's company. Integration tokens and SDK credentials cannot manage subscriptions.

| Method | Path | Request/result |
| --- | --- | --- |
| GET | `/company/webhooks` | `{ "endpoints": [...] }`; no signing secrets. |
| POST | `/company/webhooks` | `{ "name", "url", "event_types": [...] }`; 201 with `{ "id", "secret" }` once. |
| PATCH | `/company/webhooks/{endpointId}` | `{ "enabled": false }` and/or `{ "event_types": [...] }`; 204. |
| DELETE | `/company/webhooks/{endpointId}` | Delete endpoint and its delivery history; 204. |
| GET | `/company/webhook-deliveries` | `{ "deliveries": [...] }`, newest first, up to 50. Optional `endpoint=<UUID>` and `before=<delivery ID>` cursor. |
| POST | `/company/webhook-deliveries/{deliveryId}/retry` | Retry a failed delivery on an enabled endpoint; 204, otherwise 409. |

Production deployments may expose these routes beneath their API `/v1` prefix. Local Compose routes have no prefix. Apply migration `0012_outbound_webhooks` and deploy the API before deploying the dashboard Webhooks tab. The delivery worker starts with the API; no separate service or webhook environment variables are required.
