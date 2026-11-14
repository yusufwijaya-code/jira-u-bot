import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { AdminComponent } from './admin.component';
import { AdminService, ListUsersResponse } from '../../../core/services/admin.service';
import { ConfirmService } from '../../../core/ui/confirm.service';

describe('AdminComponent', () => {
  let fixture: ComponentFixture<AdminComponent>;
  let adminService: jasmine.SpyObj<AdminService>;
  let confirmService: jasmine.SpyObj<ConfirmService>;

  beforeEach(async () => {
    adminService = jasmine.createSpyObj('AdminService', ['listUsers', 'toggleActive', 'editJiraAccountId', 'deleteCredential', 'deleteUser']);
    confirmService = jasmine.createSpyObj('ConfirmService', ['ask']);

    await TestBed.configureTestingModule({
      imports: [AdminComponent],
      providers: [
        { provide: AdminService, useValue: adminService },
        { provide: ConfirmService, useValue: confirmService },
      ],
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

  it('toggles active status and reloads the list on success', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    adminService.toggleActive.and.returnValue(of({}));

    fixture.nativeElement.querySelector('.actions button').click();

    expect(adminService.toggleActive).toHaveBeenCalledWith(1, false);
    expect(adminService.listUsers).toHaveBeenCalledTimes(2);
  });

  it('shows a row-scoped error message when toggling active status fails', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    adminService.toggleActive.and.returnValue(throwError(() => ({ error: { error: 'Tidak bisa menonaktifkan diri sendiri' } })));

    fixture.nativeElement.querySelector('.actions button').click();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.actions .msg.error').textContent).toContain('Tidak bisa menonaktifkan diri sendiri');
  });

  it('saves the Jira Account ID optimistically, without reloading the list', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: 'old-id', hasCredential: false, credentialMasked: '' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    adminService.editJiraAccountId.and.returnValue(of({}));

    fixture.componentInstance['jiraAccountIdDrafts'][1] = 'new-id';
    fixture.componentInstance['onSaveJiraAccountId'](user);

    expect(adminService.editJiraAccountId).toHaveBeenCalledWith(1, 'new-id');
    expect(adminService.listUsers).toHaveBeenCalledTimes(1);
    expect(user.jiraAccountId).toBe('new-id');
  });

  it('only shows the Hapus Credential button when the user has a credential', () => {
    const withCred = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: true, credentialMasked: '••••ab3f' };
    const withoutCred = { userId: 2, email: 'b@esb.co.id', userName: 'B', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '' };
    adminService.listUsers.and.returnValue(of({ users: [withCred, withoutCred] }));
    fixture.detectChanges();

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    expect(rows[0].textContent).toContain('Hapus Credential');
    expect(rows[1].textContent).not.toContain('Hapus Credential');
  });

  it('asks for confirmation before deleting a credential, and does nothing if cancelled', async () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: true, credentialMasked: '••••ab3f' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    confirmService.ask.and.resolveTo(false);

    await fixture.componentInstance['onDeleteCredential'](user);

    expect(adminService.deleteCredential).not.toHaveBeenCalled();
  });

  it('deletes the credential and reloads the list on confirmation', async () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: true, credentialMasked: '••••ab3f' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    confirmService.ask.and.resolveTo(true);
    adminService.deleteCredential.and.returnValue(of({}));

    await fixture.componentInstance['onDeleteCredential'](user);

    expect(adminService.deleteCredential).toHaveBeenCalledWith(1);
    expect(adminService.listUsers).toHaveBeenCalledTimes(2);
  });

  it('shows the self-lockout error message when deleting a user fails', async () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    confirmService.ask.and.resolveTo(true);
    adminService.deleteUser.and.returnValue(throwError(() => ({ error: { error: 'Tidak bisa menghapus diri sendiri' } })));

    await fixture.componentInstance['onDeleteUser'](user);
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.actions .msg.error').textContent).toContain('Tidak bisa menghapus diri sendiri');
  });

  it('disables every action button for a row while an action is in flight', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    adminService.toggleActive.and.returnValue(of({}));

    fixture.componentInstance['rowActionInFlight'][1] = true;
    fixture.detectChanges();

    const buttons: HTMLButtonElement[] = Array.from(fixture.nativeElement.querySelectorAll('tbody tr button'));
    expect(buttons.every((btn) => btn.disabled)).toBe(true);
  });

  it('filters the table by the search query', () => {
    const a = { userId: 1, email: 'alice@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '' };
    const b = { userId: 2, email: 'bob@esb.co.id', userName: 'B', flagActive: false, createdDate: '', jiraAccountId: '', hasCredential: true, credentialMasked: '••••ab3f' };
    adminService.listUsers.and.returnValue(of({ users: [a, b] }));
    fixture.detectChanges();

    fixture.componentInstance['query'] = 'bob';
    fixture.detectChanges();

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    expect(rows.length).toBe(1);
    expect(rows[0].textContent).toContain('bob@esb.co.id');
  });

  it('shows summary stats for total, active and credential counts', () => {
    const a = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '' };
    const b = { userId: 2, email: 'b@esb.co.id', userName: 'B', flagActive: false, createdDate: '', jiraAccountId: '', hasCredential: true, credentialMasked: '••••ab3f' };
    adminService.listUsers.and.returnValue(of({ users: [a, b] }));
    fixture.detectChanges();

    const values = Array.from(fixture.nativeElement.querySelectorAll('.stat-value')).map((el: any) => el.textContent.trim());
    expect(values).toEqual(['2', '1', '1']);
  });
});
