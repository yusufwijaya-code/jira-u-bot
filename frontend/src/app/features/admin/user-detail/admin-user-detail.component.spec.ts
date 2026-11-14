import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { Component } from '@angular/core';
import { provideHttpClient } from '@angular/common/http';
import { of, throwError } from 'rxjs';
import { AdminUserDetailComponent } from './admin-user-detail.component';
import { AdminService, UserRow } from '../../../core/services/admin.service';
import { AdminUserSettingsApi } from '../../../core/services/admin-user-settings';
import { SETTINGS_MODE, SettingsApi } from '../../../core/services/settings-api';
import { SettingsComponent } from '../../dashboard/settings/settings.component';

@Component({ selector: 'app-settings', standalone: true, template: '<p class="settings-stub">settings</p>' })
class SettingsStubComponent {}

const USER: UserRow = {
  userId: 12,
  email: 'fernando.marco@esb.co.id',
  userName: 'Marco',
  flagActive: true,
  createdDate: '',
  jiraAccountId: '712020:abc',
  hasCredential: true,
  credentialMasked: '••••50CF',
};

describe('AdminUserDetailComponent', () => {
  let fixture: ComponentFixture<AdminUserDetailComponent>;
  let adminService: jasmine.SpyObj<AdminService>;
  const el = () => fixture.nativeElement as HTMLElement;

  beforeEach(async () => {
    adminService = jasmine.createSpyObj('AdminService', ['getUser', 'listUserBoards', 'toggleActive', 'editJiraAccountId']);
    adminService.listUserBoards.and.returnValue(of({ boards: [] }));

    await TestBed.configureTestingModule({
      imports: [AdminUserDetailComponent],
      providers: [
        provideRouter([]),
        provideHttpClient(),
        { provide: AdminService, useValue: adminService },
        { provide: ActivatedRoute, useValue: { snapshot: { paramMap: convertToParamMap({ id: '12' }) } } },
      ],
    })
      // The real Pengaturan page is covered by its own spec.
      .overrideComponent(AdminUserDetailComponent, { remove: { imports: [SettingsComponent] }, add: { imports: [SettingsStubComponent] } })
      .compileComponents();

    fixture = TestBed.createComponent(AdminUserDetailComponent);
  });

  it('shows the profile and the settings for the user', () => {
    adminService.getUser.and.returnValue(of(USER));
    fixture.detectChanges();

    expect(adminService.getUser).toHaveBeenCalledWith(12);
    expect(el().querySelector('.profile-card')!.textContent).toContain('fernando.marco@esb.co.id');
    expect(el().querySelector('.profile-card')!.textContent).toContain('••••50CF');
    expect(el().querySelector('.admin-editing-banner')!.textContent).toContain('fernando.marco@esb.co.id');
    expect(el().querySelector('.settings-stub')).not.toBeNull();
  });

  it('points SettingsApi at the admin endpoints in admin mode', () => {
    adminService.getUser.and.returnValue(of(USER));
    fixture.detectChanges();

    const injector = fixture.debugElement.injector;
    expect(injector.get(SettingsApi)).toBe(injector.get(AdminUserSettingsApi));
    expect(injector.get(SETTINGS_MODE)).toBe('admin');
  });

  it('shows "User tidak ditemukan" on 404', () => {
    adminService.getUser.and.returnValue(throwError(() => ({ status: 404 })));
    fixture.detectChanges();

    expect(el().textContent).toContain('User tidak ditemukan');
    expect(el().querySelector('a[href="/admin/users"]')).not.toBeNull();
    expect(el().querySelector('.settings-stub')).toBeNull();
  });

  it('toggles the user active status from the profile card', () => {
    adminService.getUser.and.returnValue(of(USER));
    adminService.toggleActive.and.returnValue(of({}));
    fixture.detectChanges();

    (el().querySelector('.profile-grid button') as HTMLButtonElement).click();
    expect(adminService.toggleActive).toHaveBeenCalledWith(12, false);
  });
});
