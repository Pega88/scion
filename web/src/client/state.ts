/**
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/**
 * Client-side state manager with view-scoped SSE subscriptions
 *
 * The StateManager uses view-scoped subscriptions: the subscription scope
 * follows navigation, not individual entities. A paginated list of 200 agents
 * uses one grove-level subscription, not 200 agent-level subscriptions.
 * Pagination is a rendering concern; the full state map is maintained in memory.
 *
 * See web-frontend-design.md §4.4 and §12.2.
 */

import { SSEClient } from './sse-client.js';
import type { SSEUpdateEvent } from './sse-client.js';
import type { Agent, Grove } from '../shared/types.js';

/** Subscription scope matches view context */
export type ViewScope =
  | { type: 'dashboard' }
  | { type: 'grove'; groveId: string }
  | { type: 'agent-detail'; groveId: string; agentId: string };

/** Full in-memory state for the current scope */
export interface AppState {
  agents: Map<string, Agent>;
  groves: Map<string, Grove>;
  connected: boolean;
  scope: ViewScope | null;
}

/** Events dispatched by StateManager */
export type StateEventType =
  | 'agents-updated'
  | 'groves-updated'
  | 'connected'
  | 'disconnected'
  | 'scope-changed';

export class StateManager extends EventTarget {
  private state: AppState = {
    agents: new Map(),
    groves: new Map(),
    connected: false,
    scope: null,
  };

  private sseClient = new SSEClient();

  constructor() {
    super();

    // Wire SSE client events to state management
    this.sseClient.addEventListener('update', ((event: CustomEvent<SSEUpdateEvent>) => {
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

  /**
   * Initialize state from server-rendered data.
   * Called once on page load with the __SCION_DATA__ payload.
   */
  hydrate(initialData: { agents?: Agent[]; groves?: Grove[] }): void {
    if (initialData.agents) {
      for (const agent of initialData.agents) {
        this.state.agents.set(agent.id, agent);
      }
    }

    if (initialData.groves) {
      for (const grove of initialData.groves) {
        this.state.groves.set(grove.id, grove);
      }
    }
  }

  /**
   * Set the view scope. Closes any existing SSE connection and opens
   * a new one with subjects matching the view context.
   * Called by the router on navigation.
   */
  setScope(scope: ViewScope): void {
    // Skip if scope is unchanged
    if (this.state.scope && this.scopeEquals(this.state.scope, scope)) {
      return;
    }

    this.state.scope = scope;

    // Clear state from previous scope
    this.state.agents.clear();
    this.state.groves.clear();

    const subjects = this.subjectsForScope(scope);
    if (subjects.length > 0) {
      this.sseClient.connect(subjects);
    }

    this.notify('scope-changed');
  }

  /**
   * Map view scope to event subject patterns.
   * Matches the subscription tiers defined in §12.2.
   */
  private subjectsForScope(scope: ViewScope): string[] {
    switch (scope.type) {
      case 'dashboard':
        // Aggregate stats per grove (lightweight)
        return ['grove.*.summary'];

      case 'grove':
        // Grove-level wildcard: all lightweight/medium events for agents in this grove
        return [`grove.${scope.groveId}.>`];

      case 'agent-detail':
        // Keep grove subscription for breadcrumb/sidebar freshness.
        // Add agent-specific subscription for heavy events (harness output).
        return [`grove.${scope.groveId}.>`, `agent.${scope.agentId}.>`];
    }
  }

  private scopeEquals(a: ViewScope, b: ViewScope): boolean {
    if (a.type !== b.type) return false;
    if (a.type === 'dashboard' && b.type === 'dashboard') return true;
    if (a.type === 'grove' && b.type === 'grove') return a.groveId === b.groveId;
    if (a.type === 'agent-detail' && b.type === 'agent-detail') {
      return a.groveId === b.groveId && a.agentId === b.agentId;
    }
    return false;
  }

  /**
   * Handle delta updates from SSE.
   * The server sends events with structure: { subject: string, data: unknown }
   * Subject format follows the event schema in §12.3.
   */
  private handleUpdate(update: SSEUpdateEvent): void {
    const { subject, data } = update;
    const parts = subject.split('.');

    // Agent-scoped events: agent.{agentId}.{eventType}
    if (parts[0] === 'agent' && parts.length >= 3) {
      const agentId = parts[1];
      const eventType = parts[2];
      this.handleAgentEvent(agentId, eventType, data);
      return;
    }

    // Grove-scoped events
    if (parts[0] === 'grove' && parts.length >= 3) {
      const groveId = parts[1];

      // Grove agent events: grove.{groveId}.agent.{eventType}
      if (parts[2] === 'agent' && parts.length >= 4) {
        const eventType = parts[3];
        const agentData = data as Record<string, unknown>;
        const agentId = agentData.agentId as string;
        if (agentId) {
          this.handleAgentEvent(agentId, eventType, data);
        }
        return;
      }

      // Grove broker events: grove.{groveId}.broker.{eventType}
      if (parts[2] === 'broker') {
        // Broker events don't affect agent/grove state maps currently
        return;
      }

      // Grove metadata events: grove.{groveId}.updated or grove.*.summary
      this.handleGroveEvent(groveId, parts[2], data);
    }
  }

  private handleAgentEvent(agentId: string, eventType: string, data: unknown): void {
    if (eventType === 'deleted') {
      this.state.agents.delete(agentId);
    } else {
      // Merge delta into existing agent state
      const existing = this.state.agents.get(agentId) || ({} as Agent);
      const delta = data as Partial<Agent>;
      // Ensure id is always set
      const updated = { ...existing, ...delta, id: agentId };
      this.state.agents.set(agentId, updated as Agent);
    }
    this.notify('agents-updated');
  }

  private handleGroveEvent(groveId: string, eventType: string, data: unknown): void {
    if (eventType === 'summary') {
      // Dashboard summary event: grove.*.summary
      const summaryData = data as Partial<Grove> & { groveId?: string };
      const id = summaryData.groveId || groveId;
      const existing = this.state.groves.get(id) || ({} as Grove);
      const updated = { ...existing, ...summaryData, id };
      this.state.groves.set(id, updated as Grove);
    } else if (eventType === 'updated') {
      // Grove metadata change: grove.{groveId}.updated
      const existing = this.state.groves.get(groveId) || ({} as Grove);
      const updated = { ...existing, ...(data as Partial<Grove>), id: groveId };
      this.state.groves.set(groveId, updated as Grove);
    }
    this.notify('groves-updated');
  }

  private notify(event: StateEventType): void {
    this.dispatchEvent(new CustomEvent(event, { detail: this.state }));
  }

  /** Disconnect the SSE connection. Called on page unload. */
  disconnect(): void {
    this.sseClient.disconnect();
    this.state.connected = false;
  }

  // --- Getters ---
  // The full state map is maintained regardless of pagination.
  // Components render the slice they need.

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

  get isConnected(): boolean {
    return this.state.connected;
  }

  get currentScope(): ViewScope | null {
    return this.state.scope;
  }
}

/** Singleton instance — accessed via import */
export const stateManager = new StateManager();
