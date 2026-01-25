# Scion Web Frontend Design

## Status
**Proposed**

## 1. Overview

The Scion Web Frontend provides a browser-based dashboard for managing agents, groves, and monitoring system status. It is built as a modern server-rendered web application using Lit web components with server-side rendering (SSR) via Koa and `@lit-labs/ssr`.

### Design Goals

1. **Progressive Enhancement:** Server-rendered HTML with hydration for interactive features
2. **Real-Time Updates:** Snapshot + Delta pattern for efficient state synchronization
3. **Component-Driven:** Web Awesome (Shoelace-based) component library with Lit
4. **Minimal Client Complexity:** Server handles API integration; client focuses on presentation
5. **Cloud-Native Deployment:** Optimized for Cloud Run with fast cold starts
6. **Unified Styling:** Web Awesome provides consistent theming with built-in Shoelace integration

### Technology Stack

| Layer | Technology | Purpose |
|-------|------------|---------|
| **Components** | Lit 3.x | Web component framework |
| **UI Library** | Web Awesome | Pre-built component library (Shoelace-based) |
| **Server** | Koa 2.x | Lightweight Node.js web framework |
| **SSR** | @lit-labs/ssr | Server-side rendering for Lit components |
| **Terminal** | xterm.js | PTY display in browser |
| **Real-time** | SSE + NATS | Server-Sent Events backed by NATS pub/sub |
| **Build** | Vite | Fast builds with ES modules |
| **Deployment** | Cloud Run | Serverless container hosting |

---

## 2. Architecture

### 2.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Browser                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     Lit Web Components                               │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │    │
│  │  │  Agent List  │  │  Grove View  │  │  Terminal    │               │    │
│  │  │  Component   │  │  Component   │  │  Component   │               │    │
│  │  └──────────────┘  └──────────────┘  └──────────────┘               │    │
│  │                           │                                          │    │
│  │                    ┌──────┴──────┐                                   │    │
│  │                    │ SSE Client  │  ◄─── Receives deltas             │    │
│  │                    └─────────────┘                                   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└────────────────────────────────────┬────────────────────────────────────────┘
                                     │ HTTPS / WebSocket
                                     ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Web Frontend (Koa)                                 │
│  Port: 9820                                                                  │
│                                                                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │  SSR        │  │  SSE        │  │  API        │  │  Static     │         │
│  │  Renderer   │  │  Endpoint   │  │  Proxy      │  │  Assets     │         │
│  │  (@lit/ssr) │  │  (/events)  │  │  (/api)     │  │  (/assets)  │         │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └─────────────┘         │
│         │                │                │                                  │
│         │         ┌──────┴──────┐         │                                  │
│         │         │   NATS      │         │                                  │
│         │         │   Client    │         │                                  │
│         │         └──────┬──────┘         │                                  │
│         │                │                │                                  │
│         └────────────────┼────────────────┘                                  │
│                          │                                                   │
└──────────────────────────┼───────────────────────────────────────────────────┘
                           │ HTTP + NATS
                           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Hub API (:9810)                                    │
│                                                                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                          │
│  │  REST API   │  │  WebSocket  │  │  NATS       │                          │
│  │  Endpoints  │  │  PTY/Events │  │  Publisher  │                          │
│  └─────────────┘  └─────────────┘  └─────────────┘                          │
│                          │                                                   │
│                    ┌─────┴─────┐                                             │
│                    │  SQL DB   │                                             │
│                    └───────────┘                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Snapshot + Delta Pattern

This pattern provides efficient real-time updates with minimal data transfer:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Snapshot + Delta Data Flow                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  1. Initial Load (Snapshot)                                              │
│     ┌────────┐     ┌──────────┐     ┌─────────┐     ┌─────────┐         │
│     │Browser │────►│Koa Server│────►│ Hub API │────►│  SQL DB │         │
│     │        │◄────│   SSR    │◄────│  REST   │◄────│         │         │
│     └────────┘     └──────────┘     └─────────┘     └─────────┘         │
│         │          Full HTML with                                        │
│         │          current state                                         │
│         ▼                                                                │
│  2. Hydration (Client connects to SSE)                                   │
│     ┌────────┐     ┌──────────┐                                         │
│     │Browser │────►│Koa SSE   │  Client sends: GET /events              │
│     │ Lit    │◄────│ Endpoint │  Server holds connection open           │
│     └────────┘     └──────────┘                                         │
│         │                │                                               │
│         │                ▼                                               │
│  3. Live Wire (NATS subscription)                                        │
│                    ┌──────────┐     ┌─────────┐                         │
│                    │Koa NATS  │◄────│  NATS   │  Subject: resource.*    │
│                    │ Client   │     │  Server │                         │
│                    └──────────┘     └─────────┘                         │
│                         │                ▲                               │
│                         │                │                               │
│  4. Side-Effect Trigger                  │                               │
│     ┌─────────┐     ┌─────────┐         │                               │
│     │  SQL DB │────►│ Hub API │─────────┘  Publishes on DB change       │
│     │ Updated │     │ Service │             to NATS                      │
│     └─────────┘     └─────────┘                                         │
│                         │                                                │
│  5. Reactive Update     │                                                │
│     ┌────────┐     ┌────┴─────┐                                         │
│     │Browser │◄────│Koa SSE   │  Pushes delta payload                   │
│     │Updates │     │ Stream   │  via SSE                                │
│     │  DOM   │     └──────────┘                                         │
│     └────────┘                                                           │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

#### Data Flow Steps

1. **Initial Load (Snapshot):**
   - Browser requests a page (e.g., `/groves/my-project/agents`)
   - Koa server queries Hub API for current state
   - @lit-labs/ssr renders Lit components to HTML with current data
   - Full HTML response sent to browser

2. **Hydration:**
   - Lit components hydrate (become interactive)
   - Client opens SSE connection to `/events` endpoint
   - Server assigns a subscription ID

3. **Live Wire (NATS):**
   - Koa server subscribes to NATS subjects based on client context
   - Example subjects: `grove.{groveId}.agent.*`, `agent.{agentId}.status`

4. **Side-Effect Trigger:**
   - Hub API publishes to NATS when database changes occur
   - Payload contains minimal delta information

5. **Reactive Update:**
   - Koa receives NATS message, pushes to relevant SSE connections
   - Lit components receive delta and update only affected properties
   - Lit's efficient DOM diffing minimizes re-renders

---

## 3. Server Architecture (Koa)

### 3.1 Koa Application Structure

```
web/
├── src/
│   ├── server/
│   │   ├── index.ts              # Koa app entry point
│   │   ├── app.ts                # Koa application setup
│   │   ├── middleware/
│   │   │   ├── auth.ts           # Session/JWT validation
│   │   │   ├── error-handler.ts  # Error boundary
│   │   │   ├── logger.ts         # Request logging
│   │   │   ├── static.ts         # Static asset serving
│   │   │   └── security.ts       # CORS, CSP, etc.
│   │   ├── routes/
│   │   │   ├── index.ts          # Route aggregation
│   │   │   ├── pages.ts          # SSR page routes
│   │   │   ├── api-proxy.ts      # Hub API proxy
│   │   │   ├── sse.ts            # SSE endpoint
│   │   │   ├── auth.ts           # OAuth routes
│   │   │   └── health.ts         # Health checks
│   │   ├── services/
│   │   │   ├── hub-client.ts     # Hub API client
│   │   │   ├── nats-client.ts    # NATS connection
│   │   │   ├── sse-manager.ts    # SSE connection management
│   │   │   └── session.ts        # Session store
│   │   ├── ssr/
│   │   │   ├── renderer.ts       # Lit SSR renderer
│   │   │   ├── templates.ts      # HTML shell templates
│   │   │   └── hydration.ts      # Hydration script generation
│   │   └── config.ts             # Server configuration
│   ├── components/               # Lit components (shared client/server)
│   │   ├── app-shell.ts          # Main application shell
│   │   ├── pages/
│   │   │   ├── dashboard.ts
│   │   │   ├── grove-list.ts
│   │   │   ├── grove-detail.ts
│   │   │   ├── agent-list.ts
│   │   │   ├── agent-detail.ts
│   │   │   └── terminal.ts
│   │   ├── shared/
│   │   │   ├── agent-card.ts
│   │   │   ├── status-badge.ts
│   │   │   ├── grove-selector.ts
│   │   │   └── action-menu.ts
│   │   └── terminal/
│   │       ├── pty-viewer.ts
│   │       └── xterm-wrapper.ts
│   ├── styles/
│   │   ├── theme.css             # Web Awesome theme overrides
│   │   └── utilities.css         # Utility classes
│   └── client/
│       ├── main.ts               # Client entry point
│       ├── sse-client.ts         # SSE connection handler
│       ├── router.ts             # Client-side routing
│       └── state.ts              # Client state management
├── public/
│   └── assets/                   # Static assets
├── vite.config.ts
├── tsconfig.json
└── package.json
```

### 3.2 Koa Middleware Stack

```typescript
// src/server/app.ts
import Koa from 'koa';
import Router from '@koa/router';
import session from 'koa-session';
import cors from '@koa/cors';
import serve from 'koa-static';
import { errorHandler } from './middleware/error-handler';
import { logger } from './middleware/logger';
import { auth } from './middleware/auth';
import { security } from './middleware/security';
import { pageRoutes } from './routes/pages';
import { apiProxy } from './routes/api-proxy';
import { sseRoutes } from './routes/sse';
import { authRoutes } from './routes/auth';
import { healthRoutes } from './routes/health';

export function createApp(config: AppConfig): Koa {
  const app = new Koa();
  const router = new Router();

  // Trust proxy headers (Cloud Run)
  app.proxy = true;

  // Core middleware
  app.use(errorHandler());
  app.use(logger());
  app.use(security(config));
  app.use(cors(config.cors));
  app.use(session(config.session, app));

  // Static assets (with caching)
  app.use(serve('public', {
    maxAge: config.production ? 86400000 : 0, // 24h in prod
    gzip: true,
    brotli: true
  }));

  // Health checks (unauthenticated)
  router.use('/healthz', healthRoutes.routes());
  router.use('/readyz', healthRoutes.routes());

  // Auth routes (login, logout, callback)
  router.use('/auth', authRoutes.routes());

  // API proxy (requires auth)
  router.use('/api', auth(), apiProxy.routes());

  // SSE endpoint (requires auth)
  router.use('/events', auth(), sseRoutes.routes());

  // SSR pages (requires auth for protected routes)
  router.use('/', pageRoutes.routes());

  app.use(router.routes());
  app.use(router.allowedMethods());

  return app;
}
```

### 3.3 SSR Renderer

```typescript
// src/server/ssr/renderer.ts
import { render } from '@lit-labs/ssr';
import { html } from 'lit';
import { unsafeHTML } from 'lit/directives/unsafe-html.js';
import { collectResult } from '@lit-labs/ssr/lib/render-result.js';
import { getHtmlTemplate } from './templates';

export interface RenderContext {
  url: string;
  user: User | null;
  initialData: Record<string, unknown>;
}

export async function renderPage(
  component: unknown,
  ctx: RenderContext
): Promise<string> {
  // Render the Lit component to HTML
  const componentHtml = await collectResult(render(component));

  // Wrap in HTML shell with hydration script
  const fullHtml = getHtmlTemplate({
    title: getPageTitle(ctx.url),
    content: componentHtml,
    initialData: ctx.initialData,
    user: ctx.user,
    scripts: ['/assets/main.js'],
    styles: ['/assets/main.css']
  });

  return fullHtml;
}

// src/server/ssr/templates.ts
export function getHtmlTemplate(opts: HtmlTemplateOptions): string {
  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>${opts.title} - Scion</title>

  <!-- Web Awesome / Shoelace -->
  <link rel="stylesheet" href="https://cdn.webawesome.com/dist/themes/default.css">
  <script type="module" src="https://cdn.webawesome.com/dist/webawesome.js"></script>

  <!-- App styles -->
  ${opts.styles.map(s => `<link rel="stylesheet" href="${s}">`).join('\n  ')}

  <!-- Initial state for hydration -->
  <script id="__SCION_DATA__" type="application/json">
    ${JSON.stringify(opts.initialData)}
  </script>
</head>
<body>
  <div id="app">${opts.content}</div>

  <!-- Hydration scripts -->
  ${opts.scripts.map(s => `<script type="module" src="${s}"></script>`).join('\n  ')}
</body>
</html>`;
}
```

### 3.4 SSE Manager

```typescript
// src/server/services/sse-manager.ts
import { PassThrough } from 'stream';
import { NatsClient } from './nats-client';

interface SSEConnection {
  id: string;
  stream: PassThrough;
  userId: string;
  subscriptions: Set<string>;
  lastEventId: number;
}

export class SSEManager {
  private connections = new Map<string, SSEConnection>();
  private natsClient: NatsClient;

  constructor(natsClient: NatsClient) {
    this.natsClient = natsClient;
  }

  createConnection(userId: string): SSEConnection {
    const id = crypto.randomUUID();
    const stream = new PassThrough();

    const conn: SSEConnection = {
      id,
      stream,
      userId,
      subscriptions: new Set(),
      lastEventId: 0
    };

    this.connections.set(id, conn);

    // Send initial connection event
    this.sendEvent(conn, 'connected', { connectionId: id });

    return conn;
  }

  async subscribe(
    connId: string,
    subject: string,
    filter?: (data: unknown) => boolean
  ): Promise<void> {
    const conn = this.connections.get(connId);
    if (!conn) return;

    conn.subscriptions.add(subject);

    // Subscribe to NATS subject
    await this.natsClient.subscribe(subject, (data) => {
      if (!filter || filter(data)) {
        this.sendEvent(conn, 'update', {
          subject,
          data,
          timestamp: Date.now()
        });
      }
    });
  }

  private sendEvent(conn: SSEConnection, type: string, data: unknown): void {
    conn.lastEventId++;
    const payload = JSON.stringify(data);
    conn.stream.write(`id: ${conn.lastEventId}\n`);
    conn.stream.write(`event: ${type}\n`);
    conn.stream.write(`data: ${payload}\n\n`);
  }

  removeConnection(connId: string): void {
    const conn = this.connections.get(connId);
    if (!conn) return;

    // Unsubscribe from all NATS subjects
    conn.subscriptions.forEach(subject => {
      this.natsClient.unsubscribe(subject);
    });

    conn.stream.end();
    this.connections.delete(connId);
  }
}
```

### 3.5 NATS Client

```typescript
// src/server/services/nats-client.ts
import { connect, NatsConnection, Subscription } from 'nats';

export interface NatsConfig {
  servers: string[];
  token?: string;
  reconnect: boolean;
  maxReconnectAttempts: number;
}

export class NatsClient {
  private connection: NatsConnection | null = null;
  private subscriptions = new Map<string, Subscription>();

  async connect(config: NatsConfig): Promise<void> {
    this.connection = await connect({
      servers: config.servers,
      token: config.token,
      reconnect: config.reconnect,
      maxReconnectAttempts: config.maxReconnectAttempts
    });

    console.log(`Connected to NATS: ${this.connection.getServer()}`);
  }

  async subscribe(
    subject: string,
    handler: (data: unknown) => void
  ): Promise<void> {
    if (!this.connection) {
      throw new Error('NATS not connected');
    }

    const sub = this.connection.subscribe(subject);
    this.subscriptions.set(subject, sub);

    (async () => {
      for await (const msg of sub) {
        try {
          const data = JSON.parse(new TextDecoder().decode(msg.data));
          handler(data);
        } catch (err) {
          console.error('Failed to parse NATS message:', err);
        }
      }
    })();
  }

  async publish(subject: string, data: unknown): Promise<void> {
    if (!this.connection) {
      throw new Error('NATS not connected');
    }

    const payload = new TextEncoder().encode(JSON.stringify(data));
    this.connection.publish(subject, payload);
  }

  unsubscribe(subject: string): void {
    const sub = this.subscriptions.get(subject);
    if (sub) {
      sub.unsubscribe();
      this.subscriptions.delete(subject);
    }
  }

  async close(): Promise<void> {
    if (this.connection) {
      await this.connection.drain();
      this.connection = null;
    }
  }
}
```

### 3.6 Hub API Proxy

```typescript
// src/server/routes/api-proxy.ts
import Router from '@koa/router';
import { Context } from 'koa';
import httpProxy from 'http-proxy-middleware';

const router = new Router();

// Proxy configuration
const hubApiUrl = process.env.HUB_API_URL || 'http://localhost:9810';

// Proxy all /api/* requests to Hub API
router.all('/(.*)', async (ctx: Context) => {
  const targetPath = `/api/v1/${ctx.params[0]}`;

  const response = await fetch(`${hubApiUrl}${targetPath}`, {
    method: ctx.method,
    headers: {
      'Content-Type': 'application/json',
      'Authorization': ctx.headers.authorization || '',
      'X-Request-ID': ctx.state.requestId,
      'X-Forwarded-For': ctx.ip
    },
    body: ctx.method !== 'GET' ? JSON.stringify(ctx.request.body) : undefined
  });

  ctx.status = response.status;
  ctx.set('Content-Type', response.headers.get('Content-Type') || 'application/json');

  // Forward rate limit headers
  const rateLimitHeaders = ['X-RateLimit-Limit', 'X-RateLimit-Remaining', 'X-RateLimit-Reset'];
  rateLimitHeaders.forEach(header => {
    const value = response.headers.get(header);
    if (value) ctx.set(header, value);
  });

  ctx.body = await response.json();
});

export const apiProxy = router;
```

---

## 4. Client Architecture (Lit Components)

### 4.1 Component Library: Web Awesome

Web Awesome is a component library built on Shoelace, providing:

- **Pre-styled components:** Buttons, cards, dialogs, tables, etc.
- **Built-in theming:** Shoelace-compatible CSS custom properties
- **Accessibility:** WCAG 2.1 AA compliant
- **Framework-agnostic:** Works with Lit, React, Vue, or vanilla JS

```html
<!-- Example: Using Web Awesome components -->
<wa-card>
  <div slot="header">
    <wa-icon name="box"></wa-icon>
    Agent Status
  </div>
  <wa-badge variant="success">Running</wa-badge>
  <div slot="footer">
    <wa-button variant="primary">Attach Terminal</wa-button>
    <wa-button variant="danger">Stop</wa-button>
  </div>
</wa-card>
```

### 4.2 Core Components

#### App Shell

```typescript
// src/components/app-shell.ts
import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { Router } from '@vaadin/router';

import './pages/dashboard';
import './pages/grove-list';
import './pages/grove-detail';
import './pages/agent-list';
import './pages/agent-detail';
import './pages/terminal';

@customElement('scion-app')
export class ScionApp extends LitElement {
  @property({ type: Object }) user: User | null = null;
  @state() private currentRoute = '';

  static styles = css`
    :host {
      display: flex;
      min-height: 100vh;
    }

    .sidebar {
      width: 260px;
      background: var(--wa-color-neutral-100);
      border-right: 1px solid var(--wa-color-neutral-200);
    }

    .main {
      flex: 1;
      display: flex;
      flex-direction: column;
    }

    .header {
      height: 60px;
      padding: 0 1.5rem;
      display: flex;
      align-items: center;
      justify-content: space-between;
      border-bottom: 1px solid var(--wa-color-neutral-200);
    }

    .content {
      flex: 1;
      padding: 1.5rem;
      overflow: auto;
    }
  `;

  firstUpdated() {
    const outlet = this.shadowRoot?.querySelector('#outlet');
    if (outlet) {
      const router = new Router(outlet);
      router.setRoutes([
        { path: '/', component: 'scion-dashboard' },
        { path: '/groves', component: 'scion-grove-list' },
        { path: '/groves/:groveId', component: 'scion-grove-detail' },
        { path: '/groves/:groveId/agents', component: 'scion-agent-list' },
        { path: '/agents/:agentId', component: 'scion-agent-detail' },
        { path: '/agents/:agentId/terminal', component: 'scion-terminal' },
      ]);
    }
  }

  render() {
    return html`
      <aside class="sidebar">
        <scion-nav .user=${this.user}></scion-nav>
      </aside>
      <main class="main">
        <header class="header">
          <scion-breadcrumb></scion-breadcrumb>
          <scion-user-menu .user=${this.user}></scion-user-menu>
        </header>
        <div class="content">
          <div id="outlet"></div>
        </div>
      </main>
    `;
  }
}
```

#### Agent Card Component

```typescript
// src/components/shared/agent-card.ts
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import type { Agent } from '../types';

@customElement('scion-agent-card')
export class AgentCard extends LitElement {
  @property({ type: Object }) agent!: Agent;

  static styles = css`
    :host {
      display: block;
    }

    wa-card {
      --wa-card-border-radius: var(--wa-border-radius-large);
    }

    .header {
      display: flex;
      align-items: center;
      gap: 0.75rem;
    }

    .title {
      font-weight: 600;
      font-size: 1rem;
    }

    .template {
      font-size: 0.875rem;
      color: var(--wa-color-neutral-600);
    }

    .status-row {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      margin-top: 1rem;
    }

    .actions {
      display: flex;
      gap: 0.5rem;
    }
  `;

  private getStatusVariant(status: string): string {
    const variants: Record<string, string> = {
      running: 'success',
      stopped: 'neutral',
      provisioning: 'warning',
      error: 'danger'
    };
    return variants[status] || 'neutral';
  }

  private handleAction(action: string) {
    this.dispatchEvent(new CustomEvent('agent-action', {
      detail: { agentId: this.agent.id, action },
      bubbles: true,
      composed: true
    }));
  }

  render() {
    const { agent } = this;

    return html`
      <wa-card>
        <div slot="header" class="header">
          <wa-icon name="cpu"></wa-icon>
          <div>
            <div class="title">${agent.name}</div>
            <div class="template">${agent.template}</div>
          </div>
        </div>

        <div class="status-row">
          <wa-badge variant="${this.getStatusVariant(agent.status)}">
            ${agent.status}
          </wa-badge>
          ${agent.sessionStatus ? html`
            <wa-badge variant="primary" size="small">
              ${agent.sessionStatus}
            </wa-badge>
          ` : ''}
        </div>

        ${agent.taskSummary ? html`
          <p class="task">${agent.taskSummary}</p>
        ` : ''}

        <div slot="footer" class="actions">
          <wa-button
            variant="primary"
            size="small"
            @click=${() => this.handleAction('terminal')}
            ?disabled=${agent.status !== 'running'}
          >
            <wa-icon slot="prefix" name="terminal"></wa-icon>
            Terminal
          </wa-button>
          ${agent.status === 'running' ? html`
            <wa-button
              variant="danger"
              size="small"
              @click=${() => this.handleAction('stop')}
            >
              Stop
            </wa-button>
          ` : html`
            <wa-button
              variant="success"
              size="small"
              @click=${() => this.handleAction('start')}
            >
              Start
            </wa-button>
          `}
        </div>
      </wa-card>
    `;
  }
}
```

### 4.3 SSE Client

```typescript
// src/client/sse-client.ts

export interface SSEEvent {
  type: string;
  data: unknown;
  id: string;
}

export class SSEClient extends EventTarget {
  private eventSource: EventSource | null = null;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 10;
  private reconnectDelay = 1000;

  constructor(private url: string) {
    super();
  }

  connect(): void {
    this.eventSource = new EventSource(this.url, {
      withCredentials: true
    });

    this.eventSource.onopen = () => {
      console.log('SSE connected');
      this.reconnectAttempts = 0;
      this.dispatchEvent(new CustomEvent('connected'));
    };

    this.eventSource.onerror = (error) => {
      console.error('SSE error:', error);
      this.handleReconnect();
    };

    // Handle custom event types
    this.eventSource.addEventListener('update', (event) => {
      const data = JSON.parse((event as MessageEvent).data);
      this.dispatchEvent(new CustomEvent('update', { detail: data }));
    });

    this.eventSource.addEventListener('connected', (event) => {
      const data = JSON.parse((event as MessageEvent).data);
      console.log('SSE connection ID:', data.connectionId);
    });
  }

  private handleReconnect(): void {
    if (this.eventSource) {
      this.eventSource.close();
    }

    if (this.reconnectAttempts < this.maxReconnectAttempts) {
      this.reconnectAttempts++;
      const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);
      console.log(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);

      setTimeout(() => this.connect(), delay);
    } else {
      this.dispatchEvent(new CustomEvent('disconnected'));
    }
  }

  subscribe(subjects: string[]): void {
    // Send subscription request to server
    fetch('/events/subscribe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ subjects }),
      credentials: 'include'
    });
  }

  disconnect(): void {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
  }
}
```

### 4.4 Reactive State Management

```typescript
// src/client/state.ts
import { SSEClient } from './sse-client';

export interface AppState {
  agents: Map<string, Agent>;
  groves: Map<string, Grove>;
  connected: boolean;
}

export class StateManager extends EventTarget {
  private state: AppState = {
    agents: new Map(),
    groves: new Map(),
    connected: false
  };

  private sseClient: SSEClient;

  constructor() {
    super();
    this.sseClient = new SSEClient('/events');

    // Handle SSE updates
    this.sseClient.addEventListener('update', ((event: CustomEvent) => {
      this.handleUpdate(event.detail);
    }) as EventListener);

    this.sseClient.addEventListener('connected', () => {
      this.state.connected = true;
      this.notify('connected');
    });

    this.sseClient.addEventListener('disconnected', () => {
      this.state.connected = false;
      this.notify('disconnected');
    });
  }

  // Initialize with server-rendered data
  hydrate(initialData: { agents?: Agent[]; groves?: Grove[] }): void {
    if (initialData.agents) {
      initialData.agents.forEach(agent => {
        this.state.agents.set(agent.id, agent);
      });
    }

    if (initialData.groves) {
      initialData.groves.forEach(grove => {
        this.state.groves.set(grove.id, grove);
      });
    }
  }

  connect(): void {
    this.sseClient.connect();
  }

  // Subscribe to specific resources
  watchAgents(groveId?: string): void {
    const subject = groveId
      ? `grove.${groveId}.agent.*`
      : 'agent.*';
    this.sseClient.subscribe([subject]);
  }

  watchAgent(agentId: string): void {
    this.sseClient.subscribe([`agent.${agentId}.*`]);
  }

  // Handle delta updates from SSE
  private handleUpdate(update: { subject: string; data: unknown }): void {
    const { subject, data } = update;

    // Parse subject to determine update type
    // e.g., "agent.abc123.status" or "grove.xyz.agent.created"
    const parts = subject.split('.');

    if (parts[0] === 'agent') {
      const agentId = parts[1];
      const eventType = parts[2];

      if (eventType === 'deleted') {
        this.state.agents.delete(agentId);
      } else {
        // Merge delta into existing agent
        const existing = this.state.agents.get(agentId) || {} as Agent;
        const updated = { ...existing, ...(data as Partial<Agent>) };
        this.state.agents.set(agentId, updated);
      }

      this.notify('agents-updated');
    }

    if (parts[0] === 'grove') {
      const groveId = parts[1];

      if (parts[2] === 'agent') {
        // Agent within grove updated
        const agentData = data as Agent;
        this.state.agents.set(agentData.id, agentData);
        this.notify('agents-updated');
      } else {
        // Grove itself updated
        const existing = this.state.groves.get(groveId) || {} as Grove;
        const updated = { ...existing, ...(data as Partial<Grove>) };
        this.state.groves.set(groveId, updated);
        this.notify('groves-updated');
      }
    }
  }

  private notify(event: string): void {
    this.dispatchEvent(new CustomEvent(event, { detail: this.state }));
  }

  // Getters
  getAgents(): Agent[] {
    return Array.from(this.state.agents.values());
  }

  getAgent(id: string): Agent | undefined {
    return this.state.agents.get(id);
  }

  getGroves(): Grove[] {
    return Array.from(this.state.groves.values());
  }

  getGrove(id: string): Grove | undefined {
    return this.state.groves.get(id);
  }
}

// Singleton instance
export const stateManager = new StateManager();
```

---

## 5. Terminal Component (xterm.js)

### 5.1 Terminal Wrapper

```typescript
// src/components/terminal/pty-viewer.ts
import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { WebLinksAddon } from 'xterm-addon-web-links';

@customElement('scion-terminal')
export class ScionTerminal extends LitElement {
  @property({ type: String }) agentId = '';
  @state() private connected = false;
  @state() private error: string | null = null;

  private terminal: Terminal | null = null;
  private fitAddon: FitAddon | null = null;
  private socket: WebSocket | null = null;
  private resizeObserver: ResizeObserver | null = null;

  static styles = css`
    :host {
      display: flex;
      flex-direction: column;
      height: 100%;
      background: var(--wa-color-neutral-900);
      border-radius: var(--wa-border-radius-medium);
      overflow: hidden;
    }

    .toolbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 0.5rem 1rem;
      background: var(--wa-color-neutral-800);
      border-bottom: 1px solid var(--wa-color-neutral-700);
    }

    .status {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      font-size: 0.875rem;
      color: var(--wa-color-neutral-400);
    }

    .status-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--wa-color-danger-500);
    }

    .status-dot.connected {
      background: var(--wa-color-success-500);
    }

    .terminal-container {
      flex: 1;
      padding: 0.5rem;
    }

    .error {
      color: var(--wa-color-danger-500);
      padding: 1rem;
      text-align: center;
    }
  `;

  async connectedCallback() {
    super.connectedCallback();
    await this.initTerminal();
    this.connectWebSocket();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.cleanup();
  }

  private async initTerminal(): Promise<void> {
    // Wait for first render
    await this.updateComplete;

    const container = this.shadowRoot?.querySelector('.terminal-container');
    if (!container) return;

    this.terminal = new Terminal({
      theme: {
        background: '#1a1a2e',
        foreground: '#eaeaea',
        cursor: '#f39c12',
        cursorAccent: '#1a1a2e',
        selection: 'rgba(255, 255, 255, 0.3)',
        black: '#1a1a2e',
        brightBlack: '#6c7086',
        red: '#f38ba8',
        brightRed: '#f38ba8',
        green: '#a6e3a1',
        brightGreen: '#a6e3a1',
        yellow: '#f9e2af',
        brightYellow: '#f9e2af',
        blue: '#89b4fa',
        brightBlue: '#89b4fa',
        magenta: '#cba6f7',
        brightMagenta: '#cba6f7',
        cyan: '#94e2d5',
        brightCyan: '#94e2d5',
        white: '#bac2de',
        brightWhite: '#ffffff'
      },
      fontFamily: 'JetBrains Mono, Menlo, Monaco, monospace',
      fontSize: 14,
      cursorBlink: true,
      cursorStyle: 'block',
      allowProposedApi: true
    });

    this.fitAddon = new FitAddon();
    this.terminal.loadAddon(this.fitAddon);
    this.terminal.loadAddon(new WebLinksAddon());

    this.terminal.open(container as HTMLElement);
    this.fitAddon.fit();

    // Handle terminal input
    this.terminal.onData((data) => {
      this.sendData(data);
    });

    // Handle resize
    this.resizeObserver = new ResizeObserver(() => {
      this.fitAddon?.fit();
      this.sendResize();
    });
    this.resizeObserver.observe(container);
  }

  private async connectWebSocket(): Promise<void> {
    try {
      // Get WebSocket ticket from API
      const ticketResponse = await fetch('/api/auth/ws-ticket', {
        method: 'POST',
        credentials: 'include'
      });
      const { ticket } = await ticketResponse.json();

      // Connect to PTY WebSocket
      const wsUrl = `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/api/agents/${this.agentId}/pty?ticket=${ticket}`;
      this.socket = new WebSocket(wsUrl);

      this.socket.onopen = () => {
        this.connected = true;
        this.error = null;
        this.sendResize();
      };

      this.socket.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        if (msg.type === 'data') {
          // Decode base64 data
          const bytes = Uint8Array.from(atob(msg.data), c => c.charCodeAt(0));
          this.terminal?.write(bytes);
        }
      };

      this.socket.onerror = () => {
        this.error = 'Connection error';
        this.connected = false;
      };

      this.socket.onclose = (event) => {
        this.connected = false;
        if (event.code !== 1000) {
          this.error = `Connection closed: ${event.reason || 'Unknown error'}`;
        }
      };

    } catch (err) {
      this.error = `Failed to connect: ${err}`;
    }
  }

  private sendData(data: string): void {
    if (this.socket?.readyState === WebSocket.OPEN) {
      const encoded = btoa(data);
      this.socket.send(JSON.stringify({ type: 'data', data: encoded }));
    }
  }

  private sendResize(): void {
    if (this.socket?.readyState === WebSocket.OPEN && this.terminal) {
      this.socket.send(JSON.stringify({
        type: 'resize',
        cols: this.terminal.cols,
        rows: this.terminal.rows
      }));
    }
  }

  private cleanup(): void {
    this.socket?.close();
    this.terminal?.dispose();
    this.resizeObserver?.disconnect();
  }

  private handleReconnect(): void {
    this.cleanup();
    this.initTerminal();
    this.connectWebSocket();
  }

  render() {
    return html`
      <div class="toolbar">
        <div class="status">
          <div class="status-dot ${this.connected ? 'connected' : ''}"></div>
          ${this.connected ? 'Connected' : 'Disconnected'}
        </div>
        <div class="actions">
          <wa-button size="small" variant="text" @click=${this.handleReconnect}>
            <wa-icon name="refresh-cw"></wa-icon>
          </wa-button>
        </div>
      </div>
      ${this.error ? html`
        <div class="error">
          <wa-icon name="alert-circle"></wa-icon>
          ${this.error}
        </div>
      ` : ''}
      <div class="terminal-container"></div>
    `;
  }
}
```

---

## 6. NATS Integration

### 6.1 NATS Subject Schema

The Hub API publishes events to NATS when database changes occur. The Web Frontend subscribes to relevant subjects based on user context.

| Subject Pattern | Description | Payload |
|-----------------|-------------|---------|
| `agent.{agentId}.status` | Agent status change | `{ status, sessionStatus, containerStatus }` |
| `agent.{agentId}.event` | Agent event (harness) | `StatusEvent` |
| `agent.{agentId}.created` | Agent created | Full `Agent` object |
| `agent.{agentId}.deleted` | Agent deleted | `{ agentId }` |
| `grove.{groveId}.agent.created` | Agent created in grove | Full `Agent` object |
| `grove.{groveId}.agent.deleted` | Agent deleted from grove | `{ agentId }` |
| `grove.{groveId}.updated` | Grove metadata changed | `{ name?, labels?, ... }` |
| `grove.{groveId}.host.connected` | Host joined grove | `{ hostId, hostName }` |
| `grove.{groveId}.host.disconnected` | Host left grove | `{ hostId }` |
| `host.{hostId}.status` | Host status change | `{ status, resources }` |

### 6.2 Hub-Side Publishing

The Hub API publishes events after successful database operations:

```go
// pkg/hub/service/agent_service.go

func (s *AgentService) UpdateStatus(ctx context.Context, agentID string, status StatusUpdate) error {
    // Update database
    if err := s.store.UpdateAgentStatus(ctx, agentID, status); err != nil {
        return err
    }

    // Publish to NATS
    agent, _ := s.store.GetAgent(ctx, agentID)
    s.nats.Publish(fmt.Sprintf("agent.%s.status", agentID), map[string]interface{}{
        "status":          status.Status,
        "sessionStatus":   status.SessionStatus,
        "containerStatus": status.ContainerStatus,
        "timestamp":       time.Now().UTC(),
    })

    // Also publish to grove subject for grove-level subscribers
    s.nats.Publish(fmt.Sprintf("grove.%s.agent.status", agent.GroveID), map[string]interface{}{
        "agentId":         agentID,
        "status":          status.Status,
        "sessionStatus":   status.SessionStatus,
        "containerStatus": status.ContainerStatus,
        "timestamp":       time.Now().UTC(),
    })

    return nil
}
```

### 6.3 Web Frontend Subscription Logic

```typescript
// src/server/routes/sse.ts
import Router from '@koa/router';
import { Context } from 'koa';
import { sseManager } from '../services/sse-manager';

const router = new Router();

// Main SSE endpoint
router.get('/', async (ctx: Context) => {
  // Create SSE connection
  const conn = sseManager.createConnection(ctx.state.user.id);

  // Set SSE headers
  ctx.set({
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    'Connection': 'keep-alive',
    'X-Accel-Buffering': 'no'
  });

  ctx.status = 200;
  ctx.body = conn.stream;

  // Cleanup on close
  ctx.req.on('close', () => {
    sseManager.removeConnection(conn.id);
  });
});

// Subscribe to specific subjects
router.post('/subscribe', async (ctx: Context) => {
  const { connectionId, subjects } = ctx.request.body as {
    connectionId: string;
    subjects: string[];
  };

  // Validate subjects against user permissions
  const allowedSubjects = subjects.filter(subject =>
    canSubscribe(ctx.state.user, subject)
  );

  for (const subject of allowedSubjects) {
    await sseManager.subscribe(connectionId, subject);
  }

  ctx.body = { subscribed: allowedSubjects };
});

function canSubscribe(user: User, subject: string): boolean {
  // Check if user has permission to subscribe to this subject
  // e.g., can only subscribe to groves they have access to
  const parts = subject.split('.');

  if (parts[0] === 'grove') {
    const groveId = parts[1];
    // Check grove access (implementation depends on your auth model)
    return userHasGroveAccess(user, groveId);
  }

  if (parts[0] === 'agent') {
    const agentId = parts[1];
    return userHasAgentAccess(user, agentId);
  }

  return false;
}

export const sseRoutes = router;
```

---

## 7. Authentication

### 7.1 OAuth Flow

```
┌─────────┐     ┌─────────────┐     ┌──────────────┐     ┌─────────┐
│ Browser │────►│Web Frontend │────►│OAuth Provider│────►│ Hub API │
│         │     │   :9820     │     │(Google/GitHub)│     │ :9810   │
└─────────┘     └─────────────┘     └──────────────┘     └─────────┘
     │                │                    │                  │
     │  1. /auth/login                     │                  │
     │───────────────►│                    │                  │
     │                │  2. Redirect       │                  │
     │◄───────────────│───────────────────►│                  │
     │  3. OAuth flow │                    │                  │
     │◄───────────────────────────────────►│                  │
     │                │  4. Callback       │                  │
     │───────────────►│◄───────────────────│                  │
     │                │  5. Exchange code  │                  │
     │                │───────────────────►│                  │
     │                │◄───────────────────│                  │
     │                │  6. Create/get user                   │
     │                │────────────────────────────────────►│
     │                │◄────────────────────────────────────│
     │                │  7. Set session cookie               │
     │◄───────────────│                    │                  │
     │  8. Redirect   │                    │                  │
     │◄───────────────│                    │                  │
```

### 7.2 Auth Routes

```typescript
// src/server/routes/auth.ts
import Router from '@koa/router';
import { Context } from 'koa';
import { OAuth2Client } from 'google-auth-library';

const router = new Router();

// OAuth configuration
const oauth = {
  google: new OAuth2Client({
    clientId: process.env.GOOGLE_CLIENT_ID,
    clientSecret: process.env.GOOGLE_CLIENT_SECRET,
    redirectUri: `${process.env.BASE_URL}/auth/callback/google`
  }),
  github: {
    clientId: process.env.GITHUB_CLIENT_ID,
    clientSecret: process.env.GITHUB_CLIENT_SECRET
  }
};

// Login initiation
router.get('/login/:provider', async (ctx: Context) => {
  const { provider } = ctx.params;
  const returnTo = ctx.query.returnTo || '/';

  // Store returnTo in session
  ctx.session.returnTo = returnTo;

  if (provider === 'google') {
    const authUrl = oauth.google.generateAuthUrl({
      access_type: 'offline',
      scope: ['email', 'profile']
    });
    ctx.redirect(authUrl);
  } else if (provider === 'github') {
    const authUrl = `https://github.com/login/oauth/authorize?client_id=${oauth.github.clientId}&scope=user:email`;
    ctx.redirect(authUrl);
  } else {
    ctx.status = 400;
    ctx.body = { error: 'Unknown provider' };
  }
});

// OAuth callback
router.get('/callback/:provider', async (ctx: Context) => {
  const { provider } = ctx.params;
  const { code } = ctx.query;

  try {
    let userInfo: { email: string; name: string; avatar?: string };

    if (provider === 'google') {
      const { tokens } = await oauth.google.getToken(code as string);
      oauth.google.setCredentials(tokens);
      const ticket = await oauth.google.verifyIdToken({
        idToken: tokens.id_token!,
        audience: process.env.GOOGLE_CLIENT_ID
      });
      const payload = ticket.getPayload()!;
      userInfo = {
        email: payload.email!,
        name: payload.name!,
        avatar: payload.picture
      };
    } else if (provider === 'github') {
      // Exchange code for token
      const tokenRes = await fetch('https://github.com/login/oauth/access_token', {
        method: 'POST',
        headers: {
          'Accept': 'application/json',
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          client_id: oauth.github.clientId,
          client_secret: oauth.github.clientSecret,
          code
        })
      });
      const { access_token } = await tokenRes.json();

      // Get user info
      const userRes = await fetch('https://api.github.com/user', {
        headers: { Authorization: `Bearer ${access_token}` }
      });
      const ghUser = await userRes.json();
      userInfo = {
        email: ghUser.email,
        name: ghUser.name || ghUser.login,
        avatar: ghUser.avatar_url
      };
    } else {
      throw new Error('Unknown provider');
    }

    // Create or get user from Hub API
    const hubResponse = await fetch(`${process.env.HUB_API_URL}/api/v1/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        provider,
        email: userInfo.email,
        name: userInfo.name,
        avatar: userInfo.avatar
      })
    });

    const { user, token } = await hubResponse.json();

    // Set session
    ctx.session.user = user;
    ctx.session.token = token;

    // Redirect to original destination
    const returnTo = ctx.session.returnTo || '/';
    delete ctx.session.returnTo;
    ctx.redirect(returnTo);

  } catch (err) {
    console.error('OAuth error:', err);
    ctx.redirect('/auth/error?message=Authentication+failed');
  }
});

// Logout
router.post('/logout', async (ctx: Context) => {
  ctx.session = null;
  ctx.body = { success: true };
});

// Current user
router.get('/me', async (ctx: Context) => {
  if (ctx.session.user) {
    ctx.body = { user: ctx.session.user };
  } else {
    ctx.status = 401;
    ctx.body = { error: 'Not authenticated' };
  }
});

export const authRoutes = router;
```

### 7.3 Session Configuration

```typescript
// src/server/config.ts

export interface SessionConfig {
  key: string;
  maxAge: number;
  secure: boolean;
  httpOnly: boolean;
  sameSite: 'strict' | 'lax' | 'none';
  signed: boolean;
}

export function getSessionConfig(): SessionConfig {
  return {
    key: 'scion:sess',
    maxAge: 24 * 60 * 60 * 1000, // 24 hours
    secure: process.env.NODE_ENV === 'production',
    httpOnly: true,
    sameSite: 'lax',
    signed: true
  };
}
```

---

## 8. Cloud Run Deployment

### 8.1 Container Configuration

```dockerfile
# Dockerfile
FROM node:20-alpine AS builder

WORKDIR /app
COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

# Production image
FROM node:20-alpine AS runner

WORKDIR /app

# Copy built assets
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/public ./public
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/package.json ./

# Non-root user
RUN addgroup -g 1001 -S nodejs
RUN adduser -S scion -u 1001
USER scion

ENV NODE_ENV=production
ENV PORT=8080

EXPOSE 8080

CMD ["node", "dist/server/index.js"]
```

### 8.2 Cloud Run Service Definition

```yaml
# cloudrun.yaml
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: scion-web
  labels:
    app: scion
    component: web-frontend
spec:
  template:
    metadata:
      annotations:
        autoscaling.knative.dev/minScale: "1"
        autoscaling.knative.dev/maxScale: "10"
        run.googleapis.com/cpu-throttling: "false"
        run.googleapis.com/startup-cpu-boost: "true"
    spec:
      containerConcurrency: 80
      timeoutSeconds: 300
      containers:
        - image: gcr.io/PROJECT_ID/scion-web:latest
          ports:
            - containerPort: 8080
          env:
            - name: NODE_ENV
              value: production
            - name: HUB_API_URL
              value: http://scion-hub:9810
            - name: NATS_URL
              valueFrom:
                secretKeyRef:
                  name: scion-secrets
                  key: nats-url
            - name: SESSION_SECRET
              valueFrom:
                secretKeyRef:
                  name: scion-secrets
                  key: session-secret
            - name: GOOGLE_CLIENT_ID
              valueFrom:
                secretKeyRef:
                  name: scion-secrets
                  key: google-client-id
            - name: GOOGLE_CLIENT_SECRET
              valueFrom:
                secretKeyRef:
                  name: scion-secrets
                  key: google-client-secret
          resources:
            limits:
              cpu: "2"
              memory: 1Gi
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /readyz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
```

### 8.3 Cold Start Optimization

Cloud Run instances may be scaled to zero. Optimize cold starts:

1. **Minimal dependencies:** Use ES modules, tree-shake unused code
2. **Lazy loading:** Load NATS client and other services on first request
3. **Startup CPU boost:** Enable `run.googleapis.com/startup-cpu-boost`
4. **Precompiled templates:** Pre-render static content during build

```typescript
// src/server/index.ts
import { createApp } from './app';
import { loadConfig } from './config';

async function main() {
  const config = loadConfig();
  const app = createApp(config);

  // Lazy-initialize services
  let natsClient: NatsClient | null = null;

  app.use(async (ctx, next) => {
    // Initialize NATS on first request
    if (!natsClient) {
      natsClient = new NatsClient();
      await natsClient.connect(config.nats);
    }
    ctx.state.nats = natsClient;
    await next();
  });

  const port = process.env.PORT || 8080;
  app.listen(port, () => {
    console.log(`Server listening on port ${port}`);
  });
}

main().catch(console.error);
```

### 8.4 Health Endpoints

```typescript
// src/server/routes/health.ts
import Router from '@koa/router';
import { Context } from 'koa';

const router = new Router();

// Liveness probe
router.get('/healthz', async (ctx: Context) => {
  ctx.body = {
    status: 'healthy',
    timestamp: new Date().toISOString()
  };
});

// Readiness probe
router.get('/readyz', async (ctx: Context) => {
  const checks = {
    hubApi: await checkHubApi(),
    nats: await checkNats()
  };

  const allHealthy = Object.values(checks).every(c => c === 'healthy');

  ctx.status = allHealthy ? 200 : 503;
  ctx.body = {
    status: allHealthy ? 'ready' : 'not ready',
    checks,
    timestamp: new Date().toISOString()
  };
});

async function checkHubApi(): Promise<string> {
  try {
    const res = await fetch(`${process.env.HUB_API_URL}/healthz`, {
      timeout: 5000
    });
    return res.ok ? 'healthy' : 'unhealthy';
  } catch {
    return 'unhealthy';
  }
}

async function checkNats(): Promise<string> {
  // Check NATS connection status
  // Implementation depends on nats.js client
  return 'healthy';
}

export const healthRoutes = router;
```

---

## 9. Configuration

### 9.1 Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PORT` | No | `8080` | Server port |
| `NODE_ENV` | No | `development` | Environment mode |
| `HUB_API_URL` | Yes | - | Hub API base URL |
| `NATS_URL` | Yes | - | NATS server URL |
| `SESSION_SECRET` | Yes | - | Session signing secret |
| `GOOGLE_CLIENT_ID` | No | - | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | No | - | Google OAuth client secret |
| `GITHUB_CLIENT_ID` | No | - | GitHub OAuth client ID |
| `GITHUB_CLIENT_SECRET` | No | - | GitHub OAuth client secret |
| `BASE_URL` | Yes | - | Public base URL for OAuth callbacks |
| `LOG_LEVEL` | No | `info` | Logging level |
| `LOG_FORMAT` | No | `json` | Log format (json/text) |

### 9.2 YAML Configuration

```yaml
# config/default.yaml
server:
  port: 8080
  trustProxy: true

hub:
  url: "http://localhost:9810"
  timeout: 30000

nats:
  servers:
    - "nats://localhost:4222"
  reconnect: true
  maxReconnectAttempts: 10

session:
  maxAge: 86400000  # 24 hours
  secure: true
  sameSite: lax

auth:
  providers:
    - google
    - github

sse:
  heartbeatInterval: 30000
  maxConnectionsPerUser: 5

assets:
  maxAge: 86400  # 24 hours
  gzip: true
  brotli: true
```

---

## 10. Security Considerations

### 10.1 Content Security Policy

```typescript
// src/server/middleware/security.ts
import { Context, Next } from 'koa';

export function security(config: AppConfig) {
  return async (ctx: Context, next: Next) => {
    // Content Security Policy
    ctx.set('Content-Security-Policy', [
      "default-src 'self'",
      "script-src 'self' 'unsafe-inline' https://cdn.webawesome.com",
      "style-src 'self' 'unsafe-inline' https://cdn.webawesome.com",
      "font-src 'self' https://cdn.webawesome.com",
      "img-src 'self' data: https:",
      "connect-src 'self' wss: https:",
      "frame-ancestors 'none'"
    ].join('; '));

    // Other security headers
    ctx.set('X-Content-Type-Options', 'nosniff');
    ctx.set('X-Frame-Options', 'DENY');
    ctx.set('X-XSS-Protection', '1; mode=block');
    ctx.set('Referrer-Policy', 'strict-origin-when-cross-origin');

    if (config.production) {
      ctx.set('Strict-Transport-Security', 'max-age=31536000; includeSubDomains');
    }

    await next();
  };
}
```

### 10.2 CSRF Protection

```typescript
// src/server/middleware/csrf.ts
import { Context, Next } from 'koa';
import csrf from 'koa-csrf';

export function csrfProtection() {
  return new csrf({
    invalidTokenMessage: 'Invalid CSRF token',
    invalidTokenStatusCode: 403,
    excludedMethods: ['GET', 'HEAD', 'OPTIONS'],
    disableQuery: true
  });
}
```

### 10.3 Rate Limiting

```typescript
// src/server/middleware/rate-limit.ts
import ratelimit from 'koa-ratelimit';

export function rateLimiter() {
  const db = new Map();

  return ratelimit({
    driver: 'memory',
    db,
    duration: 60000, // 1 minute
    max: 100,
    id: (ctx) => ctx.ip,
    headers: {
      remaining: 'X-RateLimit-Remaining',
      reset: 'X-RateLimit-Reset',
      total: 'X-RateLimit-Total'
    },
    disableHeader: false
  });
}
```

---

## 11. Implementation Plan

### Phase 1: Core Server Setup
- [ ] Koa application structure
- [ ] Static asset serving
- [ ] Health endpoints
- [ ] Session management
- [ ] Hub API proxy

### Phase 2: SSR with Lit
- [ ] @lit-labs/ssr integration
- [ ] HTML shell templates
- [ ] Component hydration
- [ ] Page routing

### Phase 3: Authentication
- [ ] OAuth routes (Google, GitHub)
- [ ] Session-based auth
- [ ] Protected route middleware
- [ ] User context in SSR

### Phase 4: Real-time Updates
- [ ] NATS client integration
- [ ] SSE endpoint
- [ ] SSE connection manager
- [ ] Client-side SSE handling
- [ ] Delta update processing

### Phase 5: UI Components
- [ ] Web Awesome integration
- [ ] App shell component
- [ ] Grove list/detail pages
- [ ] Agent list/detail pages
- [ ] Status badges and cards

### Phase 6: Terminal
- [ ] xterm.js integration
- [ ] WebSocket PTY connection
- [ ] Terminal theming
- [ ] Resize handling

### Phase 7: Cloud Run Deployment
- [ ] Dockerfile
- [ ] Cloud Run configuration
- [ ] Secret management
- [ ] CI/CD pipeline
- [ ] Monitoring and logging

---

## 12. References

- **Server Implementation:** `server-implementation-design.md`
- **Hub API:** `hub-api.md`
- **Hosted Architecture:** `hosted-architecture.md`
- **Lit Documentation:** https://lit.dev/
- **Web Awesome:** https://webawesome.com/
- **Koa Documentation:** https://koajs.com/
- **@lit-labs/ssr:** https://github.com/lit/lit/tree/main/packages/labs/ssr
- **NATS.js:** https://github.com/nats-io/nats.js
- **xterm.js:** https://xtermjs.org/
