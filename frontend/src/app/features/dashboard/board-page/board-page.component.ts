import { Component, DestroyRef, HostListener, computed, effect, inject, signal, untracked } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { Subscription } from 'rxjs';
import { environment } from '../../../../environments/environment';
import { DashboardService, TicketCard, TransitionOption } from '../../../core/services/dashboard.service';
import { BoardContextService } from '../../../core/services/board-context.service';
import { ToastService } from '../../../core/ui/toast.service';
import { NgTemplateOutlet } from '@angular/common';
import { BoardColumnsComponent } from './board-columns.component';
import { statusTone, shortDuration, budgetPercent, StatusTone } from './board-format';
import { IconComponent } from '../../../shared/ui/icon.component';

interface Column {
  status: string;
  cards: TicketCard[];
}

interface MenuState {
  key: string;
  loading: boolean;
  error: string | null;
  options: TransitionOption[];
}

export { statusTone, shortDuration };
export type { StatusTone };

export type BoardFilter = 'all' | 'over' | 'nosp';

export const COLLAPSED_STORAGE_KEY = 'nudge.board.collapsed';
export const BRANCHES_STORAGE_KEY = 'nudge.board.branches';
export const VIEW_STORAGE_KEY = 'nudge.board.view';

/**
 * Papan Tiket (Tech Spec "Papan Tiket & Cuti", US-1): the caller's
 * active-sprint tickets, one collapsible table section per status. The
 * status chip opens the moves the ticket's own Jira workflow allows, so
 * CORE and OMS rules apply as-is.
 */
@Component({
  selector: 'app-board-page',
  standalone: true,
  imports: [IconComponent, NgTemplateOutlet, BoardColumnsComponent],
  templateUrl: './board-page.component.html',
  styleUrl: './board-page.component.scss',
})
export class BoardPageComponent {
  private dashboard = inject(DashboardService);
  private boardContext = inject(BoardContextService);
  private toast = inject(ToastService);
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  protected readonly createCardEnabled = environment.features.createCard;
  protected tab = signal<'board' | 'create'>(
    environment.features.createCard && this.route.snapshot.queryParamMap.get('tab') === 'create' ? 'create' : 'board',
  );
  protected filter = signal<BoardFilter>('all');

  protected columns = signal<Column[]>([]);
  /**
   * The caller's subtasks / linked items of their sprint tickets that aren't
   * in the sprint themselves (GET /board → related). They hang under their
   * parent's row as branches and aren't counted per status, like on the
   * dashboard. Their status is movable like a sprint ticket's; the server
   * enforces ownership of out-of-sprint subtasks.
   */
  protected related = signal<TicketCard[]>([]);
  protected loading = signal(true);
  /** True once a board answer arrived — later loads refresh in place instead of blanking the page. */
  protected loaded = signal(false);
  /** First load: skeleton. */
  protected initialLoading = computed(() => this.loading() && !this.loaded());
  /** Reload with data on screen (board switch, after a move): progress bar + dimmed table. */
  protected refreshing = computed(() => this.loading() && this.loaded());
  protected error = signal<string | null>(null);
  /** The ticket just moved: its row is highlighted for a moment after the reload. */
  protected justMoved = signal<string | null>(null);
  private loadSub?: Subscription;
  private highlightTimer?: ReturnType<typeof setTimeout>;
  protected menu = signal<MenuState | null>(null);
  protected moving = signal<string | null>(null);
  protected view = signal<'list' | 'columns'>(readView());
  protected setView(v: 'list' | 'columns'): void {
    this.view.set(v);
    try { localStorage.setItem(VIEW_STORAGE_KEY, v); } catch { /* visit only */ }
  }
  protected branchCount = (key: string): number => (this.branchMap().get(key) ?? []).length;
  /** Moves sent to Jira and not answered yet; the board only reloads once none is left. */
  private pendingMoves = 0;
  /** Columns created on screen by applyLocalMove (status not in the last server load). */
  private syntheticStatuses = new Set<string>();
  protected query = signal('');
  /** Statuses the user collapsed (true) or expanded (false) themselves. */
  private collapsedChoice = signal<Record<string, boolean>>(readJson(COLLAPSED_STORAGE_KEY));
  /** Parents whose branches the user closed (true = closed). Default open. */
  private branchClosed = signal<Record<string, boolean>>(readJson(BRANCHES_STORAGE_KEY));

  protected readonly statusTone = statusTone;
  protected readonly shortDuration = shortDuration;

  protected total = computed(() => this.columns().reduce((n, c) => n + c.cards.length, 0));

  /** Branches keyed by their parent's issue key. */
  private branchMap = computed(() => {
    const byParent = new Map<string, TicketCard[]>();
    for (const card of this.related()) {
      if (!card.parentKey) continue;
      byParent.set(card.parentKey, [...(byParent.get(card.parentKey) ?? []), card]);
    }
    return byParent;
  });

  /** Branches shown on the page: only those whose parent row is listed. */
  protected branchTotal = computed(() => {
    const listed = new Set(this.columns().flatMap((c) => c.cards.map((card) => card.issueKey)));
    return this.related().filter((b) => b.parentKey && listed.has(b.parentKey)).length;
  });

  /**
   * Columns with only the cards matching the search (key, summary or
   * parent) — a card also stays when one of its branches matches.
   */
  protected visible = computed<Column[]>(() => {
    const q = this.query().trim().toLowerCase();
    const filtering = this.filter() !== 'all';
    if (!q && !filtering) return this.columns();
    return this.columns()
      .map((c) => ({
        status: c.status,
        cards: c.cards.filter(
          (card) =>
            this.passesFilter(card) &&
            (!q || matches(card, q) || (this.branchMap().get(card.issueKey) ?? []).some((b) => matches(b, q))),
        ),
      }))
      .filter((c) => c.cards.length > 0);
  });

  protected setTab(tab: 'board' | 'create', extra: Record<string, string | null> = {}): void {
    this.tab.set(tab);
    this.router.navigate([], { queryParams: { tab: tab === 'create' ? 'create' : null, ...extra }, queryParamsHandling: 'merge', replaceUrl: true });
  }

  protected stats = computed(() => {
    const cards = this.columns().flatMap((c) => c.cards);
    const done = cards.filter((c) => statusTone(c.status) === 'done').length;
    return {
      tickets: cards.length,
      points: Math.round(cards.reduce((n, c) => n + (c.storyPoints || 0), 0) * 10) / 10,
      over: cards.filter((c) => this.isOverBudget(c)).length,
      donePercent: cards.length ? Math.round((done / cards.length) * 100) : 0,
    };
  });

  protected setFilter(f: BoardFilter): void {
    this.filter.set(this.filter() === f ? 'all' : f);
  }

  protected resetFilters(): void {
    this.filter.set('all');
    this.query.set('');
  }

  private passesFilter(card: TicketCard): boolean {
    switch (this.filter()) {
      case 'over': return this.isOverBudget(card);
      case 'nosp': return !card.storyPoints;
      default: return true;
    }
  }

  @HostListener('document:keydown', ['$event'])
  protected onShortcut(event: KeyboardEvent): void {
    if (event.key !== '/' || this.tab() !== 'board') return;
    const t = event.target as HTMLElement | null;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
    event.preventDefault();
    (document.querySelector('[data-search]') as HTMLInputElement | null)?.focus();
  }

  /** The branches under a row; while searching, all of them when the parent matches, else only the matching ones. */
  protected branchesOf(card: TicketCard): TicketCard[] {
    if (!this.isBranchOpen(card.issueKey)) return [];
    const branches = this.branchMap().get(card.issueKey) ?? [];
    const q = this.query().trim().toLowerCase();
    if (!q || matches(card, q)) return branches;
    return branches.filter((b) => matches(b, q));
  }

  constructor() {
    inject(DestroyRef).onDestroy(() => clearTimeout(this.highlightTimer));
    // Tickets follow the navbar board, like the dashboard.
    effect(() => {
      const boardId = this.boardContext.selectedId();
      untracked(() => this.load(boardId));
    });
  }

  protected load(boardId: number | null = this.boardContext.selectedId()): void {
    this.loading.set(true);
    this.error.set(null);
    // A newer load (e.g. a quick board switch) wins over an older answer.
    this.loadSub?.unsubscribe();
    this.loadSub = this.dashboard.getBoard(boardId).subscribe({
      next: (res) => {
        this.columns.set((res.columns ?? []).map((c) => ({ status: c.status, cards: c.cards ?? [] })));
        this.related.set(res.related ?? []);
        this.syntheticStatuses.clear();
        this.loaded.set(true);
        this.loading.set(false);
      },
      error: () => {
        this.loading.set(false);
        if (this.loaded()) {
          // Keep the tickets already on screen; just say the refresh failed.
          this.toast.error('Gagal memuat ulang tiket', 'Data di layar mungkin belum terbaru, coba lagi.');
          return;
        }
        this.error.set('Gagal memuat tiket dari Jira, coba lagi');
      },
    });
  }

  /** Empty sections start collapsed; the user's own choice wins. */
  protected isCollapsed(col: Column): boolean {
    if (this.query().trim() || this.filter() !== 'all') return false;
    const choice = this.collapsedChoice()[col.status];
    return choice ?? col.cards.length === 0;
  }

  /** True when every listed section is closed — the toolbar button then offers "Buka semua". */
  protected allCollapsed = computed(() => {
    const cols = this.visible();
    return cols.length > 0 && cols.every((c) => this.isCollapsed(c));
  });

  protected toggleSection(col: Column): void {
    this.saveCollapsed({ ...this.collapsedChoice(), [col.status]: !this.isCollapsed(col) });
  }

  /** Opens every section when all are closed, otherwise closes them all. */
  protected toggleAll(): void {
    const collapse = !this.allCollapsed();
    const next = { ...this.collapsedChoice() };
    for (const c of this.columns()) next[c.status] = collapse;
    this.saveCollapsed(next);
  }

  private saveCollapsed(next: Record<string, boolean>): void {
    this.collapsedChoice.set(next);
    writeJson(COLLAPSED_STORAGE_KEY, next);
  }

  protected branchProgress(card: TicketCard): { total: number; done: number } {
    const all = this.branchMap().get(card.issueKey) ?? [];
    return { total: all.length, done: all.filter((b) => statusTone(b.status) === 'done').length };
  }

  protected isBranchOpen(key: string): boolean {
    return !!this.query().trim() || !this.branchClosed()[key];
  }

  protected toggleBranches(key: string): void {
    const next = { ...this.branchClosed(), [key]: this.isBranchOpen(key) };
    this.branchClosed.set(next);
    writeJson(BRANCHES_STORAGE_KEY, next);
  }

  protected copyKey(card: TicketCard): void {
    const clipboard = navigator.clipboard;
    if (!clipboard) {
      this.toast.error('Gagal menyalin', card.issueKey);
      return;
    }
    clipboard.writeText(card.issueKey).then(
      () => this.toast.success('Disalin', card.issueKey),
      () => this.toast.error('Gagal menyalin', card.issueKey),
    );
  }

  protected addSubtask(card: TicketCard): void {
    this.setTab('create', { mode: 'bulk', parent: card.issueKey });
  }

  protected timeTooltip(card: TicketCard): string {
    const parts = [`Dipakai ${shortDuration(card.elapsedSeconds)}`, `Budget ${shortDuration(card.budgetSeconds)}`];
    const diff = shortDuration(card.budgetSeconds - card.elapsedSeconds);
    parts.push(this.isOverBudget(card) ? `Lewat ${diff}` : `Sisa ${diff}`);
    const started = card.startedAt ? new Date(card.startedAt) : null;
    if (started && !isNaN(started.getTime())) {
      parts.push(`Mulai ${started.toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Jakarta' })}`);
    }
    return parts.join(' · ');
  }

  protected jumpTo(col: Column): void {
    if (this.isCollapsed(col)) this.toggleSection(col);
    document.getElementById(sectionId(col.status))?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  protected sectionId(status: string): string {
    return sectionId(status);
  }

  protected setQuery(value: string): void {
    this.query.set(value);
  }

  /** A running clock, or a stopped one with recorded time. */
  protected hasTime(card: TicketCard): boolean {
    return card.hasBudgetDetail || !!card.timeRecorded;
  }

  protected isOverBudget(card: TicketCard): boolean {
    return this.hasTime(card) && card.elapsedSeconds > card.budgetSeconds;
  }

  /** "sisa 1j" / "lewat 15m" while running; "di bawah budget 1j" / "lewat 15m" once stopped. */
  protected timeNote(card: TicketCard): string {
    const diff = shortDuration(card.budgetSeconds - card.elapsedSeconds);
    if (this.isOverBudget(card)) return `lewat ${diff}`;
    return card.hasBudgetDetail ? `sisa ${diff}` : `di bawah budget ${diff}`;
  }

  /** Share of the budget used, capped at 100 for the bar. */
  protected budgetPercent(card: TicketCard): number {
    return budgetPercent(card);
  }

  protected toggleMenu(card: TicketCard): void {
    if (this.menu()?.key === card.issueKey) {
      this.menu.set(null);
      return;
    }
    this.menu.set({ key: card.issueKey, loading: true, error: null, options: [] });
    this.dashboard.getTransitions(card.issueKey).subscribe({
      next: (res) => {
        // Ignore a late answer for a menu that was closed or switched.
        if (this.menu()?.key !== card.issueKey) return;
        this.menu.set({ key: card.issueKey, loading: false, error: null, options: res.transitions ?? [] });
      },
      error: (err) => {
        if (this.menu()?.key !== card.issueKey) return;
        this.menu.set({ key: card.issueKey, loading: false, error: err?.error?.error ?? 'Gagal memuat transisi', options: [] });
      },
    });
  }

  /** Jira's generic "Transition to" says nothing — the target chip already does. */
  protected optionLabel(opt: TransitionOption): string {
    return /^transition to$/i.test(opt.name.trim()) || !opt.name.trim() ? 'Pindah ke' : opt.name;
  }

  protected closeMenu(): void {
    this.menu.set(null);
  }

  /** Any click outside the open status menu closes it — anywhere on the page. */
  @HostListener('document:pointerdown', ['$event'])
  protected onDocumentPointerDown(event: Event): void {
    if (!this.menu()) return;
    const target = event.target as Element | null;
    if (target?.closest?.('.status-menu')) return;
    this.closeMenu();
  }

  @HostListener('document:keydown.escape')
  protected onEscape(): void {
    if (this.menu()) this.closeMenu();
  }

  protected move(card: TicketCard, option: TransitionOption): void {
    this.menu.set(null);
    const from = card.status;
    this.applyLocalMove(card.issueKey, option.toStatus);
    this.moving.set(card.issueKey);
    this.pendingMoves++;
    this.dashboard.moveTicket(card.issueKey, option.id).subscribe({
      next: () => {
        this.pendingMoves--;
        this.clearMoving(card.issueKey);
        this.highlight(card.issueKey);
        this.toast.show('success', 'Status dipindah', `${card.issueKey} → ${option.toStatus}`, 6000, {
          label: 'Batalkan',
          run: () => this.undo(card.issueKey, from),
        });
        // A reload now would overwrite other moves still in flight.
        if (this.pendingMoves === 0) this.load();
      },
      error: (err) => {
        this.pendingMoves--;
        this.applyLocalMove(card.issueKey, from);
        this.clearMoving(card.issueKey);
        this.toast.error('Gagal memindah status', err?.error?.error ?? 'Coba lagi beberapa saat.');
      },
    });
  }

  private clearMoving(issueKey: string): void {
    if (this.moving() === issueKey) this.moving.set(null);
  }

  /** Moves a card on screen only: sprint cards change section, branches change their status. */
  protected applyLocalMove(issueKey: string, toStatus: string): void {
    const cols = this.columns();
    const card = cols.flatMap((c) => c.cards).find((c) => c.issueKey === issueKey);
    if (card) {
      const moved = { ...card, status: toStatus };
      const without = cols.map((c) => ({ ...c, cards: c.cards.filter((x) => x.issueKey !== issueKey) }));
      const target = without.find((c) => c.status.toLowerCase() === toStatus.toLowerCase());
      if (!target) this.syntheticStatuses.add(toStatus.toLowerCase());
      const placed = target
        ? without.map((c) => (c === target ? { ...c, cards: [moved, ...c.cards] } : c))
        : [...without, { status: toStatus, cards: [moved] }];
      // A column made up by an earlier local move disappears again once it is empty (rollback).
      this.columns.set(placed.filter((c) => c.cards.length > 0 || !this.syntheticStatuses.has(c.status.toLowerCase())));
      return;
    }
    this.related.update((list) => list.map((b) => (b.issueKey === issueKey ? { ...b, status: toStatus } : b)));
  }

  protected undo(issueKey: string, fromStatus: string): void {
    this.dashboard.getTransitions(issueKey).subscribe({
      next: (res) => {
        const back = (res.transitions ?? []).find((t) => t.toStatus.toLowerCase() === fromStatus.toLowerCase());
        if (!back) {
          this.toast.error('Tidak bisa dibatalkan', `Workflow Jira tidak mengizinkan kembali ke ${fromStatus}.`);
          return;
        }
        const card = this.findCard(issueKey);
        if (!card) {
          this.toast.error('Tidak bisa dibatalkan', 'Tiket tidak lagi tampil di papan.');
          return;
        }
        this.move(card, back);
      },
      error: () => this.toast.error('Tidak bisa dibatalkan', 'Gagal memuat transisi dari Jira.'),
    });
  }

  private findCard(issueKey: string): TicketCard | undefined {
    return this.columns().flatMap((c) => c.cards).find((c) => c.issueKey === issueKey) ?? this.related().find((b) => b.issueKey === issueKey);
  }

  /** Flashes the moved row and brings it into view (its section may have changed). */
  private highlight(issueKey: string): void {
    clearTimeout(this.highlightTimer);
    this.justMoved.set(issueKey);
    const col = this.columns().find((c) => c.cards.some((card) => card.issueKey === issueKey));
    if (col && this.isCollapsed(col)) this.toggleSection(col);
    setTimeout(() => document.querySelector(`[data-card="${issueKey}"]`)?.scrollIntoView?.({ behavior: 'smooth', block: 'center' }));
    this.highlightTimer = setTimeout(() => this.justMoved.set(null), 2400);
  }
}

function readView(): 'list' | 'columns' {
  try { return localStorage.getItem(VIEW_STORAGE_KEY) === 'columns' ? 'columns' : 'list'; } catch { return 'list'; }
}

function matches(card: TicketCard, q: string): boolean {
  return [card.issueKey, card.summary, card.parentKey ?? '', card.parentSummary ?? ''].some((v) => v.toLowerCase().includes(q));
}

function sectionId(status: string): string {
  return 'status-' + status.toLowerCase().replace(/[^a-z0-9]+/g, '-');
}

function readJson(key: string): Record<string, boolean> {
  try {
    const parsed = JSON.parse(localStorage.getItem(key) ?? '{}');
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
}

function writeJson(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    /* storage unavailable — the choice lasts for this visit only */
  }
}
