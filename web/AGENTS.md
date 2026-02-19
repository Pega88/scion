# Scion Web Frontend - Agent Instructions

This document provides instructions for AI agents working on the Scion Web Frontend.

## Design Documents

Before making changes, review the relevant design documentation:

- **[Web Frontend Design](../.design/hosted/web-frontend-design.md)** - Architecture, technology stack, component patterns

## Architecture Overview

The web frontend is a **client-side SPA** built with Lit web components. There is no Node.js server at runtime. The Go `scion` binary serves the compiled client assets and handles all server-side concerns (OAuth, sessions, SSE real-time events, API routing) via `pkg/hub/web.go` and `pkg/hub/events.go`.

Node.js and npm are used **only at build time** to compile and bundle client assets via Vite.

## Development Workflow

### Building and Running

```bash
cd web
npm install    # First time only, or after package.json changes

# Build client assets
npm run build

# Run the Go server (from repository root)
scion server start --enable-web --enable-hub --web-port 8080
```

### Using Vite Dev Server

For client-side development with hot module reload:

```bash
npm run dev
```

Note: The Vite dev server only serves client assets. API calls and SSE require the Go server to be running.

### Common Commands

| Command | Purpose |
|---------|---------|
| `npm run dev` | Start Vite dev server with hot reload |
| `npm run build` | Build client assets for production |
| `npm run build:dev` | Build client assets in development mode |
| `npm run lint` | Check for linting errors |
| `npm run lint:fix` | Auto-fix linting errors |
| `npm run format` | Format code with Prettier |
| `npm run typecheck` | Run TypeScript type checking |

### Verifying Changes

After making changes, verify:

1. **Type checking passes:** `npm run typecheck`
2. **Linting passes:** `npm run lint`
3. **Client builds:** `npm run build`

## Project Structure

```
web/
├── src/
│   ├── client/           # Browser-side code
│   │   ├── main.ts       # Client entry point (hydration)
│   │   ├── state.ts      # State manager with SSE subscriptions
│   │   └── sse-client.ts # SSE client for real-time updates
│   ├── components/       # Lit web components
│   │   ├── index.ts      # Component exports
│   │   ├── app-shell.ts  # Main application shell
│   │   ├── shared/       # Reusable UI components
│   │   │   ├── index.ts      # Shared component exports
│   │   │   ├── nav.ts        # Sidebar navigation
│   │   │   ├── header.ts     # Top header bar
│   │   │   ├── breadcrumb.ts # Breadcrumb navigation
│   │   │   └── status-badge.ts # Status indicator badges
│   │   └── pages/        # Page components
│   │       ├── home.ts   # Dashboard page
│   │       └── not-found.ts # 404 page
│   ├── styles/           # CSS theme and utilities
│   │   ├── theme.css     # CSS custom properties, light/dark mode
│   │   └── utilities.css # Utility classes
│   └── shared/           # Shared types between components
│       └── types.ts      # Type definitions
├── public/               # Static assets
│   └── assets/           # CSS, JS, images
├── dist/                 # Build output (gitignored)
└── package.json
```

## Technology Stack

- **Components:** Lit 3.x with decorators
- **UI Library:** Shoelace
- **Build:** Vite for client-side bundling
- **Routing:** @vaadin/router (client-side)
- **Server:** Go (`scion` binary with `--enable-web`)

## Key Patterns

### Creating Lit Components

Components use standard Lit patterns with TypeScript decorators:

```typescript
import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';

@customElement('my-component')
export class MyComponent extends LitElement {
  @property({ type: String })
  myProp = 'default';

  static override styles = css`
    :host { display: block; }
  `;

  override render() {
    return html`<div>${this.myProp}</div>`;
  }
}
```

### Using Shoelace Components

```typescript
render() {
  return html`
    <sl-button variant="primary" @click=${() => this.handleClick()}>
      <sl-icon slot="prefix" name="plus-lg"></sl-icon>
      Create Agent
    </sl-button>

    <sl-badge variant="success">Running</sl-badge>
  `;
}
```

### Theme Variables

Use CSS custom properties with the `--scion-` prefix for consistent theming:

```css
:host {
  background: var(--scion-surface);
  color: var(--scion-text);
  border: 1px solid var(--scion-border);
  border-radius: var(--scion-radius);
}
```

### Dark Mode

Dark mode is handled automatically via CSS custom properties. The theme toggle in the navigation saves the preference to localStorage. Components should use the semantic color variables (e.g., `--scion-surface`, `--scion-text`) which automatically adjust for dark mode.
