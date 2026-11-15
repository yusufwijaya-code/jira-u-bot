import { Component, effect, inject, signal, untracked } from '@angular/core';
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

/**
 * Papan Tiket (Tech Spec "Papan Tiket & Cuti", US-1): the caller's
 * active-sprint tickets per status. The status chip opens the moves the
 * ticket's own Jira workflow allows, so CORE and OMS rules apply as-is.
 */
@Component({
  selector: 'app-board-page',
  standalone: true,
  imports: [IconComponent],
  templateUrl: './board-page.component.html',
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
