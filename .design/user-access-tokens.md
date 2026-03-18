# User Access Tokens

**Status:** Draft
**Date:** 2026-03-18
**Related:** [auth-overview](.design/hosted/auth/auth-overview.md), [server-auth-setup](.design/hosted/auth/server-auth-setup.md), [sciontool-auth](.design/hosted/auth/sciontool-auth.md)

## Problem Statement

Scion needs a mechanism for headless, non-interactive authentication in CI/CD pipelines and automation scenarios. While the existing `SCION_HUB_TOKEN` environment variable already supports bearer tokens, there is no user-facing way to create scoped, expirable tokens suitable for production automation. The existing API key system (`sk_live_*`) provides a foundation but lacks grove-scoping and action-level granularity.

Users need to:
- Create tokens scoped to a specific grove with limited action permissions
- Use tokens in CI/CD pipelines (e.g., dispatch agents, check status)
- Manage token lifecycle (create, list, revoke) via CLI and web UI
- Rotate tokens without disrupting other integrations

## Current State

### Existing Token Types

| Type | Format | Lifetime | Scoping | Use Case |
|------|--------|----------|---------|----------|
| User JWT | Hub-signed JWT | 15 min (web) / 30 days (CLI) | Full user access | Interactive sessions |
| Agent JWT | Hub-signed JWT with claims | 10 hours | `grove_id` + `AgentTokenScope` list | Agent-to-Hub communication |
| API Key | `sk_live_<base64>` | Configurable expiry | `[]string` scopes (unstructured) | Programmatic access (partially implemented) |
| Dev Token | `scion_dev_<hex>` | None | Full dev-user access | Local development |

### Existing Infrastructure

- **`SCION_HUB_TOKEN`**: Environment variable already consumed by CLI/hubclient for bearer auth
- **`APIKeyService`** (`pkg/hub/apikey.go`): Full CRUD, SHA-256 hashed storage, revocation, expiry
- **`APIKeyStore`** (`pkg/store/store.go`): SQLite-backed persistence for API keys
- **`UnifiedAuthMiddleware`** (`pkg/hub/auth.go`): Already dispatches on token type/prefix
- **`AuthzService`** (`pkg/hub/authz.go`): Policy-based authorization with actions (`create`, `read`, `update`, `delete`, `list`, `start`, `stop`, `message`, `attach`, `dispatch`, etc.) and resource types (`agent`, `grove`, `policy`, `group`)
- **Hub API endpoints**: `GET/POST /api/v1/auth/api-keys`, `DELETE /api/v1/auth/api-keys/{id}`
- **Web profile section**: Exists at `/profile` with nav for env vars, secrets, settings — but no token management UI yet

### Agent Token Scopes (for reference)

```
agent:status:update    agent:log:append     agent:token:refresh
grove:secret:read      grove:agent:create   grove:agent:lifecycle
grove:agent:notify     grove:gcp:token:<sa-id>
```

### AuthzService Actions (for reference)

```
create  read  update  delete  list  manage
start   stop  message attach  dispatch  stop_all
register  addMember  removeMember  verify
```

## Design

### Token Model: User Access Tokens (UATs)

User Access Tokens are opaque bearer tokens that carry grove-scoped, action-limited permissions. They are stored hashed (like API keys) and validated at the middleware layer.

#### Token Format

```
scion_pat_<base64url-encoded-random-32-bytes>
```

- **Prefix `scion_pat_`**: Distinguishes from API keys (`sk_live_`), dev tokens (`scion_dev_`), and JWTs. `pat` = Personal Access Token.
- **Body**: 32 bytes of cryptographic randomness, base64url-encoded (43 chars).
- **Full length**: ~53 characters.

The prefix enables the `UnifiedAuthMiddleware` to route to the correct validation path without ambiguity.

#### Storage Model

Extend or create a new table alongside `api_keys`:

```go
// UserAccessToken represents a scoped personal access token.
type UserAccessToken struct {
    ID          string     `json:"id"`                    // UUID
    UserID      string     `json:"userId"`                // FK to User.ID
    Name        string     `json:"name"`                  // User-provided label
    Prefix      string     `json:"prefix"`                // First N chars for identification
    KeyHash     string     `json:"-"`                     // SHA-256 hash (never exposed)

    // Scoping
    GroveID     string     `json:"groveId"`               // Required: grove this token is scoped to
    Scopes      []string   `json:"scopes"`                // Action scopes (see below)

    // Lifecycle
    Revoked     bool       `json:"revoked"`
    ExpiresAt   *time.Time `json:"expiresAt,omitempty"`   // Required for UATs
    LastUsed    *time.Time `json:"lastUsed,omitempty"`
    Created     time.Time  `json:"created"`
}
```

#### Capability Scopes

Scopes are defined as `resource:action` pairs, derived from the existing `AuthzService` action constants and resource types. For the initial implementation, scopes map to what the system can currently enforce:

| Scope | Permits | Typical CI/CD Use |
|-------|---------|-------------------|
| `grove:read` | Read grove metadata | Status dashboards |
| `agent:create` | Create agents in the scoped grove | Dispatch CI agents |
| `agent:read` | Read agent status/metadata | Monitor agent progress |
| `agent:list` | List agents in the scoped grove | Enumerate active agents |
| `agent:start` | Start/restart agents | Re-run failed agents |
| `agent:stop` | Stop agents | Cancel running agents |
| `agent:delete` | Delete agents | Cleanup after CI runs |
| `agent:message` | Send messages to agents | Provide input to agents |
| `agent:attach` | Attach to agent sessions | Interactive debugging |
| `agent:dispatch` | Dispatch agents (create + start) | Primary CI/CD action |

A convenience alias `agent:manage` grants `agent:create`, `agent:read`, `agent:list`, `agent:start`, `agent:stop`, `agent:delete`, `agent:dispatch`.

Scopes are validated at token creation time against a known allowlist. Unknown scopes are rejected.

#### Authentication Flow

```
Client (CI/CD)
    │
    │  Authorization: Bearer scion_pat_<token>
    │  ─── or ───
    │  SCION_HUB_TOKEN=scion_pat_<token>
    ▼
UnifiedAuthMiddleware
    │
    ├─ Detect prefix "scion_pat_"
    ├─ SHA-256 hash the token
    ├─ Look up hash in user_access_tokens table
    ├─ Check: not revoked, not expired
    ├─ Load user record
    ├─ Build UserIdentity with grove + scope constraints
    │
    ▼
Handler / AuthzService
    │
    ├─ Check: request grove matches token's grove_id
    ├─ Check: request action is in token's scopes
    ├─ Deny if either check fails
    │
    ▼
Response
```

The key difference from API keys: UATs produce a **scoped** `UserIdentity` that carries grove and action restrictions. The `AuthzService.CheckAccess` path is augmented to enforce these constraints before evaluating policies.

### API Endpoints

Build on the existing `/api/v1/auth/` namespace:

```
POST   /api/v1/auth/tokens            Create a new user access token
GET    /api/v1/auth/tokens            List user's access tokens
GET    /api/v1/auth/tokens/{id}       Get token details
DELETE /api/v1/auth/tokens/{id}       Delete (permanently remove) a token
POST   /api/v1/auth/tokens/{id}/revoke  Revoke a token (soft-delete)
```

All endpoints require user authentication (interactive session or existing valid token — but UATs cannot create other UATs).

#### Create Token Request

```json
{
  "name": "ci-deploy-token",
  "groveId": "grove-uuid-here",
  "scopes": ["agent:dispatch", "agent:read", "agent:stop"],
  "expiresAt": "2026-06-18T00:00:00Z"
}
```

#### Create Token Response

```json
{
  "token": "scion_pat_abc123...",
  "accessToken": {
    "id": "uuid",
    "name": "ci-deploy-token",
    "prefix": "scion_pat_abc1...",
    "groveId": "grove-uuid-here",
    "scopes": ["agent:dispatch", "agent:read", "agent:stop"],
    "expiresAt": "2026-06-18T00:00:00Z",
    "created": "2026-03-18T12:00:00Z"
  }
}
```

The plaintext `token` value is shown **only once** in the create response.

### CLI Commands

```
scion hub token create --grove <grove> --name <name> --scopes <scope,...> [--expires <duration|date>]
scion hub token list [--grove <grove>]
scion hub token revoke <token-id>
scion hub token delete <token-id>
```

#### Examples

```bash
# Create a token for CI that can dispatch and monitor agents, expires in 90 days
scion hub token create \
  --grove my-project \
  --name "github-actions" \
  --scopes agent:dispatch,agent:read,agent:stop \
  --expires 90d

# List tokens
scion hub token list

# Revoke a token
scion hub token revoke tok_abc123

# Use the token in CI
export SCION_HUB_TOKEN=scion_pat_...
scion hub agent dispatch --grove my-project --template default --task "Run tests"
```

### Web UI

Add a **"Access Tokens"** page to the profile section:

- **Route**: `/profile/tokens`
- **Navigation**: Add "Access Tokens" entry to `scion-profile-nav` under the Configuration section
- **Components**:
  - Token list table: name, prefix, grove, scopes, created, last used, expires, revoke button
  - Create token dialog: name input, grove selector, scope checkboxes, expiry date picker
  - One-time token display modal after creation (with copy button)

### Implementation Plan

#### Phase 1: Backend (Store + Service + Auth)

1. Add `UserAccessToken` model to `pkg/store/models.go`
2. Add `UserAccessTokenStore` interface to `pkg/store/store.go`
3. Implement SQLite storage in `pkg/store/sqlite/`
4. Create `UserAccessTokenService` in `pkg/hub/useraccesstoken.go`
   - `CreateToken(ctx, userID, name, groveID, scopes, expiresAt)` → (plaintext, *UserAccessToken, error)
   - `ValidateToken(ctx, token)` → (ScopedUserIdentity, error)
   - `ListTokens(ctx, userID)` → ([]UserAccessToken, error)
   - `RevokeToken(ctx, userID, tokenID)` → error
   - `DeleteToken(ctx, userID, tokenID)` → error
5. Extend `UnifiedAuthMiddleware` to detect `scion_pat_` prefix and validate via the new service
6. Introduce `ScopedUserIdentity` that wraps `UserIdentity` with grove/scope restrictions
7. Augment `AuthzService.CheckAccess` to enforce grove + scope constraints when identity is scoped
8. Register API handlers on the server router

#### Phase 2: CLI Commands

1. Add `cmd/hub_token.go` with `scion hub token {create,list,revoke,delete}` subcommands
2. Add hubclient methods for token CRUD

#### Phase 3: Web UI

1. Add profile token list page component
2. Add create token dialog component
3. Add to profile navigation

## Alternatives Considered

### A. Reuse Existing API Keys (`sk_live_*`)

**Approach**: Add `groveId` field to the existing `APIKey` model and enforce grove-scoping in the validation path.

**Pros**:
- Less new code; reuses existing storage, service, and endpoints.
- Already wired into `UnifiedAuthMiddleware`.

**Cons**:
- API keys were designed as general-purpose credentials without grove scoping. Retrofitting adds conditional logic throughout the validation path.
- The `sk_live_` prefix conveys a different semantic (Stripe-style API key) than a scoped access token.
- Existing API key consumers (if any) would need migration or compatibility handling.
- Scopes on API keys are currently unstructured `[]string` with no validation or enforcement.

**Verdict**: Rejected. The semantic mismatch and lack of grove-scoping in the existing model make this fragile. A clean model is worth the modest additional code.

### B. Mint Long-Lived User JWTs

**Approach**: Generate long-lived JWTs (like the CLI token type, 30-day) with grove and scope claims baked in.

**Pros**:
- Stateless validation (no DB lookup per request).
- Reuses existing `UserTokenService` and JWT infrastructure.
- Already handled by `UnifiedAuthMiddleware`.

**Cons**:
- **Irrevocable**: JWTs cannot be revoked without maintaining a blacklist, which negates the stateless advantage.
- **Size**: JWTs are ~500+ chars vs ~53 chars for opaque tokens, awkward in env vars.
- **Rotation**: Cannot update scopes on an existing token; must issue a new one.
- **Leak impact**: A leaked JWT is valid until expiry with no way to invalidate.

**Verdict**: Rejected. Revocability is a hard requirement for tokens used in CI/CD systems where secrets may be accidentally exposed in logs.

### C. Extend Agent Token System for Users

**Approach**: Use the `AgentTokenService` to mint JWTs for human users in automation contexts, reusing `AgentTokenClaims` with its scope system.

**Pros**:
- Agent tokens already have well-defined scopes and grove binding.
- Minimal new token infrastructure.

**Cons**:
- Conflates agent and user identity. Handlers that check `GetAgentFromContext()` vs `GetUserIdentityFromContext()` would need rework.
- Agent scopes (`agent:status:update`, `grove:agent:create`) are from the agent's perspective, not the user's.
- Same JWT irrevocability problem as alternative B.
- Agent tokens are designed to be provisioned by the system, not self-service by users.

**Verdict**: Rejected. Identity type confusion would create subtle authorization bugs.

### D. OAuth Client Credentials Grant

**Approach**: Implement the OAuth 2.0 Client Credentials flow. Users register a client (client_id + client_secret), then exchange credentials for short-lived access tokens.

**Pros**:
- Standards-based, well-understood by CI/CD systems.
- Short-lived tokens reduce leak exposure.
- Revocation is simple (revoke the client registration).

**Cons**:
- Two-step flow: CI must first exchange credentials, then use the token. Adds latency and complexity to pipelines.
- Requires client registration management (another CRUD surface).
- Over-engineered for the current use case where a simple bearer token suffices.

**Verdict**: Deferred. Could be a future enhancement for enterprises, but the PAT model is simpler and sufficient for the initial use case.

## Open Questions

1. **Maximum token count per user**: Should we limit the number of active tokens per user? Suggested: 25 per user, configurable.

2. **Maximum expiry duration**: Should we cap how far out `expiresAt` can be? GitHub caps at 1 year for fine-grained tokens. Suggested: 1 year max, with a default of 90 days if not specified.

3. **Token inheritance from API keys**: Should the existing `sk_live_` API keys be deprecated in favor of UATs, or should both coexist? API keys currently have no grove-scoping and no scope enforcement. Recommendation: keep both but mark API keys as "legacy" in docs; new integrations should use UATs.

4. **Cross-grove tokens**: Some automation may need to operate across multiple groves. Should we support multiple `groveId` values, or require one token per grove? Suggested: start with single-grove scoping, add multi-grove later if needed.

5. **Audit logging**: Should token usage be logged beyond `lastUsed` timestamp? CI/CD tokens are high-value audit targets. Suggested: log token prefix + action + grove for each authenticated request, defer full audit trail to a later phase.

6. **UAT-creates-UAT prevention**: The design states UATs cannot create other UATs. Should this be enforced at the scope level (no `token:create` scope) or at the middleware level (reject `scion_pat_*` auth on token endpoints)? Suggested: middleware-level check is simpler and more robust.

7. **Scope granularity evolution**: The current scope list is derived from `AuthzService` actions. As the authorization system matures (fine-grained policies, resource-level ACLs), how should token scopes evolve? Suggested: scopes remain coarse capability gates; the policy engine handles fine-grained decisions within those gates.
