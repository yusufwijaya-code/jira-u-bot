import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { AdminComponent } from './admin.component';
import { AdminService, ListUsersResponse } from '../../../core/services/admin.service';

describe('AdminComponent', () => {
  let fixture: ComponentFixture<AdminComponent>;
  let adminService: jasmine.SpyObj<AdminService>;

  beforeEach(async () => {
    adminService = jasmine.createSpyObj('AdminService', ['listUsers']);

    await TestBed.configureTestingModule({
      imports: [AdminComponent],
      providers: [{ provide: AdminService, useValue: adminService }],
    }).compileComponents();

    fixture = TestBed.createComponent(AdminComponent);
  });

  it('renders one row per user with status badge and Jira Account ID', () => {
    const response: ListUsersResponse = {
      users: [
        { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '2026-01-01', jiraAccountId: 'acc-1', hasCredential: true, credentialMasked: '••••ab3f' },
        { userId: 2, email: 'b@esb.co.id', userName: 'B', flagActive: false, createdDate: '2026-01-02', jiraAccountId: '', hasCredential: false, credentialMasked: '' },
      ],
    };
    adminService.listUsers.and.returnValue(of(response));
    fixture.detectChanges();

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    expect(rows.length).toBe(2);
    expect(rows[0].textContent).toContain('a@esb.co.id');
    expect(rows[0].querySelector('.status-badge').textContent).toContain('Aktif');
    expect(rows[1].querySelector('.status-badge').textContent).toContain('Nonaktif');
    expect(rows[1].textContent).toContain('—');
  });

  it('shows an empty table with no rows when there are no users', () => {
    adminService.listUsers.and.returnValue(of({ users: [] }));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelectorAll('tbody tr').length).toBe(0);
  });

  it('shows the access-denied message on a 403 response', () => {
    adminService.listUsers.and.returnValue(throwError(() => ({ status: 403 })));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Kamu tidak punya akses ke halaman ini');
  });

  it('shows a generic error message on a 500 response', () => {
    adminService.listUsers.and.returnValue(throwError(() => ({ status: 500 })));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal memuat daftar user, coba lagi');
  });
});
