import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideRouter } from '@angular/router';
import { BehaviorSubject, of, throwError } from 'rxjs';
import { HomeComponent, budgetLevel, budgetPercent, firstName, sprintRange } from './home.component';
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
    dashboard = jasmine.createSpyObj('DashboardService', ['getBoard', 'getSprintReport', 'transitionTicket']);
    dashboard.getBoard.and.returnValue(of(BOARD));
    dashboard.getSprintReport.and.returnValue(of({ availableSprints: [], capPercent: 12, report: REPORT }));
    confirm = jasmine.createSpyObj('ConfirmService', ['ask']);
    toast = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);
    context = new FakeBoardContext();

    await TestBed.configureTestingModule({
      imports: [HomeComponent],
      providers: [
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

    it('only offers the To Do action for review tickets', () => {
      const actions = el().querySelectorAll('[data-card="review"] .ticket-row')[0].querySelectorAll('.ticket-actions button');
      expect(actions.length).toBe(1);
      expect(actions[0].textContent).toContain('To Do');
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
});
