import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { AdminOverviewComponent } from './admin-overview.component';
import { AdminService, UserRow } from '../../../core/services/admin.service';

const user = (userId: number, flagActive: boolean, hasCredential: boolean): UserRow => ({
  userId,
  email: `u${userId}@esb.co.id`,
  userName: '',
  flagActive,
  createdDate: '',
  jiraAccountId: '',
  hasCredential,
  credentialMasked: hasCredential ? '••••abcd' : '',
  chatAppActivated: userId === 1,
  chatAppActivatedDate: null,
  chatSender: null,
  effectiveChatSender: 'operator',
});

describe('AdminOverviewComponent', () => {
  let fixture: ComponentFixture<AdminOverviewComponent>;
  let adminService: jasmine.SpyObj<AdminService>;
  const el = () => fixture.nativeElement as HTMLElement;

  beforeEach(async () => {
    adminService = jasmine.createSpyObj('AdminService', ['listUsers']);
    await TestBed.configureTestingModule({
      imports: [AdminOverviewComponent],
      providers: [provideRouter([]), { provide: AdminService, useValue: adminService }],
    }).compileComponents();
    fixture = TestBed.createComponent(AdminOverviewComponent);
  });

  it('shows the stats and the two shortcut lists', () => {
    adminService.listUsers.and.returnValue(of({ users: [user(1, true, true), user(2, true, false), user(3, false, false), user(4, true, false)] }));
    fixture.detectChanges();

    const stats = Array.from(el().querySelectorAll('.stat-value')).map((s) => s.textContent!.trim());
    expect(stats).toEqual(['4', '3', '1', '2']);
    expect(el().querySelector('[data-stat="chat-app-inactive"] .stat-value')!.textContent!.trim()).toBe('2');

    const noCred = Array.from(el().querySelectorAll('[data-list="credential"] li a')).map((a) => a.getAttribute('href'));
    expect(noCred).toEqual(['/admin/users/2', '/admin/users/3', '/admin/users/4']);
    const inactive = Array.from(el().querySelectorAll('[data-list="inactive"] li a')).map((a) => a.getAttribute('href'));
    expect(inactive).toEqual(['/admin/users/3']);
  });

  it('caps each shortcut list at 5 users', () => {
    adminService.listUsers.and.returnValue(of({ users: [1, 2, 3, 4, 5, 6, 7].map((id) => user(id, true, false)) }));
    fixture.detectChanges();

    expect(el().querySelectorAll('[data-list="credential"] li').length).toBe(5);
  });

  it('shows an empty message when nobody needs attention', () => {
    adminService.listUsers.and.returnValue(of({ users: [user(1, true, true)] }));
    fixture.detectChanges();

    expect(el().querySelector('[data-list="credential"]')!.textContent).toContain('Semua user sudah punya credential.');
    expect(el().querySelector('[data-list="inactive"]')!.textContent).toContain('Semua user sudah aktif.');
  });

  it('shows the error state when loading fails', () => {
    adminService.listUsers.and.returnValue(throwError(() => ({ status: 500 })));
    fixture.detectChanges();

    expect(el().querySelector('.msg.error')!.textContent).toContain('Gagal memuat data user');
  });
});
