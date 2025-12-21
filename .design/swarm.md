# Gemini Swarm (gswarm) Design Specification

This document details the design for `gswarm`, a container-based orchestration tool for managing concurrent Gemini CLI agents. The system enables parallel execution of specialized sub-agents with isolated identities, credentials, and workspaces.

## System Goals

- **Parallelism**: Run multiple agents concurrently as independent processes.
- **Isolation**: Ensure strict separation of identities, credentials (e.g., `gcloud`), and configuration.
- **Context Management**: Provide each agent with a dedicated git worktree to prevent conflicts.
- **Specialization**: Support role-based agent configuration via templates (e.g., "Security Auditor", "QA Tester").
- **Interactivity**: Support "detached" background operation with the ability for a user to "attach" for human-in-the-loop interaction.

## Architecture Overview

The system follows a Manager-Worker architecture:
- **Swarm Manager (`gswarm`)**: A host-side CLI that orchestrates the lifecycle of agents.
- **Swarm Workers**: Isolated containers running the Gemini CLI, acting as independent agents.

### 1. Swarms & Contexts

A **Swarm** is the top-level logical container for a group of agents.

- **Project Swarm**: Linked to a project directory. If the directory is a git repository, additional features like automatic worktree management become available. The swarm name defaults to the directory name (e.g., `my-app`).
- **Playground Swarm**: A default global swarm (`playground`) for ad-hoc agents not tied to a specific project.

### 2. Agent Templates

Agents are provisioned using **Templates**, which define their persona, capabilities, and tools.

- **Storage**:
  - Project templates: `.gswarm/templates/` (checked into the repo).
  - Global templates: `~/.gswarm/templates/`.
- **Structure**: A template directory mirrors the agent's home directory structure (`/home/gemini`), with an additional `gswarm.json` for manager-level configuration.
  ```text
  template-name/
  ├── gswarm.json             # gswarm-specific config (e.g., container image)
  ├── .gemini/
  │   ├── settings.json       # Allowed tools, MCP servers, active extensions
  │   ├── system_prompt.md    # Agent persona and behavioral instructions
  │   └── gemini.md           # Initial context
  ├── .config/
  │   └── gcloud/             # Optional: Pre-seeded credentials
  └── .bashrc                 # Optional: Shell aliases
  ```
- **Inheritance**: A `default` template acts as the base. New agents inherit from `default` unless a specific type is requested. Files in the custom template overwrite those in `default` (e.g., `gswarm.json` in a custom template overrides the image).

### 3. Swarm Manager CLI (`gswarm`)

The `gswarm` tool manages the lifecycle of swarms and agents.

**Core Commands:**
- `gswarm init`: Initialize `.gswarm/` structure in the current project.
- `gswarm start <task> --name <agent> [--type <template>]`: Provision and launch a new agent.
- `gswarm list`: Show running agents, their status, and assigned swarm.
- `gswarm attach <agent>`: Connect the host TTY to the agent's container session.
- `gswarm stop <agent>`: Gracefully terminate an agent and cleanup resources.

**Template Management:**
- `gswarm templates list`: Show available templates.
- `gswarm templates create <name>`: Create a new template derived from default.
- `gswarm templates extensions install <ext-id> --template <name>`: Add an extension to a template.

### 4. Resource Isolation

Each agent runs in a dedicated container with strictly isolated resources.

- **Filesystem**:
  - **Host Path**: `.gswarm/agents/<agent-name>/home` (Project) or `~/.gswarm/agents/...` (Playground).
  - **Container Mount**: `/home/gemini`.
  - **Contents**: Populated from the template at startup. Includes unique `settings.json`, `.config/gcloud`, and persistent `.gemini/history`.
- **Network**:
  - Agents share the `gemini-cli-sandbox` bridge network but are otherwise isolated.

### 5. Workspace Strategy (Git Worktrees)

To allow concurrent modification of the codebase without conflicts, `gswarm` uses `git worktree`.

1.  **Creation**: On `start`, the Manager creates a new worktree on the host.
    - Path: `../.gswarm_worktrees/<swarm>/<agent>` (kept outside the main worktree to avoid recursion).
    - Branch: Creates a new feature branch for the agent.
2.  **Mounting**: The worktree is mounted to `/workspace` inside the container.
3.  **Sync**: The shared `.git` directory ensures all agents see the same repository history, while working directories remain independent.

### 6. Runtime & Execution

Agents run as **detached containers** with allocated TTYs.

- **Launch Command**:
  The platform-specific runtime (`container` on macOS, `docker` on Linux) is used:
  ```bash
  RUNTIME run -d -t \
    --name <swarm>-<agent> \
    -v <host_home_path>:/home/gemini \
    -v <host_worktree_path>:/workspace \
    gemini-cli-image
  ```
- **Platform Constraints (macOS)**:
  - The Apple `container` CLI has a limitation where the **same host source directory** cannot be mounted to multiple destinations (causes VirtioFS tag conflicts).
  - **Design Compliance**: `gswarm` adheres to this by ensuring `<host_home_path>` and `<host_worktree_path>` are always distinct, non-overlapping directories on the host.
- **"Yolo" Mode**: Configurable via `settings.json` or CLI flag. Enables the agent to execute tools without requiring user confirmation for every step.

### 7. Observability & Human-in-the-Loop

The system provides visibility into agent states and facilitates intervention.

- **Status Mechanism**:
  - Agents write their state to a file: `/home/gemini/.gemini-status.json`.
  - **States**: `STARTING`, `THINKING`, `EXECUTING`, `WAITING_FOR_INPUT`, `COMPLETED`, `ERROR`.
- **Intervention Loop**:
  1.  Agent hits a tool requiring confirmation.
  2.  Agent updates status to `WAITING_FOR_INPUT`.
  3.  Manager polls status and alerts the user (via `list` or notification).
  4.  User runs `gswarm attach <agent>` to take control.
  5.  User provides input/confirmation and detaches (Ctrl-P, Ctrl-Q).
  6.  Agent resumes `EXECUTING`.
