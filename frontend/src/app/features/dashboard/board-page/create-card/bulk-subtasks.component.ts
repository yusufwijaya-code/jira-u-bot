import { Component, OnInit, computed, inject, input, output, signal } from '@angular/core';
import { DashboardService, TicketCard } from '../../../../core/services/dashboard.service';
import { BulkResult, CreateMeta, IssueCreateService, ParentInfo } from '../../../../core/services/issue-create.service';
import { ConfirmService } from '../../../../core/ui/confirm.service';
import { UserPickerComponent } from './user-picker.component';

export function titleKey(s: string): string {
  return s.toLowerCase().replace(/[^\p{L}\p{N}]+/gu, ' ').trim();
}

interface PreviewRow {
  id: string;
  checked: boolean;
  title: string;
  storyPoints: number | null;
  assignee: string | null;
  details: string;
  open: boolean;
  duplicateOf: string | null;
  state: 'idle' | 'sending' | 'done' | 'failed';
  key?: string;
  error?: string;
}

const MAX_CHARS = 30000;
const MAX_ROWS = 50;
let seq = 0;

/** Bulk subtask (spec US-6/US-7): pick parent → paste → normalize → edit preview → create in order. */
@Component({
  selector: 'app-bulk-subtasks',
  standalone: true,
  imports: [UserPickerComponent],
  templateUrl: './bulk-subtasks.component.html',
  styleUrl: './bulk-subtasks.component.scss',
})
export class BulkSubtasksComponent implements OnInit {
  private api = inject(IssueCreateService);
  private dashboard = inject(DashboardService);
  private confirm = inject(ConfirmService);
  boardId = input.required<number>();
  meta = input.required<CreateMeta>();
  aiReady = input(false);
  initialParent = input<string | null>(null);
  created = output<void>();

  protected readonly maxChars = MAX_CHARS;
  protected candidates = signal<TicketCard[]>([]);
  protected parentQuery = signal('');
  protected parent = signal<ParentInfo | null>(null);
  protected parentError = signal<string | null>(null);
  protected text = signal('');
  protected normalizing = signal(false);
  protected confirming = signal(false);
  protected refreshing = signal(false);
  protected aiError = signal<string | null>(null);
  protected warnings = signal<string[]>([]);
  protected rows = signal<PreviewRow[]>([]);
  protected sending = signal(false);
  protected finished = signal<{ ok: number; failed: number } | null>(null);

  protected checked = computed(() => this.rows().filter((r) => r.checked && r.state !== 'done'));
  protected totalSP = computed(() => Math.round(this.checked().reduce((n, r) => n + (r.storyPoints ?? 0), 0) * 100) / 100);
  protected filteredCandidates = computed(() => {
    const q = this.parentQuery().trim().toLowerCase();
    return this.candidates().filter((c) => !q || c.issueKey.toLowerCase().includes(q) || c.summary.toLowerCase().includes(q)).slice(0, 8);
  });
  protected blocker = computed(() => {
    const c = this.checked();
    if (!this.parent()) return 'Pilih parent dulu';
    if (c.length === 0) return 'Centang minimal satu baris';
    if (c.length > MAX_ROWS) return `Maksimal ${MAX_ROWS} baris`;
    if (c.some((r) => !r.title.trim())) return 'Ada judul kosong';
    if (c.some((r) => r.storyPoints !== null && !(r.storyPoints >= 0 && r.storyPoints <= 100))) return 'SP harus 0–100';
    return null;
  });
  protected canCreate = computed(() => !this.sending() && !this.refreshing() && this.blocker() === null);

  ngOnInit(): void {
    this.dashboard.getBoard(this.boardId()).subscribe({ next: (b) => this.candidates.set((b.columns ?? []).flatMap((c) => c.cards).filter((c) => !c.parentKey)) });
    const p = this.initialParent();
    if (p) this.pickParent(p);
  }

  hasUnsent(): boolean {
    return this.rows().some((r) => r.state !== 'done');
  }

  protected pickParent(key: string): void {
    const k = key.trim().toUpperCase();
    if (!k) return;
    this.parentError.set(null);
    this.api.parent(this.boardId(), k).subscribe({
      next: (p) => { this.parent.set(p); this.parentQuery.set(''); this.markDuplicates(); },
      error: (err) => { this.parent.set(null); this.parentError.set(err?.status === 404 ? `${k} tidak ditemukan` : err?.error?.error ?? 'Parent tidak valid'); },
    });
  }

  protected async normalize(): Promise<void> {
    if (!this.aiReady() || !this.text().trim() || this.normalizing() || this.confirming()) return;
    if (this.rows().length > 0) {
      this.confirming.set(true);
      const ok = await this.confirm.ask({ title: 'Ganti preview?', message: 'Preview saat ini (termasuk edit dan baris yang sudah dibuat) akan diganti hasil normalize baru.', confirmText: 'Ya, ganti', tone: 'danger' });
      this.confirming.set(false);
      if (!ok) return;
    }
    this.normalizing.set(true);
    this.aiError.set(null);
    this.api.normalize(this.boardId(), this.text()).subscribe({
      next: (res) => {
        this.normalizing.set(false);
        this.warnings.set(res.warnings ?? []);
        this.rows.set((res.rows ?? []).map((r) => this.row(r.title, r.storyPoints, r.details)));
        this.finished.set(null);
        this.markDuplicates();
      },
      error: (err) => { this.normalizing.set(false); this.aiError.set(aiMessage(err)); },
    });
  }

  private row(title: string, sp: number | null, details: string): PreviewRow {
    return { id: `r${++seq}`, checked: true, title, storyPoints: sp, assignee: null, details, open: false, duplicateOf: null, state: 'idle' };
  }

  /** `adoptSent`: after an uncertain send, a failed/sending row that already exists in Jira counts as created. */
  private markDuplicates(adoptSent = false): void {
    const existing = new Map((this.parent()?.subtasks ?? []).map((s) => [titleKey(s.summary), s.key]));
    this.rows.update((rows) => rows.map((r) => {
      if (r.state === 'done') return r;
      const dup = existing.get(titleKey(r.title)) ?? null;
      if (adoptSent && dup && (r.state === 'failed' || r.state === 'sending')) {
        return { ...r, state: 'done', key: dup, duplicateOf: null, error: undefined, checked: false };
      }
      // uncheck only on the not-duplicate -> duplicate transition, so a deliberate re-check sticks
      return { ...r, duplicateOf: dup, checked: dup && !r.duplicateOf ? false : r.checked };
    }));
  }

  protected addRow(): void { this.rows.update((rows) => [...rows, this.row('', null, '')]); }
  protected removeRow(id: string): void { this.rows.update((rows) => rows.filter((r) => r.id !== id)); }
  protected patch(id: string, p: Partial<PreviewRow>): void {
    this.rows.update((rows) => rows.map((r) => (r.id === id ? { ...r, ...p } : r)));
    if (p.title !== undefined) this.markDuplicates();
  }

  private refreshParent(adoptSent = false): void {
    const parent = this.parent();
    if (!parent) return;
    this.refreshing.set(true);
    this.api.parent(this.boardId(), parent.key).subscribe({
      next: (p) => {
        this.parent.set(p);
        this.refreshing.set(false);
        this.markDuplicates(adoptSent);
        if (adoptSent) this.finished.set({ ok: this.rows().filter((r) => r.state === 'done').length, failed: this.rows().filter((r) => r.state === 'failed').length });
      },
      error: () => this.refreshing.set(false),
    });
  }

  protected create(onlyFailed = false): void {
    const parent = this.parent();
    const targets = this.rows().filter((r) => r.state !== 'done' && (onlyFailed ? r.state === 'failed' && !r.duplicateOf : r.checked));
    if (!parent || targets.length === 0) return;
    this.sending.set(true);
    const ids = new Set(targets.map((t) => t.id));
    this.rows.update((rows) => rows.map((r) => (ids.has(r.id) ? { ...r, state: 'sending', error: undefined } : r)));
    this.api.createSubtasks(this.boardId(), parent.key, targets.map((r) => ({
      clientId: r.id, title: r.title.trim(), storyPoints: r.storyPoints, assigneeAccountId: r.assignee, details: r.details,
    }))).subscribe({
      next: (res) => this.applyResults(res.results ?? []),
      error: (err) => {
        this.sending.set(false);
        // Only an unanswered/timed-out request is uncertain; 4xx/502 from our API mean nothing was created.
        const transport = !err?.status || err.status === 504;
        this.rows.update((rows) => rows.map((r) => (ids.has(r.id) ? { ...r, state: 'failed', error: transport ? 'Koneksi terputus — sebagian subtask mungkin sudah dibuat. Baris yang sudah ada ditandai.' : err?.error?.error ?? 'Gagal mengirim' } : r)));
        this.finished.set({ ok: this.rows().filter((r) => r.state === 'done').length, failed: this.rows().filter((r) => r.state === 'failed').length });
        if (transport) this.refreshParent(true);
      },
    });
  }

  private applyResults(results: BulkResult[]): void {
    const byId = new Map(results.map((r) => [r.clientId, r]));
    this.rows.update((rows) => rows.map((r) => {
      const res = byId.get(r.id);
      if (!res) return r.state === 'sending' ? { ...r, state: 'failed', error: 'Tidak ada respons untuk baris ini' } : r;
      return res.key ? { ...r, state: 'done', key: res.key, checked: false } : { ...r, state: 'failed', error: res.error };
    }));
    this.sending.set(false);
    const ok = this.rows().filter((r) => r.state === 'done').length;
    const failed = this.rows().filter((r) => r.state === 'failed').length;
    this.finished.set({ ok, failed });
    if (results.some((r) => r.key)) { this.created.emit(); this.refreshParent(); }
  }
}

function aiMessage(err: { status?: number; error?: { scope?: string; error?: string } }): string {
  if (err?.status === 429) {
    return err.error?.scope === 'daily'
      ? 'Kuota AI hari ini habis (reset ±14:00 WIB). Isi baris manual dulu atau coba lagi nanti.'
      : 'Kuota AI penuh, coba 1 menit lagi.';
  }
  if (err?.status === 503) return 'AI belum dikonfigurasi. Isi baris manual.';
  return err?.error?.error && err.status === 400 ? err.error.error : 'Normalize gagal, coba lagi atau isi baris manual.';
}
