import { Component, computed, effect, inject, signal, untracked } from '@angular/core';
import { DashboardService, TicketCard, TransitionOption } from '../../../core/services/dashboard.service';
import { BoardContextService } from '../../../core/services/board-context.service';
import { ToastService } from '../../../core/ui/toast.service';
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
  imports: [IconComponent],
  templateUrl: './board-page.component.html',
  styleUrl: './board-page.component.scss',
})
export class BoardPageComponent {
  private dashboard = inject(DashboardService);
  private boardContext = inject(BoardContextService);
  private toast = inject(ToastService);

  protected columns = signal<Column[]>([]);
  protected loading = signal(true);
  protected error = signal<string | null>(null);
  protected menu = signal<MenuState | null>(null);
  protected moving = signal<string | null>(null);
  protected query = signal('');
  /** Statuses the user collapsed (true) or expanded (false) themselves. */
  private collapsedChoice = signal<Record<string, boolean>>(readCollapsed());

  protected readonly statusTone = statusTone;
  protected readonly shortDuration = shortDuration;

  protected total = computed(() => this.columns().reduce((n, c) => n + c.cards.length, 0));

  /** Columns with only the cards matching the search (key, summary or parent). */
  protected visible = computed<Column[]>(() => {
    const q = this.query().trim().toLowerCase();
    if (!q) return this.columns();
    return this.columns()
      .map((c) => ({
        status: c.status,
        cards: c.cards.filter((card) =>
          [card.issueKey, card.summary, card.parentKey ?? '', card.parentSummary ?? ''].some((v) => v.toLowerCase().includes(q)),
        ),
      }))
      .filter((c) => c.cards.length > 0);
  });

  constructor() {
    // Tickets follow the navbar board, like the dashboard.
    effect(() => {
      const boardId = this.boardContext.selectedId();
      untracked(() => this.load(boardId));
    });
  }

  protected load(boardId: number | null = this.boardContext.selectedId()): void {
    this.loading.set(true);
    this.error.set(null);
    this.dashboard.getBoard(boardId).subscribe({
      next: (res) => {
        this.columns.set((res.columns ?? []).map((c) => ({ status: c.status, cards: c.cards ?? [] })));
        this.loading.set(false);
      },
      error: () => {
        this.error.set('Gagal memuat tiket dari Jira, coba lagi');
        this.loading.set(false);
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

  protected isOverBudget(card: TicketCard): boolean {
    return card.hasBudgetDetail && card.elapsedSeconds > card.budgetSeconds;
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
        this.moving.set(null);
        this.toast.success('Status dipindah', `${card.issueKey} → ${option.toStatus}`);
        this.load();
      },
      error: (err) => {
        this.moving.set(null);
        this.toast.error('Gagal memindah status', err?.error?.error ?? 'Coba lagi beberapa saat.');
      },
    });
  }
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
