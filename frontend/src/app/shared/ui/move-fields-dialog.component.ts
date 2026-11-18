import { Component, HostListener, computed, effect, inject, signal, untracked, viewChildren } from '@angular/core';
import { DashboardService } from '../../core/services/dashboard.service';
import { MoveFieldsService, MoveRequirement, fieldsRequired } from '../../core/ui/move-fields.service';
import { IssueTypeIconComponent } from './issue-type-icon.component';
import { IconComponent } from './icon.component';
import { JiraFieldInputComponent, fieldFilled } from './jira-field-input.component';

/**
 * "Lengkapi dulu sebelum pindah ke <Status>": the fields a Jira move requires,
 * rendered with the Buat Card controls. "Simpan & pindahkan" saves them and
 * moves in one request; server errors show inline. Mounted once in
 * <app-overlays>, opened through MoveFieldsService.
 */
@Component({
  selector: 'app-move-fields-dialog',
  standalone: true,
  imports: [IconComponent, JiraFieldInputComponent, IssueTypeIconComponent],
  template: `
    @if (req(); as r) {
      <div class="modal-backdrop" (click)="cancel()">
        <div class="modal mf-modal" role="dialog" aria-modal="true" aria-labelledby="mf-title" data-move-fields (click)="$event.stopPropagation()">
          <div class="mf-head">
            <div class="modal-icon mf-icon"><app-icon name="edit" [size]="20" /></div>
            <div class="mf-titles">
              <h3 id="mf-title">Lengkapi dulu sebelum pindah ke {{ r.transition.to }}</h3>
              <p class="mf-key"><span class="ticket-id"><app-issue-type-icon [name]="r.issueType" [iconUrl]="r.issueTypeIconUrl" /><span data-move-key>{{ r.issueKey }}</span></span></p>
            </div>
          </div>
          @if (r.message) { <p class="mf-message" data-move-message>{{ r.message }}</p> }
          <form class="mf-form" (submit)="$event.preventDefault(); submit()">
            <div class="mf-fields">
              @for (f of r.fields; track f.id) {
                <app-jira-field-input [field]="f" [value]="values()[f.id]" [error]="errors()[f.id]" [issueUrl]="r.issueUrl ?? ''"
                  (valueChange)="setValue(f.id, $event)" />
              } @empty {
                <p class="mf-none">Tidak ada field yang perlu diisi lagi.</p>
              }
            </div>
            @if (formError()) { <p class="cc-err mf-error" role="alert" data-move-error>{{ formError() }}</p> }
            <div class="modal-actions">
              <button type="button" class="btn btn-ghost" data-move-cancel (click)="cancel()">Batal</button>
              <button type="submit" class="btn btn-primary" data-move-submit [disabled]="!canSubmit()">
                @if (saving()) { <span class="mf-spin" aria-hidden="true"></span> Menyimpan… } @else { Simpan &amp; pindahkan }
              </button>
            </div>
            @if (blockedByJira()) {
              <p class="mf-hint">Ada field yang harus diisi di Jira dulu.
                @if (r.issueUrl) { <a [href]="r.issueUrl" target="_blank" rel="noopener">Buka di Jira</a> }
              </p>
            }
          </form>
        </div>
      </div>
    }
  `,
  styles: `
    .mf-modal { width: min(520px, 100%); max-height: calc(100dvh - 32px); overflow-y: auto; text-align: left; }
    .mf-head { display: flex; align-items: center; gap: 14px; }
    .mf-icon { flex: none; width: 44px; height: 44px; margin: 0; border-radius: 14px; }
    .mf-titles { min-width: 0; h3 { margin: 0; font-size: 1.02rem; line-height: 1.35; } }
    .mf-key { margin: 2px 0 0 !important; font-size: .8rem !important; font-weight: 600; color: var(--text-faint) !important; }
    .mf-message { margin: 14px 0 0 !important; padding: 10px 12px; border-radius: var(--radius-sm); background: var(--surface-2); border: 1px solid var(--border);
      color: var(--text) !important; line-height: 1.5; }
    .mf-form { display: flex; flex-direction: column; margin-top: 16px; }
    .mf-fields { display: flex; flex-direction: column; gap: 14px;
      > * { animation: fade-up 240ms var(--ease) backwards; }
      @for $i from 1 through 6 { > :nth-child(#{$i}) { animation-delay: #{($i - 1) * 40}ms; } } }
    .mf-none { margin: 0; font-size: .85rem; color: var(--text-muted); }
    .mf-error { margin: 12px 0 0 !important; color: var(--error) !important; font-size: .82rem !important; animation: fade-in 200ms var(--ease); }
    .mf-hint { margin: 10px 0 0 !important; font-size: .8rem !important; text-align: center; a { font-weight: 600; } }
    .mf-spin { display: inline-block; width: 12px; height: 12px; vertical-align: -2px; border-radius: 50%;
      border: 2px solid currentColor; border-right-color: transparent; animation: spin .7s linear infinite; }
    @media (prefers-reduced-motion: reduce) { .mf-fields > *, .mf-error { animation: none; } .mf-spin { animation-duration: 1.5s; } }
  `,
})
export class MoveFieldsDialogComponent {
  protected service = inject(MoveFieldsService);
  private dashboard = inject(DashboardService);
  private inputs = viewChildren(JiraFieldInputComponent);

  /** The requirement on screen; replaced when the server asks for more fields. */
  protected req = signal<MoveRequirement | null>(null);
  protected values = signal<Record<string, unknown>>({});
  protected errors = signal<Record<string, string>>({});
  protected formError = signal<string | null>(null);
  protected saving = signal(false);

  protected blockedByJira = computed(() => (this.req()?.fields ?? []).some((f) => f.kind === 'unsupported'));
  protected canSubmit = computed(() => {
    const r = this.req();
    if (!r || this.saving() || this.blockedByJira()) return false;
    return r.fields.every((f) => fieldFilled(this.values()[f.id]));
  });

  constructor() {
    effect(() => {
      const pending = this.service.current();
      untracked(() => {
        this.req.set(pending?.req ?? null);
        this.values.set({});
        this.errors.set({});
        this.formError.set(null);
        this.saving.set(false);
      });
    });
  }

  protected setValue(id: string, v: unknown): void {
    this.values.update((m) => ({ ...m, [id]: v }));
    if (this.errors()[id]) this.errors.update(({ [id]: _, ...rest }) => rest);
  }

  @HostListener('document:keydown.escape')
  protected cancel(): void {
    if (!this.req() || this.saving()) return;
    this.service.close(null);
  }

  protected submit(): void {
    const r = this.req();
    for (const i of this.inputs()) i.commitLabel();
    if (!r || !this.canSubmit()) return;
    this.saving.set(true);
    this.errors.set({});
    this.formError.set(null);
    const fields: Record<string, unknown> = {};
    for (const f of r.fields) fields[f.id] = this.values()[f.id];
    this.dashboard.moveTicket(r.issueKey, r.transition.id, fields).subscribe({
      next: (res) => {
        this.saving.set(false);
        this.service.close({ message: res?.message ?? '', status: res?.status ?? r.transition.to });
      },
      error: (err) => {
        this.saving.set(false);
        const more = fieldsRequired(err);
        if (more) {
          // Jira wants something else too: show what is still missing.
          this.req.set({ ...more, issueKey: more.issueKey || r.issueKey, issueUrl: more.issueUrl || r.issueUrl, issueType: r.issueType, issueTypeIconUrl: r.issueTypeIconUrl });
          this.formError.set(more.fields.length ? null : more.message || 'Jira menolak perpindahan ini.');
          return;
        }
        const body = err?.error ?? {};
        if (body.error === 'invalid_fields' && body.fields) {
          this.errors.set(body.fields);
          this.formError.set(body.message ?? null);
          return;
        }
        this.formError.set(typeof body.error === 'string' ? body.error : 'Gagal menyimpan, coba lagi.');
      },
    });
  }
}
