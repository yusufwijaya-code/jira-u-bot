import { Component, computed, inject, input, output, signal } from '@angular/core';
import { DashboardService, TicketCard, TransitionOption } from '../../../core/services/dashboard.service';
import { budgetPercent, shortDuration, statusTone } from './board-format';

export interface ColumnView { status: string; cards: TicketCard[]; }
export interface MoveRequest { card: TicketCard; option: TransitionOption; }

/** Kanban view of Papan Tiket (spec US-2): drag a card onto a column its Jira workflow allows. */
@Component({
  selector: 'app-board-columns',
  standalone: true,
  templateUrl: './board-columns.component.html',
  styleUrl: './board-columns.component.scss',
})
export class BoardColumnsComponent {
  private dashboard = inject(DashboardService);
  columns = input.required<ColumnView[]>();
  branchCount = input<(key: string) => number>(() => 0);
  moving = input<string | null>(null);
  moveRequested = output<MoveRequest>();

  protected readonly statusTone = statusTone;
  protected readonly shortDuration = shortDuration;
  protected readonly budgetPercent = budgetPercent;
  protected dragging = signal<TicketCard | null>(null);
  /** Transitions per card, loaded once per page visit. */
  private cache = new Map<string, TransitionOption[]>();
  protected options = signal<TransitionOption[] | null>(null);

  /** Columns shown: the real ones plus, while dragging, a temporary zone per reachable status that has no card yet. */
  protected shown = computed(() => {
    const cols = this.columns().map((c) => ({ ...c, temp: false }));
    const card = this.dragging();
    const opts = this.options();
    if (!card || !opts) return cols;
    const have = new Set(cols.map((c) => c.status.toLowerCase()));
    for (const o of opts) {
      if (have.has(o.toStatus.toLowerCase())) continue;
      have.add(o.toStatus.toLowerCase());
      cols.push({ status: o.toStatus, cards: [], temp: true });
    }
    return cols;
  });
  protected touch = typeof matchMedia === 'function' && matchMedia('(hover: none), (max-width: 767px)').matches;

  private cacheKey(card: TicketCard): string {
    return `${card.issueKey}|${card.status}`;
  }

  protected targetOf(status: string): TransitionOption | undefined {
    return this.options()?.find((o) => o.toStatus.toLowerCase() === status.toLowerCase());
  }

  protected state(status: string): 'idle' | 'loading' | 'ok' | 'no' {
    const card = this.dragging();
    if (!card) return 'idle';
    if (status === card.status) return 'idle';
    if (this.options() === null) return 'loading';
    return this.targetOf(status) ? 'ok' : 'no';
  }

  protected onDragStart(event: DragEvent, card: TicketCard): void {
    event.dataTransfer?.setData('text/plain', card.issueKey);
    if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move';
    this.dragging.set(card);
    const cached = this.cache.get(this.cacheKey(card));
    if (cached) {
      this.options.set(cached);
      return;
    }
    this.options.set(null);
    this.dashboard.getTransitions(card.issueKey).subscribe({
      next: (res) => {
        this.cache.set(this.cacheKey(card), res.transitions ?? []);
        if (this.dragging()?.issueKey === card.issueKey) this.options.set(res.transitions ?? []);
      },
      error: () => this.options.set([]),
    });
  }

  protected onDragOver(event: DragEvent, status: string): void {
    if (this.state(status) === 'ok') event.preventDefault();
  }

  protected onDrop(event: DragEvent, status: string): void {
    event.preventDefault();
    const card = this.dragging();
    const option = this.targetOf(status);
    // A refused drop keeps the drag alive; dragend (always fired by the browser) clears it.
    if (!card || !option || status === card.status) return;
    this.onDragEnd();
    this.moveRequested.emit({ card, option });
  }

  protected onDragEnd(): void {
    this.dragging.set(null);
    this.options.set(null);
  }
}
