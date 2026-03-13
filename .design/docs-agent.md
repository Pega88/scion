# Scion Docs Agent - Design Document

## Overview

A lightweight, standalone satellite service that answers user questions about Scion using Gemini 3.1 Flash-Lite. The service ships as a container deployed to Cloud Run, bundling a checkout of the Scion source code and documentation as context. A simple HTTP handler accepts a query, invokes the Gemini CLI in non-interactive mode with the repository as grounding context, and returns the answer.

This is intentionally separate from the main Scion codebase. It is a small, self-contained project living in `extras/docs-agent/` within the Scion repo, with its own Dockerfile and deployment pipeline.

## Goals

- Provide a public-facing Q&A endpoint for Scion users and contributors
- Answer questions about Scion usage, configuration, architecture, and source code
- Keep the service minimal: no database, no auth, no state
- Leverage existing deployment patterns (Cloud Build + Cloud Run) from `docs-site/`
- Use Gemini 3.1 Flash-Lite for fast, low-cost responses
- Serve an embeddable chat widget suitable for integration into the docs site

## Architecture

```
┌─────────────┐     HTTP POST /ask      ┌──────────────────────────────┐
│   Client     │ ──────────────────────> │   Cloud Run Service          │
│ (browser,    │                         │                              │
│  curl, etc.) │ <───────────────────── │  ┌────────────────────────┐  │
│              │     JSON response       │  │  Go HTTP Handler       │  │
└─────────────┘                         │  │  - Validates query      │  │
                                        │  │  - Invokes gemini CLI   │  │
┌─────────────┐     GET /chat           │  │  - Returns response     │  │
│  docs-site   │ ──────────────────────>│  └────────────────────────┘  │
│  (iframe)    │ <─────────────────────│                              │
│              │     HTML chat widget    │  /workspace/scion/           │
└─────────────┘                         │  (source + docs checkout)    │
                                        └──────────────────────────────┘
```

### Request Flow

1. Client sends `POST /ask` with `{"query": "How do I start an agent?"}`.
2. Go HTTP handler validates the query (length, rate limit).
3. Handler constructs a Gemini CLI invocation with the query as a prompt.
4. Gemini CLI runs against the local source checkout, using it as context.
5. Handler captures stdout, strips any ANSI escape codes, and returns the response as JSON (Markdown body).

### Endpoints

| Method | Path       | Description                                                    |
|--------|------------|----------------------------------------------------------------|
| POST   | `/ask`     | Accepts `{"query": "..."}`, returns `{"answer": "..."}` (Markdown) |
| GET    | `/chat`    | Serves the embeddable Q&A chat widget (HTML/JS/CSS)            |
| POST   | `/refresh` | Triggers a `git pull` on the bundled repo to update content    |
| GET    | `/health`  | Health check                                                   |

## Container Image

### Dockerfile (Conceptual)

```dockerfile
# Stage 1: Clone repo and build the handler
FROM golang:1.25 AS builder

WORKDIR /build

# Clone the Scion repo at build time for latest content
ARG SCION_REPO=https://github.com/GoogleCloudPlatform/scion.git
ARG SCION_REF=main
RUN git clone --depth 1 --branch ${SCION_REF} ${SCION_REPO} /scion-source

# Build the docs-agent handler
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /docs-agent .

# Stage 2: Runtime
FROM node:20-slim

# Install git (needed for /refresh endpoint) and Gemini CLI
RUN apt-get update && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g @google/gemini-cli@latest \
    && npm cache clean --force

# Copy handler binary and source context
COPY --from=builder /docs-agent /usr/local/bin/docs-agent
COPY --from=builder /scion-source /workspace/scion

# Copy the system prompt and chat widget assets
COPY system-prompt.md /etc/docs-agent/system-prompt.md
COPY chat/ /etc/docs-agent/chat/

EXPOSE 8080
CMD ["docs-agent"]
```

Key points:
- The Scion repo is cloned at **image build time**, and can be updated at runtime via the `/refresh` endpoint
- Git is included in the runtime image to support `git pull` on the public repo
- Gemini CLI is installed via npm (same pattern as `image-build/gemini/Dockerfile`)
- No scion-base image dependency; this is fully standalone
- The Go handler is a single static binary

### System Prompt

A `system-prompt.md` file will be included in the image and passed to the CLI via the `GEMINI_SYSTEM_MD` environment variable to completely override the default system instructions, giving Gemini its persona and constraints:

```markdown
You are the Scion Documentation Agent. You answer questions about Scion,
a container-based orchestration platform for concurrent LLM-based code agents.

Your knowledge comes from:
- The Scion documentation in /workspace/scion/docs-site/
- The Scion source code in /workspace/scion/
- Design documents in /workspace/scion/.design/

Rules:
- Answer concisely and accurately based on the available source material
- If you cannot find the answer in the available context, say so
- Do not make up information or speculate about undocumented features
- Reference specific files or documentation pages when possible
- Format responses in Markdown
```

## Gemini CLI Invocation

The Gemini CLI supports non-interactive (headless) use via the `-p` or `--prompt` flag. The invocation uses the `--model` flag to select Flash-Lite:

```bash
GEMINI_SYSTEM_MD=/etc/docs-agent/system-prompt.md \
gemini --prompt "<user_query>" \
       --model gemini-3.1-flash-lite-preview \
       --sandbox_dir /workspace/scion
```

**Key considerations:**
- The `--prompt` flag provides the user query. The `GEMINI_SYSTEM_MD` environment variable points to our custom markdown file, completely replacing the default agent system prompt.
- The `--model gemini-3.1-flash-lite-preview` flag selects the Flash-Lite model for fast, low-cost responses.
- The `--sandbox_dir` flag (if available) or working directory should point to the Scion checkout so Gemini can reference files.
- The process runs to completion and stdout is captured.
- A configurable timeout (e.g., 60 seconds) kills the process if it hangs.

## Go Handler

```go
// Minimal handler sketch
func main() {
    http.HandleFunc("/ask", handleAsk)
    http.HandleFunc("/chat", handleChat)
    http.HandleFunc("/refresh", handleRefresh)
    http.HandleFunc("/health", handleHealth)
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleAsk(w http.ResponseWriter, r *http.Request) {
    // 1. Parse query from JSON body
    // 2. Validate length (e.g., max 1000 chars)
    // 3. Build gemini CLI command with --model gemini-3.1-flash-lite-preview
    // 4. Execute with timeout context
    // 5. Strip ANSI codes from output
    // 6. Return JSON response with Markdown body
}

func handleChat(w http.ResponseWriter, r *http.Request) {
    // Serve the embeddable chat widget HTML/JS/CSS
}

func handleRefresh(w http.ResponseWriter, r *http.Request) {
    // 1. Run "git pull" in /workspace/scion
    // 2. Return success/failure status
}
```

## Chat Widget

The docs-agent serves an iframeable Q&A mini-chat interface at `/chat`, designed for embedding into the docs-site Astro/Starlight layout.

### Design

- **Self-contained:** A single HTML page with inline CSS and JS (or a small bundle), served by the Go handler from `/etc/docs-agent/chat/`.
- **Minimal UI:** A chat-style interface with a text input, submit button, and scrollable message history.
- **Markdown rendering:** Responses from `/ask` are Markdown; the widget renders them to HTML client-side (e.g., using a lightweight library like marked.js).
- **Iframe-friendly:** Designed to work inside an `<iframe>` with no external dependencies, appropriate sizing, and transparent/configurable background.
- **Docs-site integration:** The Astro/Starlight docs-site embeds the widget via an iframe in a sidebar, drawer, or dedicated page:
  ```html
  <iframe src="https://scion-docs-agent-xxxxx.run.app/chat"
          style="width: 100%; height: 500px; border: none;"></iframe>
  ```

### Widget Features

- Conversational display (user question + agent response pairs)
- Loading indicator while waiting for Gemini response
- Error display for failed requests or timeouts
- Responsive layout suitable for sidebar or full-width embedding
- Optional: configurable theme/colors via query parameters to match docs-site styling

## Deployment

### Cloud Run Configuration

Follow the same pattern as `docs-site/deploy.sh` and `docs-site/cloudbuild.yaml`:

- **Project:** `duet01` (or configurable)
- **Region:** `us-west1`
- **Service name:** `scion-docs-agent`
- **Image registry:** `${REGION}-docker.pkg.dev/${PROJECT_ID}/scion-images/docs-agent`
- **Concurrency:** 5 (server handles concurrent requests; each spawns a Gemini CLI process)
- **Memory:** 1Gi (Gemini CLI + Node.js runtime)
- **CPU:** 1-2 vCPUs
- **Timeout:** 120s (request timeout)
- **Min instances:** 0 (scale to zero when idle; cold starts are acceptable for now)
- **Max instances:** 5 (cost control)

### Authentication

The Gemini CLI needs a `GEMINI_API_KEY` (or equivalent). This should be:
- Stored in Google Secret Manager
- Mounted as an environment variable on the Cloud Run service
- **Not** baked into the container image

### Rebuild Trigger

A Cloud Build trigger on the main branch of the Scion repo would rebuild and redeploy the docs-agent image, ensuring the bundled source stays current. Between rebuilds, the `/refresh` endpoint can be called to `git pull` the latest changes from the public repo without requiring a full image rebuild.

## Project Structure

```
extras/docs-agent/
├── main.go              # HTTP handler
├── go.mod
├── go.sum
├── system-prompt.md     # Gemini system prompt
├── chat/
│   └── index.html       # Embeddable chat widget
├── Dockerfile
├── cloudbuild.yaml      # Cloud Build config
├── deploy.sh            # Deploy script
└── README.md
```

## Open Questions

### 1. Rate limiting and abuse prevention

- Should the endpoint be public or require an API key?
- If public, what rate limiting strategy? (Cloud Run has no built-in rate limiting; would need Cloud Armor, API Gateway, or application-level limits)
- Cost control: each request consumes Gemini API credits

### 2. Chat widget theming and integration

- How should the widget adapt its styling to match the docs-site? Query parameters, postMessage API, or a shared CSS variables approach?
- Should the widget support multi-turn conversation context, or is each question independent?

### 3. Refresh endpoint security

- The `/refresh` endpoint triggers a `git pull` on the server. Should it require an auth token or be restricted to internal/admin callers?
- Could be triggered automatically via a Cloud Build post-deploy hook or GitHub webhook.
