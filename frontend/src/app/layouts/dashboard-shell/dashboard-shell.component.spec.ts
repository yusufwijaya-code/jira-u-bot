import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { BehaviorSubject } from 'rxjs';
import { DashboardShellComponent } from './dashboard-shell.component';
import { AuthService } from '../../core/services/auth.service';

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
      providers: [provideRouter([]), { provide: AuthService, useValue: { currentUser$: user$ } }],
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
});
