import { Component, input, model, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DateRangePickerComponent } from './date-range-picker.component';
import { SelectComponent, SelectOption } from './select.component';
import { IconComponent } from './icon.component';

/** Kinds a Jira field can be filled with from Nudge; 'unsupported' = fill it in Jira. */
export type JiraFieldKind = 'text' | 'textarea' | 'number' | 'select' | 'cascade' | 'date' | 'labels' | 'unsupported';
export interface JiraFieldOption { id: string; label: string; /** Second level of a "cascade" field. */ children?: { id: string; label: string }[] }
/**
 * One Jira field as the backend describes it (createmeta/editmeta/transition screen). Values:
 * text/textarea → string, number → number, select → option id (multi: id[]), cascade → { id, child? },
 * date → "YYYY-MM-DD", labels → string[].
 */
export interface JiraField { id: string; name: string; kind: JiraFieldKind; multi: boolean; required: boolean; options?: JiraFieldOption[] }
/** Cascade value: parent option id + optional child option id. */
export interface CascadeValue { id: string; child?: string }

/** True when a form value counts as filled (same rule the server uses for required fields). */
export function fieldFilled(v: unknown): boolean {
  if (Array.isArray(v)) return v.length > 0;
  if (v && typeof v === 'object') return !!(v as CascadeValue).id;
  return v !== undefined && v !== null && String(v).trim() !== '';
}

/**
 * Renders one Jira field with Nudge's custom controls (no native select/date
 * inputs) — shared by Buat Card and the "Lengkapi dulu" move dialog. The
 * parent owns the value; every change comes back through valueChange.
 */
@Component({
  selector: 'app-jira-field-input',
  standalone: true,
  imports: [FormsModule, SelectComponent, DateRangePickerComponent, IconComponent],
  templateUrl: './jira-field-input.component.html',
  styleUrl: './jira-field-input.component.scss',
})
export class JiraFieldInputComponent {
  field = input.required<JiraField>();
  /** The parent owns the value: bind [value] and listen to (valueChange). */
  value = model<unknown>(undefined);
  error = input<string | null | undefined>(null);
  /** Jira link for an 'unsupported' field. */
  issueUrl = input<string>('');
  /** Visible rows of a textarea field. */
  rows = input(4);

  /** Chip lists longer than this get a search box and a scrollable wrap. */
  protected readonly longList = 12;
  protected filter = signal('');
  protected labelDraft = signal('');

  protected label(): string {
    const f = this.field();
    return f.name + (f.required ? ' *' : '');
  }

  protected text(): string {
    const v = this.value();
    return typeof v === 'string' ? v : '';
  }

  protected fieldOptions(): SelectOption<string>[] {
    return [{ value: '', label: 'Pilih…' }, ...(this.field().options ?? []).map((o) => ({ value: o.id, label: o.label }))];
  }

  protected cascade(): CascadeValue | null {
    const v = this.value() as CascadeValue | undefined;
    return v && typeof v === 'object' && v.id ? v : null;
  }
  protected childOptions(): SelectOption<string>[] {
    const parent = (this.field().options ?? []).find((o) => o.id === this.cascade()?.id);
    return parent?.children?.length ? [{ value: '', label: 'Pilih…' }, ...parent.children.map((c) => ({ value: c.id, label: c.label }))] : [];
  }
  /** A new parent drops the child (children belong to one parent). */
  protected setCascadeParent(id: string): void { this.value.set(id ? { id } : null); }
  protected setCascadeChild(child: string): void {
    const cur = this.cascade();
    if (cur) this.value.set(child ? { id: cur.id, child } : { id: cur.id });
  }

  protected isSelected(optionId: string): boolean {
    const v = this.value();
    return Array.isArray(v) ? v.includes(optionId) : v === optionId;
  }
  protected selectedCount(): number {
    const v = this.value();
    return Array.isArray(v) ? v.length : 0;
  }
  protected visibleOptions(): JiraFieldOption[] {
    const q = this.filter().trim().toLowerCase();
    const all = this.field().options ?? [];
    return q ? all.filter((o) => o.label.toLowerCase().includes(q) || this.isSelected(o.id)) : all;
  }
  protected toggleMulti(optionId: string): void {
    const cur = this.value();
    const list = Array.isArray(cur) ? (cur as string[]) : [];
    this.value.set(list.includes(optionId) ? list.filter((x) => x !== optionId) : [...list, optionId]);
  }

  protected labels(): string[] {
    const v = this.value();
    return Array.isArray(v) ? (v as string[]) : [];
  }
  protected setLabelDraft(el: HTMLInputElement): void {
    // Jira labels have no spaces: a space or comma commits what was typed.
    const parts = el.value.split(/[\s,]+/);
    const last = parts.pop() ?? '';
    if (parts.length) el.value = last;
    let next = this.labels();
    for (const p of parts) next = withLabel(next, p);
    if (next !== this.labels()) this.value.set(next);
    this.labelDraft.set(last);
  }
  /** Commits the label being typed (Enter / blur; parents call it before submitting). */
  commitLabel(event?: Event): void {
    const draft = this.labelDraft().trim();
    if (!draft) return;
    event?.preventDefault();
    const next = withLabel(this.labels(), draft);
    if (next !== this.labels()) this.value.set(next);
    this.labelDraft.set('');
  }
  protected removeLabel(label: string): void { this.value.set(this.labels().filter((l) => l !== label)); }

  protected setNumber(raw: string): void { this.value.set(raw === '' ? null : +raw); }
}

function withLabel(list: string[], label: string): string[] {
  const l = label.trim();
  return l && !list.includes(l) ? [...list, l] : list;
}
