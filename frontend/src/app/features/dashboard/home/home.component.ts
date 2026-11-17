import { Component, computed, effect, inject, signal, untracked } from '@angular/core';
import { DecimalPipe, NgTemplateOutlet } from '@angular/common';
import { RouterLink } from '@angular/router';
import { toSignal } from '@angular/core/rxjs-interop';
import {
  DashboardService,
  SprintReport,
  TicketCard,
  TransitionTarget,
} from '../../../core/services/dashboard.service';
import { BoardContextService, BoardSummary } from '../../../core/services/board-context.service';
import { BoardAvatarComponent } from '../../../shared/ui/board-avatar.component';
import { AuthService } from '../../../core/services/auth.service';
import { MuteStateService } from '../../../core/services/mute-state.service';
import { NotificationSwitchService } from '../../../core/services/notification-switch.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { activeUntil } from '../settings/mute-panel.component';

const CODE_PROCESS = 'code process';
const CODE_REVIEW = 'code review';
const STATUS_ORDER = ['to do', 'code process', 'code review'];
const GROUP_PREVIEW = 5;

export type BudgetLevel = 'ok' | 'warn' | 'over';

export interface ActiveTicket extends TicketCard {
  percent: number;
  level: BudgetLevel;
  /** Short "2 jam 35 menit" texts for the meta line (like the DM reminder). */
  elapsedText: string;
  budgetText: string;
  remainingText: string;
  /** "Hari ini 16:05" / "Sen, 17 Nov 11:00"; "" when unknown. */
  startedText: string;
  deadlineText: string;
}

/** 9300 → "2 jam 35 menit", 900 → "15 menit", 0 → "0 menit"; sign is ignored. */
export function shortDuration(seconds: number): string {
  const total = Math.floor(Math.abs(seconds) / 60);
  const days = Math.floor(total / 1440);
  const hours = Math.floor((total % 1440) / 60);
  const minutes = total % 60;
  const parts = [days && `${days} hari`, hours && `${hours} jam`, minutes && `${minutes} menit`].filter(Boolean);
  return parts.length ? parts.join(' ') : '0 menit';
}

const WIB = 'Asia/Jakarta';
const dayKey = (d: Date) => d.toLocaleDateString('en-CA', { timeZone: WIB });

/** ISO instant → "Hari ini 16:05", "Besok 08:30", "Kemarin 09:10" or "Sen, 17 Nov 11:00" (WIB). */
export function whenLabel(iso: string | undefined, now: Date = new Date()): string {
  if (!iso) return '';
  const at = new Date(iso);
  if (isNaN(at.getTime())) return '';
  const time = at.toLocaleTimeString('id-ID', { timeZone: WIB, hour: '2-digit', minute: '2-digit', hour12: false }).replace('.', ':');
  const day = 86_400_000;
  const relative: Record<string, string> = {
    [dayKey(now)]: 'Hari ini',
    [dayKey(new Date(now.getTime() + day))]: 'Besok',
    [dayKey(new Date(now.getTime() - day))]: 'Kemarin',
  };
  const prefix =
    relative[dayKey(at)] ??
    at.toLocaleDateString('id-ID', { timeZone: WIB, weekday: 'short', day: 'numeric', month: 'short' }).replace('.', '');
  return `${prefix} ${time}`;
}

export interface StatusGroup {
  status: string;
  cards: TicketCard[];
}

export function budgetPercent(card: TicketCard): number {
  if (!card.hasBudgetDetail || card.budgetSeconds <= 0) return 0;
  return Math.round((card.elapsedSeconds / card.budgetSeconds) * 100);
}

export function budgetLevel(percent: number): BudgetLevel {
  if (percent > 100) return 'over';
  if (percent >= 80) return 'warn';
  return 'ok';
}

/** Adds the budget percent/level and sorts: tracked first, most budget used first. */
export function withBudget(cards: TicketCard[]): ActiveTicket[] {
  return cards
    .map((c) => {
      const percent = budgetPercent(c);
      return {
        ...c,
        percent,
        level: budgetLevel(percent),
        elapsedText: shortDuration(c.elapsedSeconds),
        budgetText: shortDuration(c.budgetSeconds),
        remainingText: shortDuration(c.budgetSeconds - c.elapsedSeconds),
        startedText: whenLabel(c.startedAt),
        deadlineText: whenLabel(c.deadlineAt) || c.deadlineLabel,
      };
    })
    .sort((a, b) => Number(b.hasBudgetDetail) - Number(a.hasBudgetDetail) || b.percent - a.percent);
}

/**
 * One entry of the active/review card: a ticket (`head`) with the branches
 * hanging under it, or — when the parent isn't in this card — just the
 * parent's key/summary/status as a compact header.
 */
export interface TicketNode {
  key: string;
  head: ActiveTicket | null;
  parentSummary: string;
  parentStatus: string;
  branches: ActiveTicket[];
}

/**
 * Groups already-sorted rows into parent → branch nodes (Tech Spec "Subtask
 * & Linked Item Branches"), one level deep: a row is a branch when it has a
 * parent and isn't itself a parent of another row here. Node order follows
 * each node's first (worst) row.
 */
export function toTree(rows: ActiveTicket[]): TicketNode[] {
  const parents = new Set(rows.map((r) => r.parentKey).filter((k): k is string => !!k));
  const nodes = new Map<string, TicketNode>();
  const nodeFor = (key: string) => {
    let node = nodes.get(key);
    if (!node) {
      node = { key, head: null, parentSummary: '', parentStatus: '', branches: [] };
      nodes.set(key, node);
    }
    return node;
  };
  for (const row of rows) {
    if (row.parentKey && !parents.has(row.issueKey)) {
      const node = nodeFor(row.parentKey);
      node.branches.push(row);
      node.parentSummary ||= row.parentSummary ?? '';
      node.parentStatus ||= row.parentStatus ?? '';
    } else {
      nodeFor(row.issueKey).head = row;
    }
  }
  return [...nodes.values()];
}

/** "yusuf.wijaya@esb.co.id" → "Yusuf". */
export function firstName(email: string | undefined | null): string {
  const local = (email ?? '').split('@')[0].split(/[._-]/)[0];
  return local ? local.charAt(0).toUpperCase() + local.slice(1) : '';
}

/** "9–22 Nov" from ISO sprint dates (WIB); "" when unknown. */
export function sprintRange(start: string, end: string): string {
  if (!start || !end) return '';
  const fmt = (iso: string, withMonth: boolean) =>
    new Date(iso).toLocaleDateString('id-ID', { timeZone: 'Asia/Jakarta', day: 'numeric', ...(withMonth ? { month: 'short' } : {}) });
  return `${fmt(start, false)}–${fmt(end, true)}`;
}

/**
 * Dashboard landing page for the board picked in the navbar (Tech Spec
 * "Per-Board Settings & Active Sprint Dashboard"): the caller's tickets in
 * that board's ACTIVE sprint — or every active sprint for "Semua board" —
 * with stats, Code Process budgets, tickets by status, and the active
 * sprint summary. All data is scoped to the session server-side.
 */
@Component({
  selector: 'app-home',
  standalone: true,
  imports: [DecimalPipe, NgTemplateOutlet, RouterLink, IconComponent, BoardAvatarComponent],
  templateUrl: './home.component.html',
})
export class HomeComponent {
  private dashboard = inject(DashboardService);
  private auth = inject(AuthService);
  private confirm = inject(ConfirmService);
  private toast = inject(ToastService);
  protected muteState = inject(MuteStateService);
  private notifSwitch = inject(NotificationSwitchService);
  protected boardContext = inject(BoardContextService);

  private user = toSignal(this.auth.currentUser$, { initialValue: null });
  protected name = computed(() => firstName(this.user()?.email));

  protected boardLoading = signal(true);
  protected boardError = signal<string | null>(null);
  protected cards = signal<TicketCard[]>([]);
  /** Subtasks/linked items outside the sprint: only feed the active/review cards. */
  protected related = signal<TicketCard[]>([]);
  protected movingKey = signal<string | null>(null);
  protected expanded = signal<Record<string, boolean>>({});

  protected reportLoading = signal(true);
  protected report = signal<SprintReport | null>(null);
  /** "Semua board": which board's sprint the summary tab shows. */
  protected reportTab = signal<number | null>(null);

  /** Board whose active sprint the summary covers. */
  protected reportBoard = computed<BoardSummary | null>(() => {
    const selected = this.boardContext.selected();
    if (selected) return selected;
    const boards = this.boardContext.boards();
    return boards.find((b) => b.boardId === this.reportTab()) ?? boards[0] ?? null;
  });

  /** Mute state shown in the banner: the global mute, or the selected board's. */
  protected notifState = computed<'on' | 'muted' | 'off'>(() => {
    // Master switch off (Tech Spec "Notification On-Off Consent") wins.
    if (this.notifSwitch.state()?.enabled === false) return 'off';
    if (this.muteState.forever()) return 'off';
    if (this.muteState.active()) return 'muted';
    const board = this.boardContext.selected();
    if (board && activeUntil(board.muted)) return board.muted.forever ? 'off' : 'muted';
    return 'on';
  });

  protected readonly preview = GROUP_PREVIEW;

  /** Code Process tickets: over budget first, then by budget used. */
  protected active = computed<ActiveTicket[]>(() =>
    withBudget([...this.cards(), ...this.related()].filter((c) => c.status.toLowerCase() === CODE_PROCESS)),
  );

  /**
   * Tickets in code review on boards whose project counts review time
   * (CORE) — Tech Spec "CORE: Waktu Code Review Ikut Dihitung".
   */
  protected reviewing = computed<ActiveTicket[]>(() =>
    withBudget(
      [...this.cards(), ...this.related()].filter(
        (c) => c.status.toLowerCase() === CODE_REVIEW && !!this.boardOf(c)?.reviewTracked,
      ),
    ),
  );

  /** The same rows as parent → branch trees. */
  protected activeTree = computed(() => toTree(this.active()));
  protected reviewTree = computed(() => toTree(this.reviewing()));

  /** The review card shows for a CORE board, or in "Semua board" when there's something in review. */
  protected showReview = computed(() => {
    const selected = this.boardContext.selected();
    return selected ? !!selected.reviewTracked : this.reviewing().length > 0;
  });

  /** Over budget = Code Process + tracked review. */
  protected overdueCount = computed(
    () => [...this.active(), ...this.reviewing()].filter((t) => t.hasBudgetDetail && t.level === 'over').length,
  );

  protected groups = computed<StatusGroup[]>(() => {
    const byStatus = new Map<string, TicketCard[]>();
    for (const card of this.cards()) {
      const list = byStatus.get(card.status) ?? [];
      list.push(card);
      byStatus.set(card.status, list);
    }
    const rank = (status: string) => {
      const i = STATUS_ORDER.indexOf(status.toLowerCase());
      return i === -1 ? STATUS_ORDER.length : i;
    };
    return [...byStatus.entries()]
      .map(([status, cards]) => ({ status, cards }))
      .sort((a, b) => rank(a.status) - rank(b.status) || a.status.localeCompare(b.status));
  });

  /**
   * "Tiket saya per status": each sprint ticket's out-of-sprint subtasks/
   * linked items (any status), keyed by parent. They hang under the parent
   * and are not counted in the groups.
   */
  protected branchesOf = computed(() => {
    const byParent = new Map<string, TicketCard[]>();
    for (const card of this.related()) {
      if (!card.parentKey) continue;
      byParent.set(card.parentKey, [...(byParent.get(card.parentKey) ?? []), card]);
    }
    return byParent;
  });

  /**
   * Every ticket of the sprint, largest deviation first, so the user sees
   * all of them — not just a top 5. `over` = actual above the estimate (the
   * API's deviation is unsigned).
   */
  protected sprintRows = computed(() =>
    [...(this.report()?.rows ?? [])]
      .map((r) => ({ ...r, over: r.actualHours > r.estimatedHours }))
      .sort((a, b) => Math.abs(b.deviationPercent) - Math.abs(a.deviationPercent)),
  );

  constructor() {
    // Tickets follow the navbar board.
    effect(() => {
      const boardId = this.boardContext.selectedId();
      untracked(() => this.loadBoard(boardId));
    });
    // The sprint summary follows the board (and its active sprint).
    effect(() => {
      const board = this.reportBoard();
      const loaded = this.boardContext.loaded();
      untracked(() => this.loadReport(board, loaded));
    });
  }

  protected boardOf(card: TicketCard): BoardSummary | null {
    return this.boardContext.boards().find((b) => b.boardId === card.boardId) ?? null;
  }

  /**
   * Jira link of any key, built from a link the board already gave us
   * (".../browse/OMS-1" → ".../browse/<key>"); "" when there is none yet.
   */
  protected jiraUrl(issueKey: string): string {
    const sample = this.cards()[0]?.issueURL || this.related()[0]?.issueURL || '';
    return sample ? sample.replace(/[^/]+$/, issueKey) : '';
  }

  /** Jira link of a header-only parent: its branch's link with the parent key. */
  protected parentURL(node: TicketNode): string {
    const url = node.branches[0]?.issueURL ?? '';
    return url ? url.replace(/[^/]+$/, node.key) : '';
  }

  protected sprintRange(start: string, end: string): string {
    return sprintRange(start, end);
  }

  protected loadBoard(boardId: number | null = this.boardContext.selectedId()): void {
    this.boardLoading.set(true);
    this.boardError.set(null);
    this.dashboard.getBoard(boardId).subscribe({
      next: (res) => {
        this.cards.set((res.columns ?? []).flatMap((col) => col.cards ?? []));
        this.related.set(res.related ?? []);
        this.boardLoading.set(false);
      },
      error: () => {
        this.boardError.set('Gagal memuat tiket dari Jira, coba lagi');
        this.boardLoading.set(false);
      },
    });
  }

  /** At least one ticket has tracked time in the sprint — otherwise show "–", never a fake 100%. */
  protected hasTrackedTickets = computed(() => (this.report()?.totalCount ?? 0) > 0);
  protected backfilling = signal(false);

  /** "Hitung ulang dari Jira": fill in time the bot missed, then reload the summary. */
  protected backfillSprint(): void {
    if (this.backfilling()) return;
    this.backfilling.set(true);
    this.dashboard.backfillSprint().subscribe({
      next: ({ added, updated }) => {
        this.backfilling.set(false);
        if (added > 0 || updated > 0) {
          const parts = [
            added > 0 ? `${added} tiket ditambahkan` : '',
            updated > 0 ? `${updated} tiket dihitung ulang` : '',
          ].filter(Boolean);
          this.toast.success('Data sprint diperbarui', `${parts.join(', ')} dari riwayat Jira.`);
        } else {
          this.toast.info('Sudah lengkap', 'Tidak ada tiket baru yang perlu dihitung dari Jira.');
        }
        this.loadReport();
      },
      error: () => {
        this.backfilling.set(false);
        this.toast.error('Gagal menghitung ulang', 'Data dari Jira belum bisa diambil, coba lagi.');
      },
    });
  }

  protected loadReport(board: BoardSummary | null = this.reportBoard(), loaded = this.boardContext.loaded()): void {
    if (!board) {
      this.report.set(null);
      this.reportLoading.set(!loaded);
      return;
    }
    this.reportLoading.set(true);
    this.dashboard.getSprintReport([board.activeSprint.name]).subscribe({
      next: (res) => {
        this.report.set(res.report);
        this.reportLoading.set(false);
      },
      error: () => {
        this.report.set(null);
        this.reportLoading.set(false);
      },
    });
  }

  protected isExpanded(status: string): boolean {
    return !!this.expanded()[status];
  }

  protected toggleGroup(status: string): void {
    this.expanded.set({ ...this.expanded(), [status]: !this.isExpanded(status) });
  }

  protected async move(ticket: TicketCard, target: TransitionTarget): Promise<void> {
    const labels: Record<TransitionTarget, string> = { 'code-review': 'Code Review', 'code-completed': 'Code Completed', todo: 'To Do' };
    const label = labels[target];
    const confirmed = await this.confirm.ask({
      title: `Geser ke ${label}?`,
      message: `${ticket.issueKey} — ${ticket.summary} akan dipindah ke ${label} di Jira.`,
      confirmText: `Geser ke ${label}`,
      icon: 'send',
    });
    if (!confirmed) return;

    this.movingKey.set(ticket.issueKey);
    this.dashboard.transitionTicket(ticket.issueKey, target).subscribe({
      next: (res) => {
        this.movingKey.set(null);
        this.toast.success(`${ticket.issueKey} digeser ke ${label}`, res?.message);
        this.loadBoard();
      },
      error: (err) => {
        this.movingKey.set(null);
        this.toast.error(`Gagal menggeser ${ticket.issueKey}`, err?.error?.error ?? err?.error?.message ?? 'Coba lagi sebentar.');
      },
    });
  }
}
