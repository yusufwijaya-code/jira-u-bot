import { Component, computed, inject, input, output, signal } from '@angular/core';
import { CreateMeta, CreateType, IssueCreateService } from '../../../../core/services/issue-create.service';
import { UserPickerComponent } from './user-picker.component';

/** "Satu card" (spec US-5): type → title → description → SP → assignee → parent → required dynamic fields. */
@Component({
  selector: 'app-single-card-form',
  standalone: true,
  imports: [UserPickerComponent],
  templateUrl: './single-card-form.component.html',
})
export class SingleCardFormComponent {
  private api = inject(IssueCreateService);
  boardId = input.required<number>();
  meta = input.required<CreateMeta>();
  created = output<string>();

  protected typeId = signal('');
  protected type = computed<CreateType | undefined>(() => this.meta().types.find((t) => t.id === this.typeId()));
  protected summary = signal('');
  protected description = signal('');
  protected sp = signal<number | null>(null);
  protected assignee = signal<string | null>(null);
  protected parentKey = signal('');
  protected values = signal<Record<string, unknown>>({});
  protected saving = signal(false);
  protected errors = signal<Record<string, string>>({});
  protected formError = signal<string | null>(null);
  protected lastCreated = signal<{ key: string; url: string } | null>(null);

  protected canSubmit = computed(() => {
    const t = this.type();
    if (!t || !t.supported || this.saving() || !this.summary().trim()) return false;
    if (t.subtask && !this.parentKey().trim()) return false;
    return t.fields.every((f) => !f.required || this.filled(this.values()[f.id]));
  });

  private filled(v: unknown): boolean {
    return Array.isArray(v) ? v.length > 0 : v !== undefined && v !== null && String(v).trim() !== '';
  }

  protected setType(id: string): void { this.typeId.set(id); this.values.set({}); this.errors.set({}); }
  protected setValue(id: string, v: unknown): void { this.values.update((m) => ({ ...m, [id]: v })); }
  protected onMulti(id: string, event: Event): void {
    this.setValue(id, Array.from((event.target as HTMLSelectElement).selectedOptions).map((o) => o.value));
  }

  protected submit(): void {
    const t = this.type();
    if (!t || !this.canSubmit()) return;
    this.saving.set(true);
    this.errors.set({});
    this.formError.set(null);
    this.api.createIssue({
      boardId: this.boardId(), typeId: t.id, summary: this.summary().trim(), description: this.description(),
      storyPoints: t.storyPointField ? this.sp() : null, assigneeAccountId: this.assignee(),
      parentKey: this.parentKey().trim() || null, fields: this.values(),
    }).subscribe({
      next: (res) => { this.saving.set(false); this.lastCreated.set(res); this.created.emit(res.key); },
      error: (err) => {
        this.saving.set(false);
        const body = err?.error ?? {};
        if (body.fields) this.errors.set(body.fields);
        this.formError.set(body.messages?.join(' ') || body.message || body.error || 'Gagal membuat card');
      },
    });
  }

  /** "Buat lagi": keep type & parent, clear the rest. */
  protected again(): void {
    this.summary.set(''); this.description.set(''); this.sp.set(null); this.values.set({}); this.lastCreated.set(null);
  }
}
