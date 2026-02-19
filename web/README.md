# Scion Web Frontend

Browser-based dashboard for managing Scion agents and groves.

## Architecture

The web frontend is a client-side single-page application (SPA) built with Lit web components. Node.js is used only at build time (Vite compiles and bundles client assets). At runtime, the Go `scion` binary serves the compiled assets and handles all server-side concerns (OAuth, sessions, SSE, API routing) via the `--enable-web` flag.

## Prerequisites

- Node.js 20.x or later (build-time only)
- npm 10.x or later
- Go 1.22+ (for running the server)

## Getting Started

### Install Dependencies

```bash
npm install
```

### Build Client Assets

```bash
npm run build
```

### Run the Server

```bash
# From the repository root
scion server start --enable-web --enable-hub --port 9810 --web-port 8080
```

The dashboard will be available at `http://localhost:8080`.

## Available Scripts

| Script | Description |
|--------|-------------|
| `npm run dev` | Start Vite dev server with hot reload |
| `npm run build` | Build client assets for production |
| `npm run build:dev` | Build client assets in development mode |
| `npm run lint` | Run ESLint |
| `npm run lint:fix` | Run ESLint with auto-fix |
| `npm run format` | Format code with Prettier |
| `npm run format:check` | Check code formatting |
| `npm run typecheck` | Run TypeScript type checking |
| `npm run clean` | Remove node_modules, dist, and public/assets |

## Project Structure

```
web/
├── src/
│   ├── client/              # Browser-side code
│   │   ├── main.ts          # Client entry point (hydration)
│   │   ├── state.ts         # State manager with SSE subscriptions
│   │   └── sse-client.ts    # SSE client for real-time updates
│   ├── components/          # Lit web components
│   │   ├── app-shell.ts     # Main application shell
│   │   ├── shared/          # Reusable UI components
│   │   └── pages/           # Page components
│   ├── styles/              # CSS theme and utilities
│   │   ├── theme.css        # CSS custom properties, light/dark mode
│   │   └── utilities.css    # Utility classes
│   └── shared/              # Shared types
│       └── types.ts         # Type definitions
├── public/                  # Static assets (built output copied here)
├── package.json
├── tsconfig.json
└── vite.config.ts
```

## Server Configuration

The Go server is configured via CLI flags and environment variables. See the [Web Dashboard Configuration](/reference/web-config) docs for details.

Key flags:
- `--enable-web` — Enable the web dashboard
- `--web-port` — Port for the web UI (default: 8080)
- `--session-secret` — Secret for signing session cookies (required in production)
