import { Component, DestroyRef, computed, effect, inject, signal, untracked } from '@angular/core';
import { Subscription } from 'rxjs';
import { DashboardService, TicketCard, TransitionOption } from '../../../core/services/dashboard.service';
import { BoardContextService } from '../../../core/services/board-context.service';
import { ToastService } from '../../../core/ui/toast.service';
import { NgTemplateOutlet } from '@angular/common';
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

/** Colour family of a status dot, from its name (Jira workflows differ per project). */
export type StatusTone = 'todo' | 'process' | 'review' | 'done' | 'other';

export const COLLAPSED_STORAGE_KEY = 'nudge.board.collapsed';

export function statusTone(status: string): StatusTone {
  const s = status.toLowerCase();
  if (/(complete|done|selesai|closed|resolved)/.test(s)) return 'done';
  if (/review/.test(s)) return 'review';
  if (/(process|progress|develop)/.test(s)) return 'process';
  if (/(to do|todo|backlog|open)/.test(s)) return 'todo';
  return 'other';
}

/** 5400 → "1j 30m", 2700 → "45m", 7200 → "2j", 20 → "0m". */
export function shortDuration(totalSeconds: number): string {
  const minutes = Math.floor(Math.abs(totalSeconds) / 60);
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h === 0) return `${m}m`;
  return m === 0 ? `${h}j` : `${h}j ${m}m`;
}

/**
 * Papan Tiket (Tech Spec "Papan Tiket & Cuti", US-1): the caller's
 * active-sprint tickets, one collapsible table section per status. The
 * status chip opens the moves the ticket's own Jira workflow allows, so
 * CORE and OMS rules apply as-is.
 */
@Component({
  selector: 'app-board-page',
  standalone: true,
  imports: [IconComponent, NgTemplateOutlet],
  templateUrl: './board-page.component.html',
  styleUrl: './board-page.component.scss',
})
export class BoardPageComponent {
  private dashboard = inject(DashboardService);
  private boardContext = inject(BoardContextService);
  private toast = inject(ToastService);

  protected columns = signal<Column[]>([]);
  /**
   * The caller's subtasks / linked items of their sprint tickets that aren't
   * in the sprint themselves (GET /board → related). They hang under their
   * parent's row as branches and aren't counted per status, like on the
   * dashboard. Their status is read-only: moves are limited to sprint
   * tickets (Tech Spec "Papan Tiket & Cuti", AC1.6).
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
  protected query = signal('');
  /** Statuses the user collapsed (true) or expanded (false) themselves. */
  private collapsedChoice = signal<Record<string, boolean>>(readCollapsed());

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
    if (!q) return this.columns();
    return this.columns()
      .map((c) => ({
        status: c.status,
        cards: c.cards.filter((card) => matches(card, q) || (this.branchMap().get(card.issueKey) ?? []).some((b) => matches(b, q))),
      }))
      .filter((c) => c.cards.length > 0);
  });

  /** The branches under a row; while searching, all of them when the parent matches, else only the matching ones. */
  protected branchesOf(card: TicketCard): TicketCard[] {
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

  protected load(boardId: number | null = this.boardContext.selectedId(), done?: () => void): void {
    this.loading.set(true);
    this.error.set(null);
    // A newer load (e.g. a quick board switch) wins over an older answer.
    this.loadSub?.unsubscribe();
    this.loadSub = this.dashboard.getBoard(boardId).subscribe({
      next: (res) => {
        this.columns.set((res.columns ?? []).map((c) => ({ status: c.status, cards: c.cards ?? [] })));
        this.related.set(res.related ?? []);
        this.loaded.set(true);
        this.loading.set(false);
        done?.();
      },
      error: () => {
        this.loading.set(false);
        done?.();
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
    if (this.query().trim()) return false;
    const choice = this.collapsedChoice()[col.status];
    return choice ?? col.cards.length === 0;
  }

  protected toggleSection(col: Column): void {
    const next = { ...this.collapsedChoice(), [col.status]: !this.isCollapsed(col) };
    this.collapsedChoice.set(next);
    try {
      localStorage.setItem(COLLAPSED_STORAGE_KEY, JSON.stringify(next));
    } catch {
      /* storage unavailable — the choice lasts for this visit only */
    }
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
    if (!card.budgetSeconds) return 0;
    return Math.min(100, Math.round((card.elapsedSeconds / card.budgetSeconds) * 100));
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

  protected move(card: TicketCard, option: TransitionOption): void {
    this.menu.set(null);
    this.moving.set(card.issueKey);
    this.dashboard.moveTicket(card.issueKey, option.id).subscribe({
      next: () => {
        this.toast.success('Status dipindah', `${card.issueKey} → ${option.toStatus}`);
        // The row keeps its spinner until the refreshed board shows it in its new section.
        this.load(undefined, () => {
          this.moving.set(null);
          this.highlight(card.issueKey);
        });
      },
      error: (err) => {
        this.moving.set(null);
        this.toast.error('Gagal memindah status', err?.error?.error ?? 'Coba lagi beberapa saat.');
      },
    });
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

function matches(card: TicketCard, q: string): boolean {
  return [card.issueKey, card.summary, card.parentKey ?? '', card.parentSummary ?? ''].some((v) => v.toLowerCase().includes(q));
}

function sectionId(status: string): string {
  return 'status-' + status.toLowerCase().replace(/[^a-z0-9]+/g, '-');
}

function readCollapsed(): Record<string, boolean> {
  try {
    const parsed = JSON.parse(localStorage.getItem(COLLAPSED_STORAGE_KEY) ?? '{}');
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
}
