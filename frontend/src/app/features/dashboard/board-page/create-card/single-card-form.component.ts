import { Component, computed, inject, input, output, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { CascadeValue, CreateField, CreateMeta, CreateType, IssueCreateService } from '../../../../core/services/issue-create.service';
import { DateRangePickerComponent } from '../../../../shared/ui/date-range-picker.component';
import { SelectComponent, SelectOption } from '../../../../shared/ui/select.component';
import { IconComponent } from '../../../../shared/ui/icon.component';
import { UserPickerComponent } from './user-picker.component';
import { ParentPickerComponent } from './parent-picker.component';

/**
 * "Satu card" (spec US-5). Two columns on wide screens: main (type, title,
 * description) and a "Detail" panel (SP, assignee, parent, dynamic fields).
 * A subtask type picks its parent with the parent picker (epics are filtered
 * out); other types that allow a parent keep a plain key input (Epic link).
 * Dynamic fields (required by Jira or picked in Aturan Board) render per kind;
 * textarea kinds sit in the main column under Deskripsi, the rest in Detail.
 */
@Component({
  selector: 'app-single-card-form',
  standalone: true,
  imports: [FormsModule, SelectComponent, UserPickerComponent, ParentPickerComponent, IconComponent, DateRangePickerComponent],
  templateUrl: './single-card-form.component.html',
  styleUrl: './single-card-form.component.scss',
})
export class SingleCardFormComponent {
  private api = inject(IssueCreateService);
  boardId = input.required<number>();
  meta = input.required<CreateMeta>();
  created = output<string>();

  protected typeId = signal('');
  protected type = computed<CreateType | undefined>(() => (this.meta().types ?? []).find((t) => t.id === this.typeId()));
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
  /** Search text of long multi-select chip lists, per field. */
  protected multiFilter = signal<Record<string, string | undefined>>({});
  /** Label being typed, per labels field. */
  protected labelDraft = signal<Record<string, string | undefined>>({});

  protected mainFields = computed(() => (this.type()?.fields ?? []).filter((f) => f.kind === 'textarea'));
  protected sideFields = computed(() => (this.type()?.fields ?? []).filter((f) => f.kind !== 'textarea'));

  protected canSubmit = computed(() => {
    const t = this.type();
    if (!t || !t.supported || this.saving() || this.lastCreated() || !this.summary().trim()) return false;
    if (t.subtask && !this.parentKey().trim()) return false;
    return t.fields.every((f) => !f.required || this.filled(this.values()[f.id]));
  });

  /** Why "Buat card" is disabled, shown next to the button. */
  protected submitHint = computed(() => {
    const t = this.type();
    if (!t) return '';
    if (!t.supported) return 'Tipe ini tidak didukung';
    if (!this.summary().trim()) return 'Isi judul dulu';
    if (t.subtask && !this.parentKey().trim()) return 'Pilih parent dulu';
    const missing = t.fields.find((f) => f.required && !this.filled(this.values()[f.id]));
    return missing ? `${missing.name} wajib diisi` : '';
  });

  private filled(v: unknown): boolean {
    if (Array.isArray(v)) return v.length > 0;
    if (v && typeof v === 'object') return !!(v as CascadeValue).id;
    return v !== undefined && v !== null && String(v).trim() !== '';
  }

  /** Chip lists longer than this get a search box and a scrollable wrap. */
  protected readonly longList = 12;

  protected typeOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Pilih tipe…' },
    ...(this.meta().types ?? []).map((t) => ({
      value: t.id,
      label: `${t.name}${t.subtask ? ' (subtask)' : ''}${t.supported ? '' : ' — tidak didukung'}`,
    })),
  ]);

  protected fieldOptions(f: CreateField): SelectOption<string>[] {
    return [{ value: '', label: 'Pilih…' }, ...(f.options ?? []).map((o) => ({ value: o.id, label: o.label }))];
  }

  protected text(fieldId: string): string {
    const v = this.values()[fieldId];
    return typeof v === 'string' ? v : '';
  }

  protected cascade(fieldId: string): CascadeValue | null {
    const v = this.values()[fieldId] as CascadeValue | undefined;
    return v && typeof v === 'object' && v.id ? v : null;
  }
  protected childOptions(f: CreateField): SelectOption<string>[] {
    const parent = (f.options ?? []).find((o) => o.id === this.cascade(f.id)?.id);
    return parent?.children?.length ? [{ value: '', label: 'Pilih…' }, ...parent.children.map((c) => ({ value: c.id, label: c.label }))] : [];
  }
  /** A new parent drops the child (children belong to one parent). */
  protected setCascadeParent(f: CreateField, id: string): void { this.setValue(f.id, id ? { id } : null); }
  protected setCascadeChild(f: CreateField, child: string): void {
    const cur = this.cascade(f.id);
    if (cur) this.setValue(f.id, child ? { id: cur.id, child } : { id: cur.id });
  }

  protected visibleOptions(f: CreateField) {
    const q = (this.multiFilter()[f.id] ?? '').trim().toLowerCase();
    const all = f.options ?? [];
    return q ? all.filter((o) => o.label.toLowerCase().includes(q) || this.isSelected(f.id, o.id)) : all;
  }
  protected selectedCount(fieldId: string): number {
    const v = this.values()[fieldId];
    return Array.isArray(v) ? v.length : 0;
  }
  protected setFilter(fieldId: string, q: string): void { this.multiFilter.update((m) => ({ ...m, [fieldId]: q })); }

  protected labels(fieldId: string): string[] {
    const v = this.values()[fieldId];
    return Array.isArray(v) ? (v as string[]) : [];
  }
  protected setLabelDraft(fieldId: string, input: HTMLInputElement): void {
    // Jira labels have no spaces: a space or comma commits what was typed.
    const parts = input.value.split(/[\s,]+/);
    const last = parts.pop() ?? '';
    if (parts.length) input.value = last;
    for (const p of parts) this.addLabel(fieldId, p);
    this.labelDraft.update((m) => ({ ...m, [fieldId]: last }));
  }
  protected commitLabel(fieldId: string, event?: Event): void {
    const draft = (this.labelDraft()[fieldId] ?? '').trim();
    if (!draft) return;
    event?.preventDefault();
    this.addLabel(fieldId, draft);
    this.labelDraft.update((m) => ({ ...m, [fieldId]: '' }));
  }
  protected removeLabel(fieldId: string, label: string): void { this.setValue(fieldId, this.labels(fieldId).filter((l) => l !== label)); }
  private addLabel(fieldId: string, label: string): void {
    const l = label.trim();
    if (l && !this.labels(fieldId).includes(l)) this.setValue(fieldId, [...this.labels(fieldId), l]);
  }

  protected isSelected(fieldId: string, optionId: string): boolean {
    const v = this.values()[fieldId];
    return Array.isArray(v) ? v.includes(optionId) : v === optionId;
  }

  protected setType(id: string): void {
    const wasSubtask = !!this.type()?.subtask;
    this.typeId.set(id);
    // A picked subtask parent is not an Epic link (and vice versa): start the parent over.
    if (wasSubtask !== !!this.type()?.subtask) this.parentKey.set('');
    this.values.set({});
    this.errors.set({});
    this.multiFilter.set({});
    this.labelDraft.set({});
  }
  protected setValue(id: string, v: unknown): void { this.values.update((m) => ({ ...m, [id]: v })); }
  protected toggleMulti(id: string, optionId: string): void {
    const cur = this.values()[id];
    const list = Array.isArray(cur) ? (cur as string[]) : [];
    this.setValue(id, list.includes(optionId) ? list.filter((x) => x !== optionId) : [...list, optionId]);
  }

  protected submit(): void {
    const t = this.type();
    if (!t || !this.canSubmit()) return;
    for (const f of t.fields) if (f.kind === 'labels') this.commitLabel(f.id);
    this.saving.set(true);
    this.errors.set({});
    this.formError.set(null);
    this.api.createIssue({
      boardId: this.boardId(), typeId: t.id, summary: this.summary().trim(), description: this.description(),
      storyPoints: t.storyPointField ? this.sp() : null, assigneeAccountId: this.assignee(),
      parentKey: (t.subtask || t.parentAllowed) ? this.parentKey().trim() || null : null, fields: this.values(),
    }).subscribe({
      next: (res) => { this.saving.set(false); this.lastCreated.set(res); this.created.emit(res.key); },
      error: (err) => {
        this.saving.set(false);
        const body = err?.error ?? {};
        if (body.fields) this.errors.set(body.fields);
        this.formError.set(this.describeError(body));
      },
    });
  }

  /** Messages + field errors that have no inline slot; never the raw "jira_rejected" code. */
  private describeError(body: { error?: string; message?: string; messages?: string[]; fields?: Record<string, string> }): string {
    const shown = new Set(['summary', ...(this.type()?.fields ?? []).map((f) => f.id)]);
    const names = new Map((this.meta().types ?? []).flatMap((t) => t.fields).map((f) => [f.id, f.name] as const));
    const parts = [...(body.messages ?? [])];
    for (const [key, msg] of Object.entries(body.fields ?? {})) {
      if (!shown.has(key)) parts.push(`${names.get(key) ?? key}: ${msg}`);
    }
    if (parts.length) return parts.join(' ');
    if (body.message) return body.message;
    return body.error && body.error !== 'jira_rejected' ? body.error : 'Gagal membuat card';
  }

  /** "Buat lagi": keep type & parent, clear the rest. */
  protected again(): void {
    this.summary.set(''); this.description.set(''); this.sp.set(null); this.values.set({}); this.lastCreated.set(null);
    this.multiFilter.set({}); this.labelDraft.set({});
  }
}
