import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { ADMIN_TARGET_USER_ID, AdminUserScope, AdminUserSettingsApi } from './admin-user-settings';
import { NotificationSettings } from './board.service';

const API = '/api/dashboard/admin/users/12';

describe('AdminUserSettingsApi', () => {
  let api: AdminUserSettingsApi;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting(), { provide: ADMIN_TARGET_USER_ID, useValue: 12 }, AdminUserSettingsApi],
    });
    api = TestBed.inject(AdminUserSettingsApi);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('targets the admin endpoints of the user', () => {
    const settings = { reminderTiers: [] } as unknown as NotificationSettings;

    api.getSettings(28).subscribe();
    http.expectOne((r) => r.method === 'GET' && r.url === `${API}/settings` && r.params.get('board') === '28').flush({});

    api.saveNotificationSettings(settings, null).subscribe();
    const put = http.expectOne((r) => r.method === 'PUT' && r.url === `${API}/settings/notification`);
    expect(put.request.params.has('board')).toBeFalse();
    put.flush({});

    api.resetNotificationSettings(28).subscribe();
    http.expectOne((r) => r.method === 'DELETE' && r.url === `${API}/settings/notification` && r.params.get('board') === '28').flush({});

    api.mute({ minutes: 60 }, null).subscribe();
    http.expectOne((r) => r.method === 'POST' && r.url === `${API}/settings/mute`).flush({});

    api.unmute(28).subscribe();
    http.expectOne((r) => r.method === 'DELETE' && r.url === `${API}/settings/mute` && r.params.get('board') === '28').flush({});

    api.saveCredential('a@esb.co.id', 'tok').subscribe();
    const cred = http.expectOne((r) => r.method === 'PUT' && r.url === `${API}/credential`);
    expect(cred.request.body).toEqual({ jiraEmail: 'a@esb.co.id', apiToken: 'tok' });
    cred.flush({ status: 'ok' });

    api.deleteCredential().subscribe();
    http.expectOne((r) => r.method === 'DELETE' && r.url === '/dashboard/admin/users/12/credential').flush({ status: 'ok' });
  });
});

describe('AdminUserScope', () => {
  let scope: AdminUserScope;
  let http: HttpTestingController;
  let navigate: jasmine.Spy;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([]), { provide: ADMIN_TARGET_USER_ID, useValue: 12 }, AdminUserScope],
    });
    navigate = spyOn(TestBed.inject(Router), 'navigate').and.resolveTo(true);
    scope = TestBed.inject(AdminUserScope);
    http = TestBed.inject(HttpTestingController);
  });

  it('loads the target user boards', () => {
    scope.load();
    http.expectOne(`${API}/boards`).flush({ boards: [{ boardId: 28, name: 'OMS' }] });
    expect(scope.boards().map((b) => b.boardId)).toEqual([28]);
  });

  it('keeps the choice in the page URL, never in localStorage', () => {
    const setItem = spyOn(localStorage, 'setItem');
    scope.select(28);
    expect(scope.selectedId()).toBe(28);
    expect(navigate).toHaveBeenCalledWith([], jasmine.objectContaining({ queryParams: { board: 28 }, queryParamsHandling: 'merge' }));
    expect(setItem).not.toHaveBeenCalled();
  });

  it('falls back to the default scope when the selected board is gone', () => {
    scope.select(99);
    scope.load();
    http.expectOne(`${API}/boards`).flush({ boards: [] });
    expect(scope.selectedId()).toBeNull();
  });
});
