import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { SettingsComponent } from './settings.component';
import { BoardService, NotificationSettings, SettingsResponse } from '../../../core/services/board.service';
import { MuteStateService } from '../../../core/services/mute-state.service';
import { ToastService } from '../../../core/ui/toast.service';

const NOTIFICATION: NotificationSettings = {
  reminderTiers: ['2h', '45m', '1m'],
  overageTiers: ['15m'],
  overageRepeatMinutes: 30,
  notifyEarlyFinish: true,
  notifyReturnedToTodo: false,
  quietHours: null,
};

function sample(overrides: Partial<SettingsResponse> = {}): SettingsResponse {
  return {
    notification: { ...NOTIFICATION },
    mute: { active: false, until: null },
    credential: { hasCredential: false, maskedPreview: '' },
    ...overrides,
  };
}

describe('SettingsComponent', () => {
  let fixture: ComponentFixture<SettingsComponent>;
  let boardService: jasmine.SpyObj<BoardService>;
  let toast: jasmine.SpyObj<ToastService>;
  const el = () => fixture.nativeElement as HTMLElement;
  const card = (name: string) => el().querySelector(`[data-card="${name}"]`) as HTMLElement;
  const component = () => fixture.componentInstance as any;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', [
      'getSettings', 'saveNotificationSettings', 'mute', 'unmute', 'saveCredential', 'deleteCredential',
    ]);
    boardService.getSettings.and.returnValue(of(sample()));
    toast = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);

    await TestBed.configureTestingModule({
      imports: [SettingsComponent],
      providers: [
        { provide: BoardService, useValue: boardService },
        { provide: ToastService, useValue: toast },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(SettingsComponent);
    fixture.detectChanges();
  });

  it('renders the Jira API key card first, then the four notification cards', () => {
    const order = Array.from(el().querySelectorAll('section.card[data-card]')).map((s) => s.getAttribute('data-card'));
    expect(order).toEqual(['credential', 'reminder', 'overage', 'events', 'quiet']);
    expect(card('credential').querySelector('app-jira-credential')).not.toBeNull();
  });

  it('checks the saved pre-budget tiers and shows a custom tier tile', () => {
    const tiles = Array.from(card('reminder').querySelectorAll('.tier-tile'));
    const checked = tiles
      .filter((t) => (t.querySelector('input') as HTMLInputElement).checked)
      .map((t) => t.querySelector('strong')!.textContent!.trim());
    expect(checked).toEqual(['T-2 jam', 'T-45 menit', 'T-1 menit']);
    expect(card('reminder').querySelector('.tier-tile.custom strong')?.textContent).toContain('T-45 menit');
  });

  it('shows the overage flow preview from tiers and repeat', () => {
    expect(card('overage').querySelector('.timeline-text')?.textContent).toContain('Budget habis → +15 menit → tiap 30 menit');
  });

  it('reflects the event toggles', () => {
    const switches = Array.from(card('events').querySelectorAll('input.switch')) as HTMLInputElement[];
    expect(switches.map((s) => s.checked)).toEqual([true, false]);
  });

  it('saves the whole draft and shows a success toast', () => {
    boardService.saveNotificationSettings.and.callFake((s) => of(s));
    component().patch({ notifyReturnedToTodo: true });

    component().save('events');

    expect(boardService.saveNotificationSettings).toHaveBeenCalledWith({ ...NOTIFICATION, notifyReturnedToTodo: true });
    expect(toast.success).toHaveBeenCalled();
  });

  it('shows a server field error on the card that owns the field', () => {
    boardService.saveNotificationSettings.and.returnValue(
      throwError(() => ({ status: 400, error: { error: 'x', fields: { overageRepeatMinutes: 'interval tidak valid' } } })),
    );

    component().save('overage');
    fixture.detectChanges();

    expect(card('overage').querySelector('.card-footer .msg.error')?.textContent).toContain('interval tidak valid');
    expect(card('reminder').querySelector('.card-footer .msg.error')?.textContent?.trim()).toBe('');
    expect(toast.error).not.toHaveBeenCalled();
  });

  it('turns quiet hours on with the default window and off again', () => {
    component().setQuietEnabled(true);
    fixture.detectChanges();
    expect(component().draft().quietHours).toEqual({ start: '12:00', end: '13:00' });
    expect(card('quiet').querySelectorAll('input[type=time]').length).toBe(2);

    component().setQuietEnabled(false);
    fixture.detectChanges();
    expect(component().draft().quietHours).toBeNull();
    expect(card('quiet').querySelectorAll('input[type=time]').length).toBe(0);
  });

  it('publishes the mute status to the shared state', () => {
    const until = new Date(Date.now() + 3600_000).toISOString();
    boardService.getSettings.and.returnValue(of(sample({ mute: { active: true, until } })));
    component().loadSettings();

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
