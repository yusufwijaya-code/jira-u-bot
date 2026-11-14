import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { of, throwError } from 'rxjs';
import { SettingsComponent } from './settings.component';
import { BoardService, NotificationSettings, SettingsResponse } from '../../../core/services/board.service';
import { BoardContextService, BoardSummary } from '../../../core/services/board-context.service';
import { MuteStateService } from '../../../core/services/mute-state.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';

const NOTIFICATION: NotificationSettings = {
  reminderTiers: ['2h', '45m', '1m'],
  overageTiers: ['15m'],
  overageRepeatMinutes: 30,
  notifyEarlyFinish: true,
  notifyReturnedToTodo: false,
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
        { provide: BoardService, useValue: boardService },
        { provide: BoardContextService, useValue: context },
        { provide: ToastService, useValue: toast },
        { provide: ConfirmService, useValue: confirm },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(SettingsComponent);
    fixture.detectChanges();
  });

  it('renders the credential card, the scope bar, then the four notification cards', () => {
    const order = Array.from(el().querySelectorAll('section.card[data-card]')).map((s) => s.getAttribute('data-card'));
    expect(order).toEqual(['credential', 'reminder', 'overage', 'events', 'quiet']);
    expect(el().querySelector('.scope-bar')?.getAttribute('data-scope')).toBe('default');
    expect(el().querySelector('.scope-bar')?.textContent).toContain('Pengaturan default semua board');
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
    component().patch({ notifyReturnedToTodo: true });

    component().save('events');

    expect(boardService.saveNotificationSettings).toHaveBeenCalledWith({ ...NOTIFICATION, notifyReturnedToTodo: true }, null);
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
});
