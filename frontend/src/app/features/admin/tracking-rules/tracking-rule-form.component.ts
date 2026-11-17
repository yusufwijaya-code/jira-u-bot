import { Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { HttpErrorResponse } from '@angular/common/http';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import {
  CreateFieldKind,
  CreateFieldType,
  JiraProjectLookup,
  PROJECT_KEY_PATTERN,
  TrackingRule,
  TrackingRuleError,
  TrackingRuleField,
  TrackingRuleInput,
  TrackingRuleService,
  normalizeProjectKey,
} from '../../../core/services/tracking-rule.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { SelectComponent, SelectOption } from '../../../shared/ui/select.component';
import { recalculateAction } from './recalculate-action';

const MAX_FIELD_SUGGESTIONS = 12;

const KIND_LABELS: Record<CreateFieldKind, string> = {
  text: 'Teks', textarea: 'Teks panjang', number: 'Angka', select: 'Pilihan', cascade: 'Pilihan bertingkat', date: 'Tanggal', labels: 'Label',
};

function hasFold(list: string[], value: string): boolean {
  return list.some((v) => v.toLowerCase() === value.toLowerCase());
}

/**
 * Tambah / edit satu aturan board (Tech Spec "Aturan Board (Admin CRUD)",
 * US-3/US-4): statuses and Story Point fields are picked from the project's
 * Jira data; stored values Jira no longer has are flagged ⚠ and must be
 * dropped before saving. "Field form Buat Card" picks, per issue type, the
 * optional Jira fields the create form shows (required ones always show).
 */
@Component({
  selector: 'app-tracking-rule-form',
  standalone: true,
  imports: [FormsModule, RouterLink, IconComponent, SelectComponent],
  templateUrl: './tracking-rule-form.component.html',
  styleUrl: './tracking-rule-form.component.scss',
})
export class TrackingRuleFormComponent {
  private api = inject(TrackingRuleService);
  private toast = inject(ToastService);
  private router = inject(Router);

  /** The rule being edited (route /admin/tracking-rules/:key); null = new rule. */
  protected readonly editKey: string | null = inject(ActivatedRoute).snapshot.paramMap.get('key');
  protected readonly isEdit = this.editKey !== null;

  protected keyInput = signal(this.editKey ?? '');
  protected lookup = signal<JiraProjectLookup | null>(null);
  protected lookingUp = signal(false);
  protected lookupError = signal<string | null>(null);
  protected loadError = signal<string | null>(null);
  protected avatarFailed = signal(false);

  protected tracked = signal<string[]>([]);
  protected done = signal('');
  protected review = signal('');
  protected spFields = signal<string[]>([]);
  protected writeField = signal('');
  protected writeFieldSubtask = signal('');
  protected fieldQuery = signal('');

  /** Issue type id → optional fields shown on Buat Card. */
  protected createFields = signal<Record<string, string[]>>({});
  protected cfTypes = signal<CreateFieldType[] | null>(null);
  protected cfLoading = signal(false);
  protected cfError = signal<string | null>(null);
  protected cfTypeId = signal('');
  protected readonly kindLabels = KIND_LABELS;
  /** The selected type, without the Story Point fields (Buat Card has its own SP input). */
  protected cfType = computed(() => {
    const t = (this.cfTypes() ?? []).find((x) => x.typeId === this.cfTypeId());
    if (!t) return null;
    const sp = new Set(this.spFields());
    return { ...t, fields: t.fields.filter((f) => !sp.has(f.id)) };
  });
  /** Total optional fields chosen, over the types Jira still has. */
  protected cfTotal = computed(() => (this.cfTypes() ?? []).reduce((n, t) => n + this.cfCount(t.typeId), 0));

  protected saving = signal(false);
  protected fieldErrors = signal<Partial<Record<TrackingRuleField, string>>>({});
  protected existingKey = signal<string | null>(null);
  /** Field names of the stored rule, shown until (or without) the Jira lookup. */
  private storedNames = signal(new Map<string, string>());

  protected keyValid = computed(() => PROJECT_KEY_PATTERN.test(normalizeProjectKey(this.keyInput())));
  protected statuses = computed(() => this.lookup()?.statuses ?? []);
  private fieldNames = computed(() => {
    const names = new Map(this.storedNames());
    for (const f of this.lookup()?.numberFields ?? []) names.set(f.id, f.name);
    return names;
  });

  /** Stored statuses no longer in the project's Jira workflow (AC4.4). */
  protected staleStatuses = computed(() => {
    if (!this.lookup()) return [];
    const all = this.statuses();
    const used = [...this.tracked(), this.done(), this.review()].filter((s) => s !== '');
    return [...new Set(used)].filter((s) => !hasFold(all, s));
  });
  /** Stored Story Point fields Jira no longer has as a number field. */
  protected staleFields = computed(() => {
    const l = this.lookup();
    if (!l) return [];
    return this.spFields().filter((id) => !l.numberFields.some((f) => f.id === id));
  });

  protected doneOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Pilih status selesai…' },
    ...this.statuses()
      .filter((s) => !hasFold(this.tracked(), s))
      .map((s) => ({ value: s, label: s })),
  ]);
  protected reviewOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Tidak ada — review tidak dihitung' },
    ...this.tracked().map((s) => ({ value: s, label: s })),
  ]);
  protected writeOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Pilih field…' },
    ...this.spFields().map((id) => ({ value: id, label: this.fieldLabel(id) })),
  ]);
  /** Number fields not picked yet, story-point-like first (server order), filtered by the search box. */
  protected suggestions = computed(() => {
    const q = this.fieldQuery().trim().toLowerCase();
    const picked = this.spFields();
    return (this.lookup()?.numberFields ?? [])
      .filter((f) => !picked.includes(f.id))
      .filter((f) => !q || f.name.toLowerCase().includes(q) || f.id.includes(q))
      .slice(0, MAX_FIELD_SUGGESTIONS);
  });

  /** AC3.3 — live sentence of what the rule does. */
  protected preview = computed(() => {
    const label = (id: string) => (id ? this.fieldLabel(id) : '…');
    const tracked = this.tracked();
    const fields = this.spFields();
    return (
      `Budget berjalan selama ${tracked.length ? tracked.join(' atau ') : '…'}; selesai saat ${this.done() || '…'}. ` +
      `Review ${this.review() ? 'ikut' : 'tidak'} dihitung. ` +
      `SP dibaca dari ${fields.length ? fields.map(label).join(' → ') : '…'}; ` +
      `ditulis ke ${label(this.writeField())} (card biasa) / ${label(this.writeFieldSubtask())} (subtask).`
    );
  });

  protected canSave = computed(
    () =>
      !!this.lookup() &&
      !this.saving() &&
      this.tracked().length > 0 &&
      this.done() !== '' &&
      this.spFields().length > 0 &&
      this.writeField() !== '' &&
      this.writeFieldSubtask() !== '' &&
      this.staleStatuses().length === 0 &&
      this.staleFields().length === 0,
  );

  constructor() {
    if (this.editKey) this.loadRule(this.editKey);
  }

  protected fieldLabel(id: string): string {
    const name = this.fieldNames().get(id);
    return name ? `${name} (${id})` : id;
  }

  private loadRule(key: string): void {
    this.api.list().subscribe({
      next: (res) => {
        const rule = res.rules.find((r) => r.projectKey === key);
        if (!rule) {
          this.loadError.set(`Aturan untuk ${key} tidak ditemukan.`);
          return;
        }
        this.apply(rule);
        // AC4.1: reload the Jira data so the choices are current.
        this.runLookup();
      },
      error: () => this.loadError.set('Gagal memuat aturan, coba lagi.'),
    });
  }

  private apply(rule: TrackingRule): void {
    this.storedNames.set(new Map(rule.storyPointFields.map((f) => [f.id, f.name] as [string, string])));
    this.tracked.set([...rule.trackedStatuses]);
    this.done.set(rule.doneStatus);
    this.review.set(rule.reviewStatus ?? '');
    this.spFields.set(rule.storyPointFields.map((f) => f.id));
    this.writeField.set(rule.storyPointWriteField);
    this.writeFieldSubtask.set(rule.storyPointWriteFieldSubtask);
    this.createFields.set(Object.fromEntries(Object.entries(rule.createFields ?? {}).map(([k, v]) => [k, [...v]])));
  }

  protected onKeyInput(value: string): void {
    this.keyInput.set(value ?? '');
    const current = this.lookup();
    // Another key: what was picked for the old project no longer applies.
    if (current && normalizeProjectKey(value ?? '') !== current.projectKey) {
      this.lookup.set(null);
      this.tracked.set([]);
      this.done.set('');
      this.review.set('');
      this.spFields.set([]);
      this.writeField.set('');
      this.writeFieldSubtask.set('');
      this.createFields.set({});
      this.cfTypes.set(null);
      this.cfError.set(null);
    }
  }

  protected runLookup(): void {
    const key = normalizeProjectKey(this.keyInput());
    if (!PROJECT_KEY_PATTERN.test(key) || this.lookingUp()) return;
    this.keyInput.set(key);
    this.lookingUp.set(true);
    this.lookupError.set(null);
    this.existingKey.set(null);
    this.avatarFailed.set(false);
    // Edit (load + "Muat ulang dari Jira") reads Jira fresh, past the server cache.
    this.api.lookup(key, this.isEdit).subscribe({
      next: (l) => {
        this.lookingUp.set(false);
        this.lookup.set(l);
        this.loadCreateFields();
      },
      error: (err: HttpErrorResponse) => {
        this.lookingUp.set(false);
        this.lookup.set(null);
        this.lookupError.set(
          err?.status === 404
            ? `Project ${key} tidak ditemukan di Jira`
            : err?.status === 400
              ? 'Format project key tidak valid'
              : 'Jira tidak merespons, coba lagi',
        );
      },
    });
  }

  /** Issue types + fillable fields for "Field form Buat Card"; a failure only affects that section. */
  protected loadCreateFields(): void {
    const key = this.lookup()?.projectKey;
    if (!key) return;
    this.cfLoading.set(true);
    this.cfError.set(null);
    this.api.createFields(key, this.isEdit).subscribe({
      next: (res) => {
        if (this.lookup()?.projectKey !== key) return;
        this.cfLoading.set(false);
        const types = res.types ?? [];
        this.cfTypes.set(types);
        if (!types.some((t) => t.typeId === this.cfTypeId())) this.cfTypeId.set(types.find((t) => !t.subtask)?.typeId ?? types[0]?.typeId ?? '');
      },
      error: () => {
        this.cfLoading.set(false);
        this.cfTypes.set(null);
        this.cfError.set('Gagal memuat field Jira, coba lagi.');
      },
    });
  }

  protected cfCount(typeId: string): number {
    const t = (this.cfTypes() ?? []).find((x) => x.typeId === typeId);
    const chosen = this.createFields()[typeId] ?? [];
    return t ? t.fields.filter((f) => !f.required && chosen.includes(f.id)).length : chosen.length;
  }

  protected cfChosen(typeId: string, fieldId: string): boolean {
    return (this.createFields()[typeId] ?? []).includes(fieldId);
  }

  protected toggleCreateField(typeId: string, fieldId: string): void {
    const cur = this.createFields()[typeId] ?? [];
    const next = cur.includes(fieldId) ? cur.filter((f) => f !== fieldId) : [...cur, fieldId];
    this.createFields.set({ ...this.createFields(), [typeId]: next });
    this.clearErrors('createFields');
  }

  /** What is saved: with Jira's list loaded, only optional fields Jira still has on types it still has. */
  private createFieldsInput(): Record<string, string[]> {
    const types = this.cfTypes();
    const out: Record<string, string[]> = {};
    for (const [typeId, ids] of Object.entries(this.createFields())) {
      const t = types?.find((x) => x.typeId === typeId);
      if (types && !t) continue;
      const keep = t ? ids.filter((id) => t.fields.some((f) => f.id === id && !f.required)) : ids;
      if (keep.length) out[typeId] = keep;
    }
    return out;
  }

  protected isTracked(status: string): boolean {
    return hasFold(this.tracked(), status);
  }

  protected toggleTracked(status: string): void {
    const next = this.isTracked(status)
      ? this.tracked().filter((s) => s.toLowerCase() !== status.toLowerCase())
      : [...this.tracked(), status];
    this.tracked.set(next);
    if (this.done() && hasFold(next, this.done())) this.done.set('');
    if (this.review() && !hasFold(next, this.review())) this.review.set('');
    this.clearErrors('trackedStatuses', 'doneStatus', 'reviewStatus');
  }

  protected setDone(value: string): void {
    this.done.set(value ?? '');
    this.clearErrors('doneStatus');
  }

  protected setReview(value: string): void {
    this.review.set(value ?? '');
    this.clearErrors('reviewStatus');
  }

  protected setWrite(which: 'card' | 'subtask', value: string): void {
    if (which === 'card') {
      this.writeField.set(value ?? '');
      this.clearErrors('storyPointWriteField');
    } else {
      this.writeFieldSubtask.set(value ?? '');
      this.clearErrors('storyPointWriteFieldSubtask');
    }
  }

  protected addField(id: string): void {
    if (this.spFields().includes(id)) return;
    const next = [...this.spFields(), id];
    this.spFields.set(next);
    // Default write field = the first read field.
    if (!this.writeField()) this.writeField.set(next[0]);
    if (!this.writeFieldSubtask()) this.writeFieldSubtask.set(next[0]);
    this.fieldQuery.set('');
    this.clearErrors('storyPointFields');
  }

  protected moveField(index: number, step: -1 | 1): void {
    const list = [...this.spFields()];
    const target = index + step;
    if (target < 0 || target >= list.length) return;
    [list[index], list[target]] = [list[target], list[index]];
    this.spFields.set(list);
  }

  protected removeField(id: string): void {
    this.spFields.set(this.spFields().filter((f) => f !== id));
    // A write dropdown pointing at the removed field is reset.
    if (this.writeField() === id) this.writeField.set('');
    if (this.writeFieldSubtask() === id) this.writeFieldSubtask.set('');
  }

  protected dropStaleStatus(status: string): void {
    this.tracked.set(this.tracked().filter((s) => s !== status));
    if (this.done() === status) this.done.set('');
    if (this.review() === status || (this.review() && !hasFold(this.tracked(), this.review()))) this.review.set('');
  }

  private clearErrors(...fields: TrackingRuleField[]): void {
    const errors = { ...this.fieldErrors() };
    for (const f of fields) delete errors[f];
    this.fieldErrors.set(errors);
  }

  protected save(): void {
    const l = this.lookup();
    if (!l || !this.canSave()) return;
    const key = l.projectKey;
    const input: TrackingRuleInput = {
      trackedStatuses: this.tracked(),
      doneStatus: this.done(),
      reviewStatus: this.review() || null,
      storyPointFields: this.spFields(),
      storyPointWriteField: this.writeField(),
      storyPointWriteFieldSubtask: this.writeFieldSubtask(),
      createFields: this.createFieldsInput(),
    };
    this.saving.set(true);
    this.existingKey.set(null);
    const request = this.isEdit ? this.api.update(key, input) : this.api.create({ ...input, projectKey: key });
    request.subscribe({
      next: () => {
        this.saving.set(false);
        this.toast.show(
          'success',
          'Aturan board tersimpan',
          `${key} berlaku mulai cycle poller berikutnya. Waktu yang sudah tercatat bisa dihitung ulang dari Jira.`,
          10000,
          recalculateAction(this.api, this.toast),
        );
        this.router.navigate(['/admin/tracking-rules']);
      },
      error: (err: HttpErrorResponse) => {
        this.saving.set(false);
        const body = err?.error as TrackingRuleError | undefined;
        if (err?.status === 400 && body?.fields) {
          this.fieldErrors.set(body.fields);
          return;
        }
        if (err?.status === 409) {
          this.existingKey.set(key);
          return;
        }
        this.toast.error('Gagal menyimpan aturan', body?.message ?? 'Coba lagi sebentar.');
      },
    });
  }
}
