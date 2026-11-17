import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ChatAppService } from '../../../core/services/chat-app.service';
import { signal } from '@angular/core';
import { Subject, of, throwError } from 'rxjs';
import { SettingsComponent } from './settings.component';
import { BoardService, NotificationSettings, SettingsResponse } from '../../../core/services/board.service';
import { BoardContextService, BoardSummary, OtherBoard } from '../../../core/services/board-context.service';
import { MuteStateService } from '../../../core/services/mute-state.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { NotificationSwitchService } from '../../../core/services/notification-switch.service';
import { SETTINGS_MODE } from '../../../core/services/settings-api';

const NOTIFICATION: NotificationSettings = {
  reminderTiers: ['2h', '45m', '1m'],
  overageTiers: ['15m'],
  overageRepeatMinutes: 30,
  notifyStatusChange: false,
  statusChangeStatuses: [],
  quietHours: null,
};

const OFF = { active: false, until: null };

function sample(overrides: Partial<SettingsResponse> = {}): SettingsResponse {
  return {
    scope: 'default',
    boardId: 0,
    useDefault: false,
    notification: { ...NOTIFICATION },
    mute: { global: OFF, board: null },
    credential: { hasCredential: false, maskedPreview: '' },
    ...overrides,
  };
}

const OMS: BoardSummary = {
  boardId: 28,
  name: 'ESB OMS Board',
  projectKey: 'OMS',
  activeSprint: { id: 8294, name: 'OMS Sprint 23 - 2026', startDate: '', endDate: '' },
  ticketCount: 5,
  hasCustomSettings: false,
  muted: OFF,
};

class FakeBoardContext {
  boards = signal<BoardSummary[]>([OMS]);
  otherBoards = signal<OtherBoard[]>([]);
  loaded = signal(true);
  selectedId = signal<number | null>(null);
  selected = () => this.boards().find((b) => b.boardId === this.selectedId()) ?? null;
  avatarUrl = (id: number) => `/avatar/${id}`;
  select = jasmine.createSpy('select').and.callFake((id: number | null) => this.selectedId.set(id));
  load = jasmine.createSpy('load');
}

describe('SettingsComponent', () => {
  let fixture: ComponentFixture<SettingsComponent>;
  let boardService: jasmine.SpyObj<BoardService>;
  let toast: jasmine.SpyObj<ToastService>;
  let confirm: jasmine.SpyObj<ConfirmService>;
  let context: FakeBoardContext;
  const el = () => fixture.nativeElement as HTMLElement;
  const card = (name: string) => el().querySelector(`[data-card="${name}"]`) as HTMLElement;
  const component = () => fixture.componentInstance as any;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', [
      'getSettings', 'saveNotificationSettings', 'resetNotificationSettings', 'mute', 'unmute', 'saveCredential', 'deleteCredential',
    ]);
    boardService.getSettings.and.callFake((boardId: number | null = null) =>
      of(boardId ? sample({ scope: 'board', boardId, useDefault: true, mute: { global: OFF, board: OFF } }) : sample()),
    );
    toast = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);
    confirm = jasmine.createSpyObj('ConfirmService', ['ask']);
    context = new FakeBoardContext();

    await TestBed.configureTestingModule({
      imports: [SettingsComponent],
      providers: [
        provideHttpClient(),
        { provide: ChatAppService, useValue: { load: () => of({ enabled: false }), activateUrl: () => '' } },
        provideHttpClientTesting(),
        { provide: BoardService, useValue: boardService },
        { provide: BoardContextService, useValue: context },
        { provide: ToastService, useValue: toast },
        { provide: ConfirmService, useValue: confirm },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(SettingsComponent);
    fixture.detectChanges();
    // Most tests are about the notification cards, on the "Per board" tab.
    openTab('board');
  });

  function openTab(name: 'umum' | 'board'): void {
    (el().querySelector(`[data-tab="${name}"]`) as HTMLButtonElement).click();
    fixture.detectChanges();
  }
  const cardNames = () => Array.from(el().querySelectorAll('section.card[data-card]')).map((s) => s.getAttribute('data-card'));

  it('splits Umum (every board) from Per board (notification cards)', () => {
    openTab('umum');
    expect(cardNames()).toEqual(['credential']);
    expect(el().querySelector('app-chat-sender-card')).not.toBeNull();
    expect(el().querySelector('.scope-bar')).toBeNull();

    openTab('board');
    expect(cardNames()).toEqual(['reminder', 'overage', 'events', 'quiet']);
    expect(el().querySelector('app-chat-sender-card')).toBeNull();
    expect(el().querySelector('.scope-bar')?.getAttribute('data-scope')).toBe('default');
    expect(el().querySelector('.scope-bar')?.textContent).toContain('Pengaturan default semua board');
  });

  it('opens on Umum, or on Per board when a board is already picked', () => {
    expect(TestBed.createComponent(SettingsComponent).componentInstance['tab']()).toBe('umum');
    context.selectedId.set(28);
    expect(TestBed.createComponent(SettingsComponent).componentInstance['tab']()).toBe('board');
  });

  it('picks the board to configure from the chips in the Per board tab', () => {
    const chips = Array.from(el().querySelectorAll('.settings-board')).map((c) => c.textContent!.replace(/\s+/g, ' ').trim());
    expect(chips[0]).toContain('Default semua board');
    expect(chips[1]).toContain('ESB OMS Board');
    expect(chips[1]).toContain('Ikut default');

    (el().querySelector('[data-scope-chip="28"]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(context.select).toHaveBeenCalledWith(28);
    expect(el().querySelector('[data-scope-chip="28"]')!.classList).toContain('active');
    expect(component().tab()).toBe('board');
  });

  it('checks the saved pre-budget tiers and shows a custom tier tile', () => {
    const tiles = Array.from(card('reminder').querySelectorAll('.tier-tile'));
    const checked = tiles
      .filter((t) => (t.querySelector('input') as HTMLInputElement).checked)
      .map((t) => t.querySelector('strong')!.textContent!.trim());
    expect(checked).toEqual(['T-2 jam', 'T-45 menit', 'T-1 menit']);
  });

  it('shows the overage flow preview from tiers and repeat', () => {
    expect(card('overage').querySelector('.timeline-text')?.textContent).toContain('Budget habis → +15 menit → tiap 30 menit');
  });

  it('saves the default scope without a board', () => {
    boardService.saveNotificationSettings.and.callFake((s) => of(s));
    component().patch({ notifyStatusChange: true });

    component().save('events');

    expect(boardService.saveNotificationSettings).toHaveBeenCalledWith({ ...NOTIFICATION, notifyStatusChange: true }, null);
    expect(toast.success).toHaveBeenCalled();
  });

  it('shows a server field error on the card that owns the field', () => {
    boardService.saveNotificationSettings.and.returnValue(
      throwError(() => ({ status: 400, error: { error: 'x', fields: { overageRepeatMinutes: 'interval tidak valid' } } })),
    );
    component().save('overage');
    fixture.detectChanges();

    expect(card('overage').querySelector('.card-footer .msg.error')?.textContent).toContain('interval tidak valid');
  });

  describe('board scope', () => {
    beforeEach(() => {
      context.selectedId.set(28);
      fixture.detectChanges();
    });

    it('loads the board settings and shows it follows the default, read-only', () => {
      expect(boardService.getSettings).toHaveBeenCalledWith(28);
      expect(el().querySelector('.scope-bar')?.getAttribute('data-scope')).toBe('following');
      expect(el().querySelector('.scope-bar')?.textContent).toContain('ESB OMS Board mengikuti pengaturan default');
      expect(card('reminder').classList).toContain('readonly');
      expect(card('reminder').hasAttribute('inert')).toBeTrue();
      expect(card('reminder').querySelector('.btn-primary')).toBeNull();
    });

    it('"Atur khusus" makes the cards editable and saving makes the board custom', () => {
      (el().querySelector('.scope-actions .btn-primary') as HTMLButtonElement).click();
      fixture.detectChanges();
      expect(card('reminder').classList).not.toContain('readonly');

      boardService.saveNotificationSettings.and.callFake((s) => of(s));
      component().save('reminder');
      fixture.detectChanges();

      expect(boardService.saveNotificationSettings).toHaveBeenCalledWith(jasmine.any(Object), 28);
      expect(el().querySelector('.scope-bar')?.getAttribute('data-scope')).toBe('custom');
      expect(el().querySelector('.scope-badge')?.textContent).toContain('Pengaturan khusus');
      expect(context.load).toHaveBeenCalled();
    });

    it('"Kembalikan ke default" asks first, then resets and reloads', async () => {
      boardService.getSettings.and.returnValue(of(sample({ scope: 'board', boardId: 28, useDefault: false, mute: { global: OFF, board: OFF } })));
      component().loadSettings(28);
      fixture.detectChanges();
      confirm.ask.and.resolveTo(true);
      boardService.resetNotificationSettings.and.returnValue(of(NOTIFICATION));

      (el().querySelector('.scope-actions .btn-ghost') as HTMLButtonElement).click();
      await fixture.whenStable();

      expect(confirm.ask).toHaveBeenCalled();
      expect(boardService.resetNotificationSettings).toHaveBeenCalledWith(28);
      expect(context.load).toHaveBeenCalled();
    });

    it('shows the board mute panel plus the global mute status', () => {
      expect(card('quiet').textContent).toContain('Mute board ini');
      expect(card('quiet').querySelector('.global-mute-status')?.textContent).toContain('Mute semua board tidak aktif');

      (card('quiet').querySelector('.global-mute-status .link-btn') as HTMLButtonElement).click();
      expect(context.select).toHaveBeenCalledWith(null);
    });
  });

  it('publishes the global mute to the shared state', () => {
    const until = new Date(Date.now() + 3600_000).toISOString();
    boardService.getSettings.and.returnValue(of(sample({ mute: { global: { active: true, until }, board: null } })));
    component().loadSettings(null);

    expect(TestBed.inject(MuteStateService).active()).toBeTrue();
  });

  it('shows an error and hides the cards when loading fails', () => {
    boardService.getSettings.and.returnValue(throwError(() => ({ status: 500 })));
    fixture = TestBed.createComponent(SettingsComponent);
    fixture.detectChanges();

    expect(el().querySelector('.msg.error')?.textContent).toContain('Gagal memuat pengaturan, coba lagi');
    expect(card('reminder')).toBeNull();
  });

  // --- review fixes ---

  it('keeps the newest scope when an older settings response arrives late (#2)', () => {
    const slow = new Subject<SettingsResponse>();
    boardService.getSettings.and.callFake((boardId: number | null = null) =>
      boardId === 28 ? slow : of(sample({ scope: 'board', boardId: 99, useDefault: false, mute: { global: OFF, board: OFF } })),
    );
    component().loadSettings(28);
    component().loadSettings(99);
    slow.next(sample({ scope: 'board', boardId: 28, useDefault: true, mute: { global: OFF, board: OFF } }));

    expect(component().scopeBoardId()).toBe(99);
  });

  it('saves only the clicked card and keeps other cards as drafts (#5)', () => {
    boardService.saveNotificationSettings.and.callFake((s) => of(s));
    component().patch({ quietHours: { start: '09:00', end: '10:00' } }); // unsaved, quiet card
    component().patch({ notifyStatusChange: true });

    component().save('events');

    expect(boardService.saveNotificationSettings).toHaveBeenCalledWith({ ...NOTIFICATION, notifyStatusChange: true }, null);
    // the quiet hours edit is still pending in the draft
    expect(component().draft().quietHours).toEqual({ start: '09:00', end: '10:00' });
  });

  it('suggests 09:00–10:00 quiet hours and resets them per scope (#4)', () => {
    component().setQuietEnabled(true);
    expect(component().draft().quietHours).toEqual({ start: '09:00', end: '10:00' });
  });

  it('names the tracked statuses and offers the statuses of each project (#3)', () => {
    fixture.detectChanges();
    expect(el().querySelector('[data-hint="status-change"]')?.textContent).toContain('status yang dihitung waktunya');

    context.boards.set([OMS, { ...OMS, boardId: 26, name: 'ESB Core Board', projectKey: 'CORE', reviewTracked: true }]);
    context.selectedId.set(26);
    fixture.detectChanges();
    expect(el().querySelector('[data-hint="status-change"]')?.textContent).toContain('Code Process atau CODE REVIEW');
    expect(component().statusOptions()).toContain('Code Completed');

    context.selectedId.set(28);
    fixture.detectChanges();
    expect(component().statusOptions()).toContain('Testing & Review');
  });

  it('picks destination statuses for the status-change notice', () => {
    component().patch({ notifyStatusChange: true });
    fixture.detectChanges();
    const box = (status: string) => el().querySelector(`[data-status-filter] input[data-status="${status}"]`) as HTMLInputElement;
    expect(box('Done').checked).toBeTrue(); // empty filter = every status

    box('Done').click();
    fixture.detectChanges();
    const picked = component().draft().statusChangeStatuses;
    expect(picked).not.toContain('Done');
    expect(picked.length).toBe(component().statusOptions().length - 1);
    expect(box('Done').checked).toBeFalse();

    (el().querySelector('[data-pick-all]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(component().draft().statusChangeStatuses).toEqual([]);
  });

  it('keeps at least one status picked', () => {
    component().patch({ notifyStatusChange: true, statusChangeStatuses: ['Done'] });
    fixture.detectChanges();
    (el().querySelector('[data-status-filter] input[data-status="Done"]') as HTMLInputElement).click();
    expect(component().draft().statusChangeStatuses).toEqual(['Done']);
  });

  it('hides the status filter while the notice is off', () => {
    fixture.detectChanges();
    expect(el().querySelector('[data-status-filter]')).toBeNull();
  });

  it('lists boards outside the active sprints and can reset or unmute them (#7)', async () => {
    const until = new Date(Date.now() + 3600_000).toISOString();
    context.otherBoards.set([
      { boardId: 40, name: 'Old Board', projectKey: 'OLD', hasCustomSettings: true, muted: OFF },
      { boardId: 41, name: 'Muted Board', projectKey: 'MUT', hasCustomSettings: false, muted: { active: true, until } },
    ]);
    fixture.detectChanges();
    expect(card('other-boards').textContent).toContain('Old Board');

    confirm.ask.and.resolveTo(true);
    boardService.resetNotificationSettings.and.returnValue(of(NOTIFICATION));
    await component().resetOtherBoard(context.otherBoards()[0]);
    expect(boardService.resetNotificationSettings).toHaveBeenCalledWith(40);

    boardService.unmute.and.returnValue(of(OFF));
    component().unmuteOtherBoard(context.otherBoards()[1]);
    expect(boardService.unmute).toHaveBeenCalledWith(41);
    expect(context.load).toHaveBeenCalled();
  });

  it('hides the other-boards card on a board scope', () => {
    context.otherBoards.set([{ boardId: 40, name: 'Old Board', projectKey: 'OLD', hasCustomSettings: true, muted: OFF }]);
    context.selectedId.set(28);
    fixture.detectChanges();
    expect(card('other-boards')).toBeNull();
  });

  it('offers the board workflow statuses from Jira and falls back to the static list (AC5.5)', () => {
    const http = TestBed.inject(HttpTestingController);
    context.boards.set([OMS, { ...OMS, boardId: 26, name: 'ESB Core Board', projectKey: 'CORE', reviewTracked: true }]);
    context.selectedId.set(26);
    fixture.detectChanges();
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/boards/26/statuses')).flush({ statuses: ['To Do', 'Doing', 'Done'] });
    fixture.detectChanges();
    expect(component().statusOptions()).toEqual(['To Do', 'Doing', 'Done']);

    context.selectedId.set(28);
    fixture.detectChanges();
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/boards/28/statuses')).flush({ error: 'jira_unavailable' }, { status: 502, statusText: 'Bad Gateway' });
    fixture.detectChanges();
    expect(component().statusOptions()).toContain('Testing & Review');
  });

  it('cancels a pending board statuses request on destroy', () => {
    const http = TestBed.inject(HttpTestingController);
    context.boards.set([OMS]);
    context.selectedId.set(28);
    fixture.detectChanges();
    fixture.detectChanges();
    const pending = http.expectOne((r) => r.url.endsWith('/api/dashboard/boards/28/statuses'));
    fixture.destroy();
    expect(pending.cancelled).toBeTrue();
  });
});

describe('SettingsComponent in admin mode (#6)', () => {
  it("shows the target's master switch read-only and leaves the admin's own switch alone", async () => {
    const boardService = jasmine.createSpyObj('BoardService', ['getSettings']);
    boardService.getSettings.and.returnValue(of(sample({ notifications: { enabled: false, chosen: true } })));
    await TestBed.configureTestingModule({
      imports: [SettingsComponent],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: BoardService, useValue: boardService },
        { provide: BoardContextService, useValue: new FakeBoardContext() },
        { provide: SETTINGS_MODE, useValue: 'admin' },
      ],
    }).compileComponents();
    const fixture = TestBed.createComponent(SettingsComponent);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;

    const card = el.querySelector('[data-card="target-switch"]') as HTMLElement;
    expect(card.textContent).toContain('Notifikasi mati');
    expect(card.textContent).toContain('Dipilih sendiri oleh user');
    expect(card.querySelector('input')).toBeNull();
    expect(el.querySelector('[data-card="master-switch"]')).toBeNull();
    expect(TestBed.inject(NotificationSwitchService).state()).toBeNull();
  });
});
