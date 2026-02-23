/**
 * Grove creation page component
 *
 * Form for creating a new grove from a git repository URL
 */

import { LitElement, html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';

import '../shared/status-badge.js';

@customElement('scion-page-grove-create')
export class ScionPageGroveCreate extends LitElement {
  @state()
  private submitting = false;

  @state()
  private error: string | null = null;

  @state()
  private gitUrl = '';

  @state()
  private name = '';

  @state()
  private nameManuallySet = false;

  @state()
  private branch = 'main';

  static override styles = css`
    :host {
      display: block;
    }

    .back-link {
      display: inline-flex;
      align-items: center;
      gap: 0.5rem;
      color: var(--scion-text-muted, #64748b);
      text-decoration: none;
      font-size: 0.875rem;
      margin-bottom: 1rem;
    }

    .back-link:hover {
      color: var(--scion-primary, #3b82f6);
    }

    .page-header {
      margin-bottom: 1.5rem;
    }

    .page-header h1 {
      font-size: 1.5rem;
      font-weight: 700;
      color: var(--scion-text, #1e293b);
      margin: 0 0 0.25rem 0;
      display: flex;
      align-items: center;
      gap: 0.75rem;
    }

    .page-header h1 sl-icon {
      color: var(--scion-primary, #3b82f6);
      font-size: 1.5rem;
    }

    .page-header p {
      color: var(--scion-text-muted, #64748b);
      margin: 0;
      font-size: 0.875rem;
    }

    .form-card {
      background: var(--scion-surface, #ffffff);
      border: 1px solid var(--scion-border, #e2e8f0);
      border-radius: var(--scion-radius-lg, 0.75rem);
      padding: 1.5rem;
      max-width: 640px;
    }

    .form-field {
      margin-bottom: 1.25rem;
    }

    .form-field label {
      display: block;
      font-size: 0.875rem;
      font-weight: 600;
      color: var(--scion-text, #1e293b);
      margin-bottom: 0.375rem;
    }

    .form-field .hint {
      font-size: 0.75rem;
      color: var(--scion-text-muted, #64748b);
      margin-top: 0.25rem;
    }

    .form-field sl-input {
      width: 100%;
    }

    .form-actions {
      display: flex;
      gap: 0.75rem;
      margin-top: 1.5rem;
      padding-top: 1.5rem;
      border-top: 1px solid var(--scion-border, #e2e8f0);
    }

    .error-banner {
      background: var(--sl-color-danger-50, #fef2f2);
      border: 1px solid var(--sl-color-danger-200, #fecaca);
      border-radius: var(--scion-radius, 0.5rem);
      padding: 0.75rem 1rem;
      margin-bottom: 1.25rem;
      display: flex;
      align-items: flex-start;
      gap: 0.5rem;
      color: var(--sl-color-danger-700, #b91c1c);
      font-size: 0.875rem;
    }

    .error-banner sl-icon {
      flex-shrink: 0;
      margin-top: 0.125rem;
    }
  `;

  /**
   * Extract a display name from a git URL.
   * Handles HTTPS and SSH formats.
   */
  private deriveNameFromUrl(url: string): string {
    try {
      // Strip trailing .git
      const cleaned = url.trim().replace(/\.git$/, '');

      // SSH format: git@github.com:org/repo
      const sshMatch = cleaned.match(/[:/]([^/:]+)$/);
      if (sshMatch) {
        return sshMatch[1];
      }

      // HTTPS format: https://github.com/org/repo
      const parts = cleaned.split('/');
      return parts[parts.length - 1] || '';
    } catch {
      return '';
    }
  }

  private onGitUrlInput(e: Event): void {
    this.gitUrl = (e.target as HTMLElement & { value: string }).value;

    // Auto-derive name if user hasn't manually set it
    if (!this.nameManuallySet) {
      this.name = this.deriveNameFromUrl(this.gitUrl);
    }
  }

  private onNameInput(e: Event): void {
    const value = (e.target as HTMLElement & { value: string }).value;
    this.name = value;
    this.nameManuallySet = value.length > 0;
  }

  private async handleSubmit(_e: Event): Promise<void> {
    const trimmedUrl = this.gitUrl.trim();

    if (!trimmedUrl) {
      this.error = 'Git repository URL is required.';
      return;
    }

    // Use derived name if none provided
    const displayName = this.name.trim() || this.deriveNameFromUrl(trimmedUrl);
    if (!displayName) {
      this.error = 'Could not determine a name from the URL. Please provide a display name.';
      return;
    }

    this.submitting = true;
    this.error = null;

    try {
      const labels: Record<string, string> = {
        'scion.dev/default-branch': this.branch.trim() || 'main',
        'scion.dev/clone-url': trimmedUrl,
        'scion.dev/source-url': trimmedUrl,
      };

      const body = {
        name: displayName,
        gitRemote: trimmedUrl,
        labels,
      };

      const response = await fetch('/api/v1/groves', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });

      if (!response.ok) {
        const errorData = (await response.json().catch(() => ({}))) as {
          message?: string;
          error?: string;
        };
        throw new Error(errorData.message || errorData.error || `HTTP ${response.status}`);
      }

      const result = (await response.json()) as { grove?: { id: string }; id?: string };
      const groveId = result.grove?.id || result.id;

      if (!groveId) {
        throw new Error('No grove ID in response');
      }

      // Navigate to grove detail page
      window.history.pushState({}, '', `/groves/${groveId}`);
      window.dispatchEvent(new PopStateEvent('popstate'));
    } catch (err) {
      console.error('Failed to create grove:', err);
      this.error = err instanceof Error ? err.message : 'Failed to create grove';
    } finally {
      this.submitting = false;
    }
  }

  override render() {
    return html`
      <a href="/groves" class="back-link">
        <sl-icon name="arrow-left"></sl-icon>
        Back to Groves
      </a>

      <div class="page-header">
        <h1>
          <sl-icon name="folder-plus"></sl-icon>
          Create Grove
        </h1>
        <p>Create a new grove from a git repository.</p>
      </div>

      <div class="form-card">
        ${this.error
          ? html`
              <div class="error-banner">
                <sl-icon name="exclamation-triangle"></sl-icon>
                <span>${this.error}</span>
              </div>
            `
          : ''}

        <div>
          <div class="form-field">
            <label for="git-url">Git Repository URL</label>
            <sl-input
              id="git-url"
              placeholder="https://github.com/org/repo.git"
              .value=${this.gitUrl}
              @sl-input=${(e: Event) => this.onGitUrlInput(e)}
              required
            ></sl-input>
            <div class="hint">HTTPS or SSH URL of the git repository.</div>
          </div>

          <div class="form-field">
            <label for="name">Display Name</label>
            <sl-input
              id="name"
              placeholder="Auto-derived from URL"
              .value=${this.name}
              @sl-input=${(e: Event) => this.onNameInput(e)}
            ></sl-input>
            <div class="hint">
              Human-readable name for this grove. Auto-derived from the repository URL if left
              empty.
            </div>
          </div>

          <div class="form-field">
            <label for="branch">Default Branch</label>
            <sl-input
              id="branch"
              placeholder="main"
              .value=${this.branch}
              @sl-input=${(e: Event) => {
                this.branch = (e.target as HTMLElement & { value: string }).value;
              }}
            ></sl-input>
            <div class="hint">The default branch to use for this repository.</div>
          </div>

          <div class="form-actions">
            <sl-button
              variant="primary"
              ?loading=${this.submitting}
              ?disabled=${this.submitting}
              @click=${(e: Event) => this.handleSubmit(e)}
            >
              <sl-icon slot="prefix" name="folder-plus"></sl-icon>
              Create Grove
            </sl-button>
            <a href="/groves" style="text-decoration: none;">
              <sl-button variant="default" ?disabled=${this.submitting}> Cancel </sl-button>
            </a>
          </div>
        </div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'scion-page-grove-create': ScionPageGroveCreate;
  }
}
