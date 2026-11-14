import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { BehaviorSubject } from 'rxjs';
import { DashboardShellComponent } from './dashboard-shell.component';
import { AuthService } from '../../core/services/auth.service';
import { BoardService } from '../../core/services/board.service';
import { MuteStateService } from '../../core/services/mute-state.service';
import { of } from 'rxjs';

describe('DashboardShellComponent', () => {
  let fixture: ComponentFixture<DashboardShellComponent>;
  let user$: BehaviorSubject<any>;

  const el = () => fixture.nativeElement as HTMLElement;
  const trigger = () => el().querySelector('.user-menu-trigger') as HTMLButtonElement;
  const menu = () => el().querySelector('.user-menu-list');

  beforeEach(async () => {
    user$ = new BehaviorSubject<any>({ email: 'dev@esb.co.id', isAdmin: false });
    await TestBed.configureTestingModule({
      imports: [DashboardShellComponent],
      providers: [
        provideRouter([]),
        { provide: AuthService, useValue: { currentUser$: user$ } },
        { provide: BoardService, useValue: { getSettings: () => of({ mute: { active: false, until: null } }) } },
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
});
