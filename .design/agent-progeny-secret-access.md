# Agent Progeny Secret Access

## Status
**Proposed** — 2026-04-05

## Related Documents
- [Agent-to-Hub Access](hosted/agent-hub-access.md) — Sub-agent creation and management
- [Agent Credential Resolution](agent-credentials.md) — Auth pipeline for agent containers
- [Access & Visibility](access-visibility.md) — Visibility and implicit policy scaffolding
- [Permissions Design](hosted/auth/permissions-design.md) — RBAC policy model
- [Groups Design](hosted/auth/groups-design.md) — Principal model and group types

---

## 1. Problem Statement

When a user sets a secret or environment variable at the `user` or `grove` scope, that secret is resolved and injected into every agent the user creates. However, when an agent spawns a sub-agent (progeny), the secret resolution pipeline runs with the **parent agent's identity**, not the original user's identity. Because agents are not users, they have no `user`-scoped secrets, and the resulting progeny may lack credentials the user intended all their agents to have.

Today, there is no way for a user to express: *"This secret should be available not only to agents I create directly, but also to agents my agents create."*

### Use Cases

1. **API keys for sub-agents**: A user stores `ANTHROPIC_API_KEY` at user scope. They create a lead agent that spawns worker sub-agents. The workers need the same API key but don't receive it because they were created by an agent, not the user.

2. **Shared service credentials**: A grove has a `DATABASE_URL` secret. Sub-agents spawned by other agents within the grove should inherit access to it without requiring manual per-agent configuration.

3. **Scoped tool tokens**: A user sets a GitHub PAT as a secret. They want their agents and any sub-agents to be able to use it for code operations, but not agents created by other users.

### Goals

1. Allow users to opt-in individual secrets/env vars for progeny access at creation or edit time.
2. Ensure progeny access flows through the existing policy engine — no special-case bypass logic.
3. Include ancestry data in agent token claims so the secret resolution pipeline can verify lineage.
4. Support both `user`-scoped and `grove`-scoped secrets.
5. Maintain the principle of least privilege — progeny access is opt-in, not default.

### Non-Goals

- Cross-grove secret sharing (agents in grove A accessing secrets from grove B).
- Automatic secret inheritance without explicit opt-in.
- Recursive depth limits on progeny access (all descendants or none; depth limits can be added later).
- Secret value caching or forwarding through the parent agent (progeny always resolve from the backend).

---

## 2. Current State

### 2.1 Secret Resolution at Dispatch

When the Hub dispatches an agent, it calls `secretBackend.Resolve(userID, groveID, brokerID)` to collect secrets from all three scopes. The resolution merges in priority order: `user < grove < runtime_broker`.

For **user-created agents**, `userID` is the creating user's ID — so user-scoped secrets are included.

For **agent-created sub-agents**, the creating principal is an agent, not a user. The dispatch flow does not currently propagate the originating user's ID into the secret resolution call. As a result, user-scoped secrets are invisible to progeny.

### 2.2 Agent Ancestry

Ancestry is already tracked and stored on every agent (`Agent.Ancestry []string`):

```
User (alice-123) creates Agent A
  → A.Ancestry = ["alice-123"]

Agent A creates Agent B
  → B.Ancestry = ["alice-123", "agent-a-id"]

Agent B creates Agent C
  → C.Ancestry = ["alice-123", "agent-a-id", "agent-b-id"]
```

The ancestry chain always begins with the originating user's ID. This data is computed at creation time and is immutable.

### 2.3 Agent Token Claims

The current `AgentTokenClaims` struct contains:
- `Subject` (agent ID)
- `GroveID`
- `Scopes` (list of `AgentTokenScope` values)

It does **not** include ancestry information. This means when an agent calls the Hub API (e.g., to create a sub-agent), the Hub can identify the calling agent but cannot determine its lineage from the token alone — it must look up the agent record.

### 2.4 Policy Engine

The policy engine (`AuthzService.CheckAccess`) already supports ancestry-based access via `canAccessAsAncestor()`. However, this is used for agent-to-agent resource access (e.g., a parent reading a child's status), not for secret resolution. Secrets bypass the policy engine entirely — they are resolved by scope matching in `SecretBackend.Resolve()`.

---

## 3. Design

### 3.1 New Secret Metadata: `AllowProgeny`

Add a boolean field `AllowProgeny` to the secret and env var models to let users opt-in to progeny access.

#### Secret Model Changes

**`pkg/secret/secret.go`**:
```go
type SecretMeta struct {
    // ... existing fields ...
    AllowProgeny bool `json:"allowProgeny,omitempty"` // Allow creator's progeny agents to access
}

type SetSecretInput struct {
    // ... existing fields ...
    AllowProgeny bool // Allow creator's progeny agents to access
}
```

**`pkg/store/models.go`**:
```go
type Secret struct {
    // ... existing fields ...
    AllowProgeny bool `json:"allowProgeny,omitempty"` // Progeny access opt-in
}

type EnvVar struct {
    // ... existing fields ...
    AllowProgeny bool `json:"allowProgeny,omitempty"` // Progeny access opt-in
}
```

#### API Changes

**`PUT /api/v1/secrets/{key}`** — Add `allowProgeny` field to request body:

```json
{
  "value": "sk-...",
  "scope": "user",
  "scopeId": "alice-123",
  "type": "environment",
  "allowProgeny": true
}
```

**`PUT /api/v1/env/{key}`** — Same addition:

```json
{
  "key": "MY_TOKEN",
  "value": "...",
  "scope": "grove",
  "scopeId": "grove-456",
  "allowProgeny": true
}
```

**`GET` responses** — Include `allowProgeny` in metadata responses so the UI and CLI can display and edit the flag.

#### Database Schema

Add `allow_progeny BOOLEAN NOT NULL DEFAULT FALSE` column to both the `secrets` and `env_vars` tables (Ent schema or SQLite migration).

### 3.2 Ancestry Claims in Agent Tokens

Add the agent's ancestry chain to the JWT claims so that the Hub can verify lineage without a database lookup during secret resolution.

**`pkg/hub/agenttoken.go`**:
```go
type AgentTokenClaims struct {
    jwt.Claims
    GroveID  string            `json:"grove_id,omitempty"`
    Scopes   []AgentTokenScope `json:"scopes,omitempty"`
    Ancestry []string          `json:"ancestry,omitempty"` // [root_user, ..., parent_agent]
}
```

**Token generation** — When generating tokens during dispatch, include the agent's ancestry from the agent record:

```go
claims := AgentTokenClaims{
    Claims:   jwt.Claims{...},
    GroveID:  groveID,
    Scopes:   scopes,
    Ancestry: agent.Ancestry, // Already computed and stored at creation time
}
```

**`AgentIdentity` interface** — Add an `Ancestry() []string` method so handlers and the policy engine can access lineage from the authenticated context without a store lookup:

```go
type AgentIdentity interface {
    Identity
    GroveID() string
    HasScope(AgentTokenScope) bool
    Ancestry() []string       // NEW: ordered ancestor chain
    OriginUserID() string     // NEW: convenience — returns Ancestry[0] if present
}
```

#### Token Size Considerations

Ancestry chains are typically short (3–5 entries, each a UUID). Even a deep chain of 20 ancestors adds ~720 bytes to the token — well within JWT size limits. No truncation or compression is needed.

### 3.3 Policy-Based Progeny Secret Access

Rather than adding progeny logic directly into `SecretBackend.Resolve()`, this design routes progeny access through the existing policy engine. When a secret is marked `allowProgeny: true`, the system creates an **implicit policy** that grants read access to agents whose ancestry includes the secret's creator.

#### 3.3.1 Implicit Policy Generation

When a secret is created or updated with `allowProgeny: true`, the system generates an implicit (system-managed) policy:

```go
// Conceptual policy — may be materialized in the DB or evaluated inline
Policy{
    Name:         "progeny-secret-access:<secret-id>",
    ScopeType:    secret.Scope,           // "user" or "grove"
    ScopeID:      secret.ScopeID,
    ResourceType: "secret",
    ResourceID:   secret.ID,
    Actions:      []string{"read"},
    Effect:       "allow",
    Conditions: PolicyConditions{
        DelegatedFrom: &DelegatedFromCondition{
            PrincipalType: "user",
            PrincipalID:   secret.CreatedBy,
        },
    },
    Labels: map[string]string{
        "scion.dev/managed-by": "progeny-secret-access",
        "scion.dev/secret-key": secret.Name,
    },
    Priority: 0,
}
```

This policy says: *"Any principal whose creation was delegated from user X may read this secret."*

#### 3.3.2 Evaluation Flow

The existing `DelegatedFrom` condition in the policy engine already supports matching a principal against a delegation chain. The ancestry claim in the agent token provides exactly this chain. The evaluation path is:

1. Agent token is validated → `AgentIdentity` is populated with `Ancestry`.
2. Secret resolution calls `authzService.CheckAccess()` for each `allowProgeny` secret.
3. Policy engine finds the implicit progeny policy for the secret.
4. `DelegatedFrom` condition checks if `secret.CreatedBy` appears in `agent.Ancestry()`.
5. If matched → secret is included in the resolution result.

#### 3.3.3 Materialized vs. Inline Evaluation

Two implementation strategies:

| Strategy | Pros | Cons |
|----------|------|------|
| **Materialized policies** — Write actual policy records to the DB when `allowProgeny` is toggled | Standard policy evaluation path; visible in policy listings; auditable | Extra DB writes; must keep policies in sync with secret lifecycle (delete secret → delete policy) |
| **Inline evaluation** — Evaluate progeny access as a virtual policy during secret resolution | No extra DB records; no sync concerns | Less visible; bypasses standard policy listing |

**Recommendation**: Use **materialized policies** for Phase 1. The overhead is minimal (one policy per progeny-enabled secret), and the benefits of auditability and consistency with the existing policy model outweigh the sync cost. The policy should be labeled with `scion.dev/managed-by: progeny-secret-access` so it can be identified as system-managed.

#### 3.3.4 Policy Lifecycle

| Secret Event | Policy Action |
|-------------|--------------|
| Create with `allowProgeny: true` | Create implicit policy |
| Update: set `allowProgeny: true` | Create implicit policy (if not exists) |
| Update: set `allowProgeny: false` | Delete implicit policy |
| Delete secret | Delete implicit policy |
| Update `createdBy` (ownership transfer) | Update policy's `DelegatedFrom.PrincipalID` |

### 3.4 Secret Resolution Changes

The `SecretBackend.Resolve()` method needs a new code path for progeny secrets. Currently it resolves secrets by scope matching only. The updated flow:

```
Resolve(ctx, userID, groveID, brokerID) → []SecretWithValue

Current:
  1. Query secrets WHERE scope=user AND scopeID=userID
  2. Query secrets WHERE scope=grove AND scopeID=groveID
  3. Query secrets WHERE scope=runtime_broker AND scopeID=brokerID
  4. Merge (later scope overrides earlier)

Updated (when caller is an agent with ancestry):
  1. Query secrets WHERE scope=grove AND scopeID=groveID  (unchanged)
  2. Query secrets WHERE scope=runtime_broker AND scopeID=brokerID  (unchanged)
  3. Query secrets WHERE allowProgeny=true AND createdBy IN agent.Ancestry
     → For each candidate, verify access via policy engine
  4. Merge all results (scope priority still applies; progeny secrets
     have the same priority as their original scope)
```

**New `Resolve` signature** (adds optional ancestry context):

```go
// ResolveOpts provides additional context for secret resolution.
type ResolveOpts struct {
    // AgentAncestry is the ordered ancestor chain from the agent's token.
    // When present, secrets marked allowProgeny whose creator appears
    // in this chain are included in the result.
    AgentAncestry []string
}

// Resolve collects and merges secrets for an agent.
// The opts parameter is optional; pass nil for the current behavior.
func (b *Backend) Resolve(ctx context.Context, userID, groveID, brokerID string, opts *ResolveOpts) ([]SecretWithValue, error)
```

### 3.5 Env Var Resolution Changes

Environment variables follow the same pattern as secrets. The `resolveEnvSecretAccess()` function in `pkg/hub/handlers.go` should apply the same progeny logic when resolving env vars for dispatch:

1. Standard scope-based resolution (unchanged).
2. If the requesting agent has ancestry, query env vars with `allowProgeny=true` whose `createdBy` is in the ancestry chain.
3. Verify access via policy engine.
4. Merge into the result set.

---

## 4. User Experience

### 4.1 CLI

**Setting a secret with progeny access:**
```bash
scion secret set ANTHROPIC_API_KEY --scope user --allow-progeny
# Enter secret value: ****

scion env set LOG_LEVEL=debug --scope grove --allow-progeny
```

**Viewing progeny flag:**
```bash
scion secret list --scope user
NAME                 TYPE          SCOPE   PROGENY   UPDATED
ANTHROPIC_API_KEY    environment   user    ✓         2026-04-05
DATABASE_URL         environment   grove   -         2026-04-01
```

**Toggling progeny access on existing secret:**
```bash
scion secret update ANTHROPIC_API_KEY --scope user --allow-progeny=false
```

### 4.2 Web UI

- Add a toggle/checkbox labeled **"Allow agent progeny to access"** on the secret creation and edit forms.
- Show a progeny indicator icon/badge in the secrets list view.
- Tooltip: *"When enabled, agents spawned by your agents (and their descendants) will also receive this secret."*

### 4.3 API

All existing secret and env var endpoints accept and return the `allowProgeny` field. No new endpoints are needed.

---

## 5. Security Considerations

### 5.1 Opt-In Only

Progeny access is **never default**. Users must explicitly enable it per secret. This prevents accidental credential leakage to unexpected sub-agents.

### 5.2 Ancestry Verification

The ancestry chain in the agent token is set at token generation time from the immutable `Agent.Ancestry` field. It cannot be modified by the agent. An agent cannot forge ancestry to gain access to secrets it shouldn't have.

### 5.3 Scope Boundaries

Progeny access does not cross grove boundaries. A user-scoped secret marked `allowProgeny` is only available to progeny agents within the same grove as the creating user's agent lineage. The grove ID in the agent token provides this boundary.

### 5.4 Creator-Scoped, Not User-Scoped

The progeny policy is bound to the **secret's creator** (`createdBy`), not to all users. If Alice creates a secret with `allowProgeny`, only agents in Alice's ancestry chain can access it — not agents created by Bob, even if Bob is in the same grove.

### 5.5 Policy Override

Because progeny access is implemented via the policy engine, explicit `deny` policies take precedence. An administrator can create a deny policy that blocks a specific agent or group from accessing progeny secrets, overriding the implicit allow.

### 5.6 Audit Trail

All progeny secret access decisions flow through `CheckAccess()`, which emits authorization decision logs. The materialized policy approach ensures that progeny grants appear in policy listings and can be reviewed.

### 5.7 Token Revocation

If a parent agent is stopped or deleted, its progeny agents retain their own tokens with their own ancestry. This is by design — ancestry is a historical fact, not a live permission. To revoke progeny access, the user should set `allowProgeny: false` on the secret, which deletes the implicit policy.

---

## 6. Implementation Plan

### Phase 1: Data Model and Storage

1. Add `AllowProgeny` field to `SecretMeta`, `SetSecretInput`, `Secret`, and `EnvVar` models.
2. Add `allow_progeny` column to secrets and env_vars tables (Ent schema migration).
3. Update secret backend `Set()` to persist the flag.
4. Update secret backend `List()` and `GetMeta()` to return the flag.
5. Update API handlers for `PUT /api/v1/secrets/{key}` and `PUT /api/v1/env/{key}` to accept `allowProgeny`.
6. Update CLI `secret set` and `env set` commands to accept `--allow-progeny` flag.

### Phase 2: Ancestry in Token Claims

7. Add `Ancestry` field to `AgentTokenClaims`.
8. Update `GenerateAgentToken` call sites in dispatcher to include agent ancestry.
9. Update `AgentIdentity` interface to expose `Ancestry()` and `OriginUserID()`.
10. Update agent auth middleware to populate ancestry from validated token claims.

### Phase 3: Policy Integration

11. Implement implicit policy creation/deletion when `allowProgeny` is toggled on a secret.
12. Label implicit policies with `scion.dev/managed-by: progeny-secret-access` for identification.
13. Ensure policy cleanup on secret deletion.
14. Add `DelegatedFrom` condition matching against agent ancestry in policy evaluation (verify existing support is sufficient).

### Phase 4: Secret Resolution

15. Add `ResolveOpts` parameter to `SecretBackend.Resolve()`.
16. Update dispatch flow to pass agent ancestry into `Resolve()` when the creating principal is an agent.
17. Implement progeny secret query: `WHERE allowProgeny=true AND createdBy IN ancestry`.
18. Verify via policy engine before including each progeny secret.
19. Apply same logic to env var resolution.

### Phase 5: UX and Testing

20. Add progeny column to CLI `secret list` and `env list` output.
21. Add toggle to web UI secret/env var forms.
22. Integration tests: user creates secret with `allowProgeny`, agent creates sub-agent, verify sub-agent receives the secret.
23. Negative tests: verify progeny access denied when flag is false, when ancestry doesn't match, when deny policy exists.

---

## 7. Files Affected

| File | Change |
|------|--------|
| `pkg/secret/secret.go` | Add `AllowProgeny` to `SecretMeta` and `SetSecretInput` |
| `pkg/secret/localbackend.go` | Persist and query `AllowProgeny` |
| `pkg/secret/gcpbackend.go` | Persist `AllowProgeny` in metadata labels |
| `pkg/store/models.go` | Add `AllowProgeny` to `Secret` and `EnvVar` models |
| `pkg/ent/schema/secret.go` | Add `allow_progeny` field |
| `pkg/ent/schema/envvar.go` | Add `allow_progeny` field (if Ent-managed) |
| `pkg/hub/agenttoken.go` | Add `Ancestry` to `AgentTokenClaims`; add `OriginUserID()` |
| `pkg/hub/handlers.go` | Accept `allowProgeny` in secret/env PUT handlers; policy lifecycle on toggle |
| `pkg/hub/httpdispatcher.go` | Pass ancestry to `GenerateAgentToken`; pass ancestry to `Resolve()` |
| `pkg/hub/authz.go` | Verify `DelegatedFrom` works with ancestry claims (may need no changes) |
| `pkg/api/types.go` | Add `AllowProgeny` to API request/response types |
| `pkg/hubclient/secrets.go` | Add `AllowProgeny` to client SDK types |
| `pkg/hubclient/env.go` | Add `AllowProgeny` to client SDK types |
| `cmd/hub_secret.go` | Add `--allow-progeny` flag to `secret set` and `secret update` |
| `cmd/hub_env.go` | Add `--allow-progeny` flag to `env set` |
| `web/src/client/...` | Progeny toggle on secret/env forms; badge in list views |

---

## 8. Resolved Design Decisions

### 8.1 Opt-In vs. Opt-Out

**Decision**: Opt-in (default `false`).

**Rationale**: Secrets are sensitive. Expanding access should require explicit intent. Users who want all their secrets available to progeny can enable the flag on each secret individually. A future "bulk enable" or grove-level default could reduce friction but should not be the initial behavior.

### 8.2 Materialized vs. Inline Policies

**Decision**: Materialized policies.

**Rationale**: Materialized policies are visible in `policy list`, appear in audit logs, and can be overridden by explicit deny policies using standard policy resolution. The sync cost (create/delete policy when flag toggles) is low and can be handled transactionally with the secret write.

### 8.3 Ancestry in Token vs. Store Lookup

**Decision**: Include ancestry in the token.

**Rationale**: The ancestry chain is immutable after agent creation and is typically short. Including it in the token avoids a database round-trip on every secret resolution call. It also makes the agent's lineage available to all Hub handlers without a store lookup, which benefits future features beyond secret access.

### 8.4 Depth Limits

**Decision**: No depth limits in the initial implementation.

**Rationale**: The `allowProgeny` flag applies to all descendants, regardless of depth. This matches the simplest mental model ("my agents and their agents can use this"). Depth limits add complexity and are unlikely to be needed initially. They can be added later as an optional field on the policy condition if demand arises.

### 8.5 Grove-Scoped Secrets and Progeny

**Decision**: `allowProgeny` applies to grove-scoped secrets as well, not just user-scoped.

**Rationale**: A grove-scoped secret with `allowProgeny` means "any agent in this grove whose ancestry includes the secret's creator can access this." This is useful for team leads who set grove secrets and want their agents' sub-agents to inherit them. The grove boundary provides the outer security perimeter; `allowProgeny` + `createdBy` provides the inner control.
