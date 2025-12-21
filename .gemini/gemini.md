# Gemini Swarm (gswarm) Project Context

## Overview
`gswarm` is a container-based orchestration tool designed to manage concurrent Gemini CLI agents. It provides isolation, parallelism, and context management (via git worktrees) for multiple specialized agents.

## Core Technologies
- **Language**: Go (Golang)
- **CLI Framework**: [Cobra](https://github.com/spf13/cobra)
- **Isolation**: Containers (Apple `container` CLI on macOS, Docker on Linux)
- **Workspace**: Git Worktrees

## Project Structure
- `cmd/`: CLI command definitions (using Cobra).
- `.design/`: Design specifications and architectural documents.
  - `swarm.md`: Primary design spec.
  - `apple-container.md`: Notes on macOS container runtime.
- `.gemini/`: Project-specific Gemini context.

## Current State
- Project initialized with Go modules and Cobra.
- Root command `gswarm` configured with basic help and description.
- Initial Git repository created.

## Development Guidelines
- Follow idiomatic Go patterns.
- Adhere to the Manager-Worker architecture defined in `.design/swarm.md`.
- Ensure all new commands are added via Cobra in the `cmd/` package.
- **Do not update the local `.gswarm` folder directly.** It is populated during initialization (`gswarm init`).
- **Any changes to the default template must be made in `pkg/config/init.go`** (the config init package), as that is where the seeding logic resides.
