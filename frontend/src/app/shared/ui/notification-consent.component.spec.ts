import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { NotificationConsentComponent } from './notification-consent.component';
import { NotificationSwitchService } from '../../core/services/notification-switch.service';
import { ToastService } from '../../core/ui/toast.service';
import { environment } from '../../../environments/environment';

describe('NotificationConsentComponent', () => {
  let fixture: ComponentFixture<NotificationConsentComponent>;
  let http: HttpTestingController;
  let notifSwitch: NotificationSwitchService;
  let toast: jasmine.SpyObj<ToastService>;
  const url = `${environment.apiBasePath}/api/dashboard/settings/notifications`;
  const el = () => fixture.nativeElement as HTMLElement;
  const dialog = () => el().querySelector('[role="dialog"]');
  const toggle = () => el().querySelector('[data-consent-switch]') as HTMLInputElement;
  const saveBtn = () => el().querySelector('[data-consent-save]') as HTMLButtonElement;

  beforeEach(() => {
    toast = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);
    TestBed.configureTestingModule({
      imports: [NotificationConsentComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), { provide: ToastService, useValue: toast }],
    });
    http = TestBed.inject(HttpTestingController);
    notifSwitch = TestBed.inject(NotificationSwitchService);
    fixture = TestBed.createComponent(NotificationConsentComponent);
    fixture.detectChanges();
  });

  afterEach(() => http.verify());

  it('stays hidden until the switch is known, and when the user already chose', () => {
    expect(dialog()).toBeNull();
    notifSwitch.set({ enabled: true, chosen: true });
    fixture.detectChanges();
    expect(dialog()).toBeNull();
  });

  it('asks a user who has not chosen yet with the switch OFF — even if notifications are on today', () => {
    notifSwitch.set({ enabled: true, chosen: false });
    fixture.detectChanges();
    expect(dialog()).not.toBeNull();
    expect(toggle().checked).toBeFalse();
    expect(el().textContent).toContain('Notifikasi mati');
    // No way out but saving: no close button, the backdrop and Esc do nothing.
    expect(el().querySelector('.icon-btn, [aria-label="Tutup"]')).toBeNull();
    (el().querySelector('.modal-backdrop') as HTMLElement).click();
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    fixture.detectChanges();
    expect(dialog()).not.toBeNull();
  });

  it('saves the picked value and closes', () => {
    notifSwitch.set({ enabled: false, chosen: false });
    fixture.detectChanges();
    toggle().click();
    fixture.detectChanges();
    expect(el().textContent).toContain('Notifikasi aktif');

    saveBtn().click();
    const req = http.expectOne(url);
    expect(req.request.method).toBe('PUT');
    expect(req.request.body).toEqual({ enabled: true });
    req.flush({ enabled: true, chosen: true });
    fixture.detectChanges();

    expect(dialog()).toBeNull();
    expect(notifSwitch.state()).toEqual({ enabled: true, chosen: true });
    expect(toast.success).toHaveBeenCalled();
  });

  it('keeps the dialog open with an error when saving fails', () => {
    notifSwitch.set({ enabled: false, chosen: false });
    fixture.detectChanges();
    saveBtn().click();
    http.expectOne(url).flush({ error: 'x' }, { status: 500, statusText: 'err' });
    fixture.detectChanges();

    expect(dialog()).not.toBeNull();
    expect(el().querySelector('[role="alert"]')?.textContent).toContain('Gagal menyimpan');
    expect(saveBtn().disabled).toBeFalse();
  });
});
