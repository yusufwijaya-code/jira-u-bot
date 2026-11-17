import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideRouter } from '@angular/router';
import { BehaviorSubject, of, throwError } from 'rxjs';
import { HomeComponent, budgetLevel, budgetPercent, firstName, shortDuration, sprintRange, toTree, whenLabel, withBudget } from './home.component';
import { DashboardService, TicketCard } from '../../../core/services/dashboard.service';
import { BoardContextService, BoardSummary } from '../../../core/services/board-context.service';
import { AuthService } from '../../../core/services/auth.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';

function card(partial: Partial<TicketCard>): TicketCard {
  return {
    issueKey: 'OMS-1',
    summary: 'Ticket',
    status: 'To Do',
    issueURL: 'https://jira/browse/OMS-1',
    boardId: 28,
    hasBudgetDetail: false,
    storyPoints: 0,
    elapsedSeconds: 0,
    budgetSeconds: 0,
    elapsedLabel: '',
    remainingLabel: '',
    deadlineLabel: '',
    ...partial,
  };
}

const BOARD = {
  columns: [
    {
      status: 'Code Process',
      cards: [
        card({ issueKey: 'OMS-10', status: 'Code Process', hasBudgetDetail: true, budgetSeconds: 3600, elapsedSeconds: 1800, storyPoints: 1 }),
        card({ issueKey: 'OMS-11', status: 'Code Process', hasBudgetDetail: true, budgetSeconds: 3600, elapsedSeconds: 4500, storyPoints: 1 }),
        card({ issueKey: 'OMS-12', status: 'Code Process' }),
      ],
    },
    { status: 'Code Review', cards: [card({ issueKey: 'OMS-20', status: 'Code Review' })] },
    {
      status: 'CODE REVIEW',
      cards: [
        card({ issueKey: 'CORE-40', status: 'CODE REVIEW', boardId: 26, hasBudgetDetail: true, budgetSeconds: 3600, elapsedSeconds: 3000, storyPoints: 1 }),
        card({ issueKey: 'CORE-41', status: 'CODE REVIEW', boardId: 26, hasBudgetDetail: true, budgetSeconds: 3600, elapsedSeconds: 5400, storyPoints: 1 }),
      ],
    },
    { status: 'To Do', cards: [card({ issueKey: 'OMS-30' }), card({ issueKey: 'CORE-31', boardId: 26 })] },
  ],
};

const REPORT = {
  rows: [
    { sprintName: 'S', issueKey: 'OMS-1', task: 'a', estimatedHours: 2, actualHours: 2, deviationPercent: 0, within: true },
    { sprintName: 'S', issueKey: 'OMS-2', task: 'b', estimatedHours: 2, actualHours: 4, deviationPercent: 100, within: false },
  ],
  sumEstimatedHours: 4,
  sumActualHours: 6,
  aggregateDeviationPercent: 50,
  achievementPercent: 50,
  capPercent: 12,
  withinCount: 1,
  totalCount: 2,
};

const OMS: BoardSummary = {
  boardId: 28,
  name: 'ESB OMS Board',
  projectKey: 'OMS',
  activeSprint: { id: 8294, name: 'OMS Sprint 23 - 2026', startDate: '2026-11-09T01:00:00.000Z', endDate: '2026-11-22T10:00:00.000Z' },
  ticketCount: 5,
  hasCustomSettings: false,
  muted: { active: false, until: null },
};
const CORE: BoardSummary = { ...OMS, boardId: 26, name: 'ESB Core Board', projectKey: 'CORE', reviewTracked: true, activeSprint: { ...OMS.activeSprint, id: 7988, name: 'Core Sprint 23 2026' } };

class FakeBoardContext {
  boards = signal<BoardSummary[]>([OMS, CORE]);
  loaded = signal(true);
  selectedId = signal<number | null>(null);
  selected = () => this.boards().find((b) => b.boardId === this.selectedId()) ?? null;
  avatarUrl = (id: number) => `/avatar/${id}`;
  select = (id: number | null) => this.selectedId.set(id);
  load = () => undefined;
}

describe('dashboard helpers', () => {
  it('groups rows into parent → branch nodes, one level deep, keeping the worst-first order', () => {
    const rows = withBudget([
      card({ issueKey: 'A', status: 'Code Process', hasBudgetDetail: true, budgetSeconds: 100, elapsedSeconds: 10 }),
      card({ issueKey: 'B', status: 'Code Process', hasBudgetDetail: true, budgetSeconds: 100, elapsedSeconds: 150, parentKey: 'P', parentSummary: 'Parent P', parentStatus: 'To Do', relation: 'subtask' }),
      card({ issueKey: 'C', status: 'Code Process', parentKey: 'A', relation: 'relates to' }),
      card({ issueKey: 'D', status: 'Code Process', parentKey: 'X', relation: 'subtask' }),
      card({ issueKey: 'X2', status: 'Code Process', parentKey: 'D', relation: 'subtask' }),
    ]);
    const tree = toTree(rows);
    expect(tree.map((n) => [n.key, n.head?.issueKey ?? null, n.branches.map((b) => b.issueKey)])).toEqual([
      ['P', null, ['B']],
      ['A', 'A', ['C']],
      ['D', 'D', ['X2']],
    ]);
    expect(tree[0].parentSummary).toBe('Parent P');
    expect(tree[0].parentStatus).toBe('To Do');
  });

  it('formats short durations', () => {
    expect(shortDuration(9300)).toBe('2 jam 35 menit');
    expect(shortDuration(-900)).toBe('15 menit');
    expect(shortDuration(3600)).toBe('1 jam');
    expect(shortDuration(90_000)).toBe('1 hari 1 jam');
    expect(shortDuration(30)).toBe('0 menit');
  });

  it('formats an instant relative to today in WIB', () => {
    const now = new Date('2026-11-14T03:00:00Z'); // Sab, 14 Nov 10:00 WIB
    expect(whenLabel('2026-11-14T09:05:00Z', now)).toBe('Hari ini 16:05');
    expect(whenLabel('2026-11-15T01:30:00Z', now)).toBe('Besok 08:30');
    expect(whenLabel('2026-11-13T02:10:00Z', now)).toBe('Kemarin 09:10');
    expect(whenLabel('2026-11-17T04:00:00Z', now)).toMatch(/^Sel, 17 Nov 11:00$/);
    expect(whenLabel(undefined, now)).toBe('');
  });

  it('computes budget percent and level', () => {
    expect(budgetPercent(card({ hasBudgetDetail: true, budgetSeconds: 3600, elapsedSeconds: 900 }))).toBe(25);
    expect(budgetPercent(card({ hasBudgetDetail: false }))).toBe(0);
    expect(budgetLevel(50)).toBe('ok');
    expect(budgetLevel(85)).toBe('warn');
    expect(budgetLevel(101)).toBe('over');
  });

  it('derives a first name from the email', () => {
    expect(firstName('yusuf.wijaya@esb.co.id')).toBe('Yusuf');
    expect(firstName(null)).toBe('');
  });

  it('formats a sprint date range in WIB', () => {
    expect(sprintRange(OMS.activeSprint.startDate, OMS.activeSprint.endDate)).toBe('9–22 Nov');
    expect(sprintRange('', '')).toBe('');
  });
});

describe('HomeComponent', () => {
  let fixture: ComponentFixture<HomeComponent>;
  let dashboard: jasmine.SpyObj<DashboardService>;
  let confirm: jasmine.SpyObj<ConfirmService>;
  let toast: jasmine.SpyObj<ToastService>;
  let context: FakeBoardContext;
  const el = () => fixture.nativeElement as HTMLElement;
  const stat = (name: string) => el().querySelector(`[data-stat="${name}"]`)?.textContent?.trim();

  beforeEach(async () => {
    dashboard = jasmine.createSpyObj('DashboardService', ['getBoard', 'getSprintReport', 'transitionTicket', 'backfillSprint']);
    dashboard.getBoard.and.returnValue(of(BOARD));
    dashboard.getSprintReport.and.returnValue(of({ availableSprints: [], capPercent: 12, report: REPORT }));
    confirm = jasmine.createSpyObj('ConfirmService', ['ask']);
    toast = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);
    context = new FakeBoardContext();

    await TestBed.configureTestingModule({
      imports: [HomeComponent],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        provideRouter([]),
        { provide: DashboardService, useValue: dashboard },
        { provide: BoardContextService, useValue: context },
        { provide: ConfirmService, useValue: confirm },
        { provide: ToastService, useValue: toast },
        { provide: AuthService, useValue: { currentUser$: new BehaviorSubject({ email: 'yusuf.wijaya@esb.co.id', isAdmin: false }) } },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(HomeComponent);
    fixture.detectChanges();
  });

  describe('live refresh', () => {
    beforeEach(() => {
      jasmine.clock().install();
      fixture.destroy();
      dashboard.getBoard.calls.reset();
      dashboard.getSprintReport.calls.reset();
      fixture = TestBed.createComponent(HomeComponent);
      fixture.detectChanges();
    });

    afterEach(() => {
      fixture.destroy();
      jasmine.clock().uninstall();
    });

    it('reloads tickets and the sprint summary every 30 seconds without a skeleton', () => {
      expect(dashboard.getBoard).toHaveBeenCalledTimes(1);
      expect(dashboard.getSprintReport).toHaveBeenCalledTimes(1);

      jasmine.clock().tick(30_000);
      fixture.detectChanges();

      expect(dashboard.getBoard).toHaveBeenCalledTimes(2);
      expect(dashboard.getSprintReport).toHaveBeenCalledTimes(2);
      expect(el().querySelector('.table-skeleton')).toBeNull();
    });

    it('keeps the last tickets when a background reload fails', () => {
      dashboard.getBoard.and.returnValue(throwError(() => ({ status: 502 })));
      jasmine.clock().tick(30_000);
      fixture.detectChanges();

      expect(stat('active')).toBe('3');
      expect(el().textContent).not.toContain('Gagal memuat tiket');
    });

    it('stops reloading once the page is left', () => {
      fixture.destroy();
      jasmine.clock().tick(60_000);
      expect(dashboard.getBoard).toHaveBeenCalledTimes(1);
    });
  });

  it('has no manual reload button on "Sedang dikerjakan" (it refreshes itself)', () => {
    expect(el().querySelector('[aria-label="Muat ulang"]')).toBeNull();
  });

  it('greets the user and describes all active sprints when no board is selected', () => {
    expect(el().querySelector('.hero-strip h2')?.textContent).toContain('Halo, Yusuf');
    expect(el().querySelector('.hero-strip p')?.textContent).toContain('semua sprint aktif (2 board)');
    expect(dashboard.getBoard).toHaveBeenCalledWith(null);
  });

  it('shows the stats from the board and the sprint report', () => {
    expect(stat('active')).toBe('3');
    expect(stat('overdue')).toBe('2'); // OMS-11 (Code Process) + CORE-41 (review)
    expect(stat('total')).toBe('8');
    expect(stat('achievement')).toBe('50%');
  });

  it("uses the first board's active sprint for the summary and offers a tab per board", () => {
    expect(dashboard.getSprintReport).toHaveBeenCalledWith(['OMS Sprint 23 - 2026']);
    const tabs = Array.from(el().querySelectorAll('.board-tab'));
    expect(tabs.length).toBe(2);

    (tabs[1] as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(dashboard.getSprintReport).toHaveBeenCalledWith(['Core Sprint 23 2026']);
  });

  it('reloads tickets and the sprint summary for the board picked in the navbar', () => {
    context.selectedId.set(26);
    fixture.detectChanges();

    expect(dashboard.getBoard).toHaveBeenCalledWith(26);
    expect(dashboard.getSprintReport).toHaveBeenCalledWith(['Core Sprint 23 2026']);
    expect(el().querySelector('.hero-strip p')?.textContent).toContain('ESB Core Board — Core Sprint 23 2026');
    expect(el().querySelectorAll('.board-tab').length).toBe(0);
  });

  it('shows the board icon on each ticket only in "Semua board"', () => {
    expect(el().querySelectorAll('[data-card="active"] .ticket-board').length).toBe(3);
    context.selectedId.set(28);
    fixture.detectChanges();
    expect(el().querySelectorAll('[data-card="active"] .ticket-board').length).toBe(0);
  });

  it('reports a muted selected board in the banner', () => {
    context.boards.set([{ ...OMS, muted: { active: true, until: new Date(Date.now() + 3600_000).toISOString() } }, CORE]);
    context.selectedId.set(28);
    fixture.detectChanges();
    expect(el().querySelector('.notif-status')?.textContent).toContain('Board ini dimute');
  });

  it('shows elapsed vs budget, remaining and deadline like the DM reminder', () => {
    const [over, ok] = Array.from(el().querySelectorAll('[data-card="active"] .ticket-row'));
    const meta = (row: Element, key: string) => row.querySelector(`[data-meta="${key}"]`)?.textContent?.replace(/\s+/g, ' ').trim();
    expect(meta(ok, 'elapsed')).toBe('Berjalan 30 menit dari 1 jam (1 SP)');
    expect(meta(ok, 'remaining')).toBe('Sisa 30 menit');
    expect(meta(over, 'elapsed')).toBe('Berjalan 1 jam 15 menit dari 1 jam (1 SP)');
    expect(meta(over, 'remaining')).toBe('Lewat 15 menit');
    expect(meta(over, 'deadline')).toBeUndefined();
  });

  it('lists Code Process tickets over budget first, untracked last, with budget levels', () => {
    const rows = Array.from(el().querySelectorAll('[data-card="active"] .ticket-row'));
    expect(rows.map((r) => r.querySelector('.ticket-key')?.textContent?.trim())).toEqual(['OMS-11', 'OMS-10', 'OMS-12']);
    expect(rows.map((r) => r.getAttribute('data-level'))).toEqual(['over', 'ok', 'untracked']);
  });

  it('groups every ticket by status in workflow order', () => {
    const heads = Array.from(el().querySelectorAll('.status-name')).map((n) => n.textContent?.trim());
    expect(heads).toEqual(['To Do', 'Code Process', 'Code Review', 'CODE REVIEW']);
  });

  it('moves a ticket after confirmation and reloads the board', async () => {
    confirm.ask.and.resolveTo(true);
    dashboard.transitionTicket.and.returnValue(of({ message: 'ok' }));

    (el().querySelector('[data-card="active"] .ticket-actions .btn-soft') as HTMLButtonElement).click();
    await fixture.whenStable();

    expect(dashboard.transitionTicket).toHaveBeenCalledWith('OMS-11', 'code-review');
    expect(toast.success).toHaveBeenCalled();
    expect(dashboard.getBoard).toHaveBeenCalledTimes(2);
  });

  it('does not move a ticket when the confirmation is cancelled', async () => {
    confirm.ask.and.resolveTo(false);

    (el().querySelector('[data-card="active"] .ticket-actions .btn-ghost') as HTMLButtonElement).click();
    await fixture.whenStable();

    expect(dashboard.transitionTicket).not.toHaveBeenCalled();
  });

  it('shows an error state with retry when the board fails', () => {
    dashboard.getBoard.and.returnValue(throwError(() => ({ status: 500 })));
    fixture = TestBed.createComponent(HomeComponent);
    fixture.detectChanges();

    expect(el().querySelector('.msg.error')?.textContent).toContain('Gagal memuat tiket');
    expect(el().querySelector('[data-card="active"]')).toBeNull();
  });

  it('shows an empty state when nothing is in Code Process', () => {
    dashboard.getBoard.and.returnValue(of({ columns: [{ status: 'To Do', cards: [card({})] }] }));
    fixture = TestBed.createComponent(HomeComponent);
    fixture.detectChanges();

    expect(el().querySelector('[data-card="active"] .empty-inline')?.textContent).toContain('Tidak ada tiket di Code Process');
  });

  describe('Sedang di-review (projects that count review time)', () => {
    const reviewCard = () => el().querySelector('[data-card="review"]');
    const reviewKeys = () => Array.from(el().querySelectorAll('[data-card="review"] .ticket-key')).map((n) => n.textContent?.trim());

    it('shows CORE review tickets in "Semua board", over budget first, without OMS review tickets', () => {
      expect(reviewCard()).not.toBeNull();
      expect(reviewKeys()).toEqual(['CORE-41', 'CORE-40']);
      const levels = Array.from(el().querySelectorAll('[data-card="review"] .ticket-row')).map((r) => r.getAttribute('data-level'));
      expect(levels).toEqual(['over', 'warn']);
    });

    it('only offers the Code Completed action for review tickets', () => {
      const actions = el().querySelectorAll('[data-card="review"] .ticket-row')[0].querySelectorAll('.ticket-actions button');
      expect(actions.length).toBe(1);
      expect(actions[0].textContent).toContain('Code Completed');
      expect(actions[0].textContent).not.toContain('To Do');
    });

    it('sits right below "Sedang dikerjakan"', () => {
      const cards = Array.from(el().querySelectorAll('section.card[data-card]')).map((s) => s.getAttribute('data-card'));
      expect(cards.slice(0, 2)).toEqual(['active', 'review']);
    });

    it('always shows for a CORE board, even when empty', () => {
      dashboard.getBoard.and.returnValue(of({ columns: [] }));
      context.selectedId.set(26);
      fixture.detectChanges();
      expect(reviewCard()?.textContent).toContain('Tidak ada tiket yang sedang di-review');
    });

    it('is hidden for an OMS board', () => {
      context.selectedId.set(28);
      fixture.detectChanges();
      expect(reviewCard()).toBeNull();
    });

    it('is hidden in "Semua board" when nothing is in review', () => {
      dashboard.getBoard.and.returnValue(of({ columns: [{ status: 'To Do', cards: [card({})] }] }));
      fixture = TestBed.createComponent(HomeComponent);
      fixture.detectChanges();
      expect(reviewCard()).toBeNull();
    });
  });

  describe('subtask & linked item branches', () => {
    const RELATED_BOARD = {
      columns: [
        { status: 'Code Review', cards: [card({ issueKey: 'OMS-11727', summary: 'Upload Queue', status: 'Code Review' })] },
        {
          status: 'Code Process',
          cards: [
            card({ issueKey: 'OMS-50', status: 'Code Process', hasBudgetDetail: true, budgetSeconds: 3600, elapsedSeconds: 600, storyPoints: 1 }),
            card({ issueKey: 'OMS-51', status: 'Code Process', parentKey: 'OMS-50', parentSummary: 'Parent', parentStatus: 'Code Process', relation: 'subtask' }),
          ],
        },
        { status: 'To Do', cards: [card({ issueKey: 'OMS-60' })] },
      ],
      related: [
        card({
          issueKey: 'OMS-11905',
          issueURL: 'https://jira/browse/OMS-11905',
          status: 'Code Process',
          hasBudgetDetail: true,
          budgetSeconds: 3600,
          elapsedSeconds: 4000,
          storyPoints: 1,
          parentKey: 'OMS-11727',
          parentSummary: 'Upload Queue',
          parentStatus: 'Code Review',
          relation: 'relates to',
        }),
        card({ issueKey: 'OMS-11938', status: 'Code Process', parentKey: 'OMS-11727', parentSummary: 'Upload Queue', parentStatus: 'Code Review', relation: 'subtask' }),
        card({ issueKey: 'OMS-52', status: 'Code Process', parentKey: 'OMS-50', parentSummary: 'Parent', parentStatus: 'Code Process', relation: 'relates to' }),
        card({ issueKey: 'OMS-61', status: 'Done', parentKey: 'OMS-60', relation: 'subtask' }),
      ],
    };
    const activeCard = () => el().querySelector('[data-card="active"]') as HTMLElement;

    beforeEach(() => {
      dashboard.getBoard.and.returnValue(of(RELATED_BOARD));
      fixture = TestBed.createComponent(HomeComponent);
      fixture.detectChanges();
    });

    it('hangs related items under a compact header when the parent is not in the card', () => {
      const header = activeCard().querySelector('[data-parent="OMS-11727"]') as HTMLElement;
      expect(header).not.toBeNull();
      expect(header.textContent).toContain('Upload Queue');
      expect(header.textContent).toContain('Code Review');
      expect((header.querySelector('.ticket-key') as HTMLAnchorElement).href).toBe('https://jira/browse/OMS-11727');

      const branches = (header.nextElementSibling as HTMLElement).querySelectorAll('.ticket-branch');
      expect(Array.from(branches).map((b) => b.querySelector('.ticket-key')?.textContent?.trim())).toEqual(['OMS-11905', 'OMS-11938']);
      expect(branches[0].querySelector('[data-relation]')?.textContent?.trim()).toBe('relates to');
    });

    it('hangs branches under the full row when the parent is in the card', () => {
      const keys = Array.from(activeCard().querySelectorAll(':scope > ul > .ticket-row:not(.ticket-branch) .ticket-key')).map((k) => k.textContent?.trim());
      expect(keys).toEqual(['OMS-50']);
      const parentRow = activeCard().querySelector(':scope > ul > .ticket-row') as HTMLElement;
      const branches = (parentRow.nextElementSibling as HTMLElement).querySelectorAll('.ticket-branch .ticket-key');
      expect(Array.from(branches).map((b) => b.textContent?.trim())).toEqual(['OMS-51', 'OMS-52']);
    });

    it('hangs every related item (any status) under its parent in "Tiket saya per status"', () => {
      const status = el().querySelector('[data-card="status"]') as HTMLElement;
      const branchKeys = (parent: string) =>
        Array.from(status.querySelectorAll(`[data-branches-of="${parent}"] .mini-branch`)).map((b) => [
          b.querySelector('[data-relation]')?.textContent?.trim(),
          b.querySelector('.ticket-key')?.textContent?.trim(),
          b.querySelector('.status-chip')?.textContent?.trim(),
        ]);
      expect(branchKeys('OMS-11727')).toEqual([
        ['relates to', 'OMS-11905', 'Code Process'],
        ['subtask', 'OMS-11938', 'Code Process'],
      ]);
      expect(branchKeys('OMS-60')).toEqual([['subtask', 'OMS-61', 'Done']]);
      expect(status.querySelector('[data-branches-of="OMS-51"]')).toBeNull();
      // Branches don't add to the per-status counts.
      expect(Array.from(status.querySelectorAll('.status-head .count-pill')).map((p) => p.textContent?.trim())).toEqual(['1', '2', '1']);
    });

    it('counts branches in the active stats but not in the total or per-status list', () => {
      expect(stat('active')).toBe('5');
      expect(stat('overdue')).toBe('1');
      expect(stat('total')).toBe('4');
      expect(el().textContent).not.toContain('OMS-11905 ');
    });
  });
  describe('Sprint aktif card (Tech Spec "Active Sprint Tracking & Changelog Backfill")', () => {
    const sprintCard = () => el().querySelector('[data-card="sprint"]') as HTMLElement;

    it('lists every ticket of the sprint, largest deviation first, linked to Jira and signed by actual vs estimate', () => {
      const rows = Array.from(sprintCard().querySelectorAll('[data-sprint-rows] li'));
      expect(rows.length).toBe(2);
      expect(rows.map((r) => r.querySelector('.ticket-key')?.textContent?.trim())).toEqual(['OMS-2', 'OMS-1']);
      expect((rows[0].querySelector('a.ticket-key') as HTMLAnchorElement).href).toBe('https://jira/browse/OMS-2');
      expect(rows[0].querySelector('.dev-badge')?.textContent?.trim()).toBe('+100%');
      expect(rows[0].textContent).toContain('4 dari 2 jam');
      expect(rows[1].querySelector('.dev-badge')?.textContent?.trim()).toBe('0%');
    });

    it('shows "–" instead of a fake 100% when no ticket has tracked time', () => {
      dashboard.getSprintReport.and.returnValue(of({
        availableSprints: [], capPercent: 12,
        report: { ...REPORT, rows: [], totalCount: 0, withinCount: 0, aggregateDeviationPercent: 0, achievementPercent: 100 },
      }));
      fixture = TestBed.createComponent(HomeComponent);
      fixture.detectChanges();

      expect(stat('achievement')).toBe('–');
      expect(sprintCard().querySelector('[data-sprint-empty]')?.textContent).toContain('Belum ada tiket yang tercatat waktunya');
      expect(sprintCard().querySelector('.achievement-ring:not(.empty)')).toBeNull();
    });

    const clickSync = async () => {
      (sprintCard().querySelector('[data-backfill]') as HTMLButtonElement).click();
      await fixture.whenStable();
      fixture.detectChanges();
    };

    it('labels the sync button instead of showing a bare icon', () => {
      const btn = sprintCard().querySelector('[data-backfill]') as HTMLButtonElement;
      expect(btn.textContent).toContain('Sinkron Jira');
      expect(btn.getAttribute('title')).toContain('riwayat Jira');
    });

    it('explains what the sync does before running it', async () => {
      confirm.ask.and.resolveTo(false);
      await clickSync();

      const opts = confirm.ask.calls.mostRecent().args[0];
      expect(opts.title).toContain('Sinkron');
      expect(opts.points?.length).toBe(2);
      expect(opts.points?.join(' ')).toContain('bot sempat mati');
      expect(opts.points?.join(' ')).toContain('CODE REVIEW');
      expect(opts.note).toContain('07:00');
      expect(dashboard.backfillSprint).not.toHaveBeenCalled();
    });

    it('syncs after confirmation, reports how many were added and reloads the summary', async () => {
      confirm.ask.and.resolveTo(true);
      dashboard.backfillSprint.and.returnValue(of({ added: 8, updated: 0 }));
      const before = dashboard.getSprintReport.calls.count();

      await clickSync();

      expect(dashboard.backfillSprint).toHaveBeenCalled();
      expect(toast.success).toHaveBeenCalledWith('Data sprint diperbarui', '8 tiket ditambahkan dari riwayat Jira.');
      expect(dashboard.getSprintReport.calls.count()).toBe(before + 1);
    });

    it('reports recalculated tickets too', async () => {
      confirm.ask.and.resolveTo(true);
      dashboard.backfillSprint.and.returnValue(of({ added: 1, updated: 7 }));
      await clickSync();
      expect(toast.success).toHaveBeenCalledWith(
        'Data sprint diperbarui',
        '1 tiket ditambahkan, 7 tiket dihitung ulang dari riwayat Jira.',
      );
    });

    it('tells the user when the sync fails', async () => {
      confirm.ask.and.resolveTo(true);
      dashboard.backfillSprint.and.returnValue(throwError(() => ({ status: 500 })));
      await clickSync();
      expect(toast.error).toHaveBeenCalled();
    });
  });
});
