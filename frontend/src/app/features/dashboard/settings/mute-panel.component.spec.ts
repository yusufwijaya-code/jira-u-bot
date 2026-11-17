import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { MutePanelComponent } from './mute-panel.component';
import { BoardService } from '../../../core/services/board.service';
import { MuteStateService } from '../../../core/services/mute-state.service';
import { ToastService } from '../../../core/ui/toast.service';
import { BoardContextService } from '../../../core/services/board-context.service';
import { SETTINGS_MODE, SettingsApi, SettingsScope } from '../../../core/services/settings-api';

describe('MutePanelComponent', () => {
  let fixture: ComponentFixture<MutePanelComponent>;
  let boardService: jasmine.SpyObj<BoardService>;
  let toast: jasmine.SpyObj<ToastService>;
  let muteState: MuteStateService;
  let boardContext: jasmine.SpyObj<BoardContextService>;
  const el = () => fixture.nativeElement as HTMLElement;
  const inOneHour = () => new Date(Date.now() + 3600_000).toISOString();

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', ['mute', 'unmute']);
    toast = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);
    boardContext = jasmine.createSpyObj('BoardContextService', ['load']);

    await TestBed.configureTestingModule({
      imports: [MutePanelComponent],
      providers: [
        { provide: BoardService, useValue: boardService },
        { provide: ToastService, useValue: toast },
        { provide: BoardContextService, useValue: boardContext },
      ],
    }).compileComponents();

    muteState = TestBed.inject(MuteStateService);
    muteState.set({ active: false, until: null });
    fixture = TestBed.createComponent(MutePanelComponent);
    fixture.detectChanges();
  });

  it('offers 1 jam / 2 jam / Sampai besok (no Selamanya) when not muted', () => {
    const labels = Array.from(el().querySelectorAll('.mute-actions button')).map((b) => b.textContent!.trim());
    expect(labels).toEqual(['1 jam', '2 jam', 'Sampai besok']);
    expect(el().querySelector('.mute-banner')).toBeNull();
  });

  it('mutes instantly and switches to the banner', () => {
    boardService.mute.and.returnValue(of({ active: true, until: inOneHour() }));

    (el().querySelectorAll('.mute-actions button')[0] as HTMLButtonElement).click();
    fixture.detectChanges();

    expect(boardService.mute).toHaveBeenCalledWith({ minutes: 60 }, null);
    expect(muteState.active()).toBeTrue();
    expect(el().querySelector('.mute-banner')?.textContent).toContain('Notifikasi sedang dimute');
    expect(toast.success).toHaveBeenCalled();
  });

  it('mutes until tomorrow', () => {
    boardService.mute.and.returnValue(of({ active: true, until: inOneHour() }));
    (el().querySelectorAll('.mute-actions button')[2] as HTMLButtonElement).click();
    expect(boardService.mute).toHaveBeenCalledWith({ until: 'tomorrow' }, null);
  });

  it('cancels an active mute', () => {
    muteState.set({ active: true, until: inOneHour() });
    fixture.detectChanges();
    boardService.unmute.and.returnValue(of({ active: false, until: null }));

    (el().querySelector('.mute-banner button') as HTMLButtonElement).click();
    fixture.detectChanges();

    expect(boardService.unmute).toHaveBeenCalled();
    expect(muteState.active()).toBeFalse();
    expect(el().querySelector('.mute-actions')).not.toBeNull();
  });

  it('shows an error toast and keeps the state when muting fails', () => {
    boardService.mute.and.returnValue(throwError(() => ({ status: 500 })));
    (el().querySelectorAll('.mute-actions button')[0] as HTMLButtonElement).click();

    expect(toast.error).toHaveBeenCalled();
    expect(muteState.active()).toBeFalse();
  });

  it('treats an already-expired mute as inactive', () => {
    muteState.set({ active: true, until: new Date(Date.now() - 1000).toISOString() });
    expect(muteState.active()).toBeFalse();
  });

  it('shows an existing forever mute and switches it back on', () => {
    muteState.set({ active: true, until: '9999-12-31T23:59:59+07:00', forever: true });
    fixture.detectChanges();
    const banner = el().querySelector('.mute-banner.forever');
    expect(banner?.textContent).toContain('Notifikasi dimatikan');
    expect(banner?.querySelector('button')?.textContent).toContain('Aktifkan lagi');

    boardService.unmute.and.returnValue(of({ active: false, until: null }));
    (banner!.querySelector('button') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(boardService.unmute).toHaveBeenCalledWith(null);
    expect(el().querySelector('.mute-actions')).not.toBeNull();
  });

  describe('board scope', () => {
    beforeEach(() => {
      fixture.componentRef.setInput('boardId', 28);
      fixture.componentRef.setInput('boardStatus', { active: false, until: null });
      fixture.detectChanges();
    });

    it('mutes only the board, leaves the global mute alone and refreshes the picker', () => {
      boardService.mute.and.returnValue(of({ active: true, until: inOneHour() }));
      const emitted = jasmine.createSpy('statusChange');
      fixture.componentInstance.statusChange.subscribe(emitted);

      (el().querySelectorAll('.mute-actions button')[0] as HTMLButtonElement).click();
      fixture.detectChanges();

      expect(boardService.mute).toHaveBeenCalledWith({ minutes: 60 }, 28);
      expect(muteState.active()).toBeFalse();
      expect(boardContext.load).toHaveBeenCalled();
      expect(emitted).toHaveBeenCalled();
      expect(el().querySelector('.mute-banner')?.textContent).toContain('Board ini sedang dimute');
    });

    it('shows the board mute from the input and unmutes the board', () => {
      fixture.componentRef.setInput('boardStatus', { active: true, until: inOneHour() });
      fixture.detectChanges();
      boardService.unmute.and.returnValue(of({ active: false, until: null }));

      (el().querySelector('.mute-banner button') as HTMLButtonElement).click();
      fixture.detectChanges();

      expect(boardService.unmute).toHaveBeenCalledWith(28);
      expect(el().querySelector('.mute-actions')).not.toBeNull();
    });
  });
});

describe('MutePanelComponent (admin mode)', () => {
  let fixture: ComponentFixture<MutePanelComponent>;
  let api: jasmine.SpyObj<SettingsApi>;
  let muteState: MuteStateService;
  let setSpy: jasmine.Spy;
  const el = () => fixture.nativeElement as HTMLElement;
  const inOneHour = () => new Date(Date.now() + 3600_000).toISOString();

  beforeEach(async () => {
    api = jasmine.createSpyObj('SettingsApi', ['mute', 'unmute']);
    await TestBed.configureTestingModule({
      imports: [MutePanelComponent],
      providers: [
        { provide: SettingsApi, useValue: api },
        { provide: SettingsScope, useValue: jasmine.createSpyObj('SettingsScope', ['load']) },
        { provide: SETTINGS_MODE, useValue: 'admin' },
        { provide: ToastService, useValue: jasmine.createSpyObj('ToastService', ['success', 'error', 'info']) },
      ],
    }).compileComponents();

    muteState = TestBed.inject(MuteStateService);
    muteState.set({ active: false, until: null });
    setSpy = spyOn(muteState, 'set').and.callThrough();
    fixture = TestBed.createComponent(MutePanelComponent);
  });

  it('shows the target user global mute from globalStatus, not the admin own state', () => {
    fixture.componentRef.setInput('globalStatus', { active: true, until: inOneHour() });
    fixture.detectChanges();

    expect(el().querySelector('.mute-banner')).not.toBeNull();
  });

  it('mutes the target without touching the admin top bar mute state', () => {
    fixture.detectChanges();
    api.mute.and.returnValue(of({ active: true, until: inOneHour() }));

    (el().querySelectorAll('.mute-actions button')[0] as HTMLButtonElement).click();
    fixture.detectChanges();

    expect(api.mute).toHaveBeenCalledWith({ minutes: 60 }, null);
    expect(setSpy).not.toHaveBeenCalled();
    expect(el().querySelector('.mute-banner')).not.toBeNull();
  });
});
