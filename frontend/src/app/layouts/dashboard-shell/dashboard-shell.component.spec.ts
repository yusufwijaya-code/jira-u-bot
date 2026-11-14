import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { BehaviorSubject } from 'rxjs';
import { DashboardShellComponent } from './dashboard-shell.component';
import { AuthService } from '../../core/services/auth.service';
import { BoardService } from '../../core/services/board.service';
import { MuteStateService } from '../../core/services/mute-state.service';
import { BoardContextService, BoardSummary } from '../../core/services/board-context.service';
import { signal } from '@angular/core';
import { of } from 'rxjs';

describe('DashboardShellComponent', () => {
  let fixture: ComponentFixture<DashboardShellComponent>;
  let user$: BehaviorSubject<any>;

  const el = () => fixture.nativeElement as HTMLElement;
  const trigger = () => el().querySelector('.user-menu-trigger') as HTMLButtonElement;
  const menu = () => el().querySelector('.user-menu-list');

  const OMS: BoardSummary = {
    boardId: 28,
    name: 'ESB OMS Board',
    projectKey: 'OMS',
    activeSprint: { id: 8294, name: 'OMS Sprint 23 - 2026', startDate: '2026-11-09T01:00:00.000Z', endDate: '2026-11-22T10:00:00.000Z' },
    ticketCount: 17,
    hasCustomSettings: true,
    muted: { active: false, until: null },
  };
  let boardContext: any;
  const boardTrigger = () => el().querySelector('.board-picker-trigger') as HTMLButtonElement;
  const boardMenu = () => el().querySelector('.board-menu-list');

  beforeEach(async () => {
    boardContext = {
      boards: signal<BoardSummary[]>([OMS]),
      loaded: signal(true),
      selectedId: signal<number | null>(null),
      selected: () => boardContext.boards().find((b: BoardSummary) => b.boardId === boardContext.selectedId()) ?? null,
      avatarUrl: (id: number) => '/avatar/' + id,
      load: jasmine.createSpy('load'),
      select: jasmine.createSpy('select').and.callFake((id: number | null) => boardContext.selectedId.set(id)),
    };
    user$ = new BehaviorSubject<any>({ email: 'dev@esb.co.id', isAdmin: false });
    await TestBed.configureTestingModule({
      imports: [DashboardShellComponent],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        provideRouter([]),
        { provide: AuthService, useValue: { currentUser$: user$ } },
        { provide: BoardService, useValue: { getSettings: () => of({ mute: { global: { active: false, until: null }, board: null } }) } },
        { provide: BoardContextService, useValue: boardContext },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(DashboardShellComponent);
    fixture.detectChanges();
  });

  it('shows only the user email in the navbar, with the menu closed', () => {
    expect(trigger().textContent).toContain('dev@esb.co.id');
    expect(menu()).toBeNull();
  });

  it('opens the dropdown with a Logout item when the email is clicked', () => {
    trigger().click();
    fixture.detectChanges();

    expect(menu()).not.toBeNull();
    expect(menu()!.textContent).toContain('Logout');
  });

  it('closes the dropdown on outside click and on Escape', () => {
    trigger().click();
    fixture.detectChanges();
    document.body.click();
    fixture.detectChanges();
    expect(menu()).toBeNull();

    trigger().click();
    fixture.detectChanges();
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    fixture.detectChanges();
    expect(menu()).toBeNull();
  });

  it('shows the Admin item only for admins', () => {
    trigger().click();
    fixture.detectChanges();
    expect(menu()!.textContent).not.toContain('Admin');

    user$.next({ email: 'yusuf.wijaya@esb.co.id', isAdmin: true });
    fixture.detectChanges();
    expect(menu()!.textContent).toContain('Admin');
  });

  it('stays open when clicking inside the menu itself (e.g. the header)', () => {
    trigger().click();
    fixture.detectChanges();

    (el().querySelector('.user-menu-header') as HTMLElement).click();
    fixture.detectChanges();

    expect(menu()).not.toBeNull();
  });

  it('closes when clicking any element elsewhere on the page, including inputs', () => {
    const input = document.createElement('input');
    document.body.appendChild(input);
    trigger().click();
    fixture.detectChanges();

    input.click();
    fixture.detectChanges();

    expect(menu()).toBeNull();
    input.remove();
  });

  it('closes when keyboard focus moves outside the menu', () => {
    const outside = document.createElement('button');
    document.body.appendChild(outside);
    trigger().click();
    fixture.detectChanges();

    outside.focus();
    fixture.detectChanges();

    expect(menu()).toBeNull();
    outside.remove();
  });

  it('toggles closed when the trigger is clicked a second time', () => {
    trigger().click();
    fixture.detectChanges();
    trigger().click();
    fixture.detectChanges();

    expect(menu()).toBeNull();
  });

  it('shows a mute badge in the top bar only while a mute is active', () => {
    expect(el().querySelector('.mute-badge')).toBeNull();

    const muteState = TestBed.inject(MuteStateService);
    muteState.set({ active: true, until: new Date(Date.now() + 60 * 60 * 1000).toISOString() });
    fixture.detectChanges();
    expect(el().querySelector('.mute-badge')?.textContent).toContain('Dimute');

    muteState.set({ active: false, until: null });
    fixture.detectChanges();
    expect(el().querySelector('.mute-badge')).toBeNull();
  });

  it('shows "Notif dimatikan" in the badge for a forever mute', () => {
    TestBed.inject(MuteStateService).set({ active: true, until: '9999-12-31T23:59:59+07:00', forever: true });
    fixture.detectChanges();
    const badge = el().querySelector('.mute-badge.forever');
    expect(badge?.textContent).toContain('Notif dimatikan');
  });

  it('loads the boards and shows "Semua board" by default', () => {
    expect(boardContext.load).toHaveBeenCalled();
    expect(boardTrigger().textContent).toContain('Semua board');
    expect(boardTrigger().textContent).toContain('1 board aktif');
  });

  it('opens the board menu with every board, its sprint, ticket count and custom dot', () => {
    boardTrigger().click();
    fixture.detectChanges();

    const options = Array.from(el().querySelectorAll('.board-option'));
    expect(options.length).toBe(2);
    expect(options[1].textContent).toContain('ESB OMS Board');
    expect(options[1].textContent).toContain('OMS Sprint 23 - 2026');
    expect(options[1].querySelector('.count-pill')?.textContent).toContain('17');
    expect(options[1].querySelector('.custom-dot')).not.toBeNull();
  });

  it('selects a board, closes the menu and shows it on the trigger', () => {
    boardTrigger().click();
    fixture.detectChanges();
    (el().querySelectorAll('.board-option')[1] as HTMLButtonElement).click();
    fixture.detectChanges();

    expect(boardContext.select).toHaveBeenCalledWith(28);
    expect(boardMenu()).toBeNull();
    expect(boardTrigger().textContent).toContain('ESB OMS Board');
    expect(boardTrigger().querySelector('img')?.getAttribute('src')).toBe('/avatar/28');
  });

  it('closes the board menu on an outside click and only one menu is open at a time', () => {
    boardTrigger().click();
    fixture.detectChanges();
    trigger().click();
    fixture.detectChanges();
    expect(boardMenu()).toBeNull();
    expect(menu()).not.toBeNull();

    boardTrigger().click();
    fixture.detectChanges();
    document.body.click();
    fixture.detectChanges();
    expect(boardMenu()).toBeNull();
  });
});
