import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { BehaviorSubject } from 'rxjs';
import { AdminShellComponent } from './admin-shell.component';
import { AuthService } from '../../core/services/auth.service';

describe('AdminShellComponent', () => {
  let fixture: ComponentFixture<AdminShellComponent>;
  const el = () => fixture.nativeElement as HTMLElement;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [AdminShellComponent],
      providers: [
        provideRouter([]),
        { provide: AuthService, useValue: { currentUser$: new BehaviorSubject({ email: 'admin@esb.co.id', isAdmin: true }) } },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(AdminShellComponent);
    fixture.detectChanges();
  });

  it('renders the three admin menu items', () => {
    const links = Array.from(el().querySelectorAll('.sidebar-nav .nav-item')).map((a) => a.textContent!.trim());
    expect(links).toEqual(['Dashboard', 'User', 'Broadcast Message', 'Riwayat Pesan']);
    const hrefs = Array.from(el().querySelectorAll('.sidebar-nav .nav-item')).map((a) => a.getAttribute('href'));
    expect(hrefs).toEqual(['/admin', '/admin/users', '/admin/broadcast', '/admin/chat-logs']);
  });

  it('has a Dashboard Utama button to the main dashboard', () => {
    const btn = el().querySelector('a.btn-home') as HTMLAnchorElement;
    expect(btn.textContent).toContain('Dashboard Utama');
    expect(btn.getAttribute('href')).toBe('/dashboard');
  });

  it('has no board picker', () => {
    expect(el().querySelector('.board-picker')).toBeNull();
  });

  it('shows the Admin badge under the logo', () => {
    expect(el().querySelector('.sidebar-brand .admin-badge')?.textContent).toContain('Admin');
  });

  it('opens the user menu with the email, Administrator badge and Logout', () => {
    (el().querySelector('.user-menu-trigger') as HTMLButtonElement).click();
    fixture.detectChanges();
    const menu = el().querySelector('.user-menu-list')!;
    expect(menu.textContent).toContain('admin@esb.co.id');
    expect(menu.textContent).toContain('Administrator');
    expect(menu.textContent).toContain('Logout');
  });
});
