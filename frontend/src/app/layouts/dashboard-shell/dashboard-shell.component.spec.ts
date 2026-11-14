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
});
