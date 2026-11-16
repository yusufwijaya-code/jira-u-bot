import { ComponentFixture, TestBed } from '@angular/core/testing';
import { WritableSignal, signal } from '@angular/core';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import { BehaviorSubject } from 'rxjs';
import { ChatAppBannerComponent } from './chat-app-banner.component';
import { ChatAppService, ChatAppStatus } from '../../core/services/chat-app.service';
import { ToastService } from '../../core/ui/toast.service';

const BASE: ChatAppStatus = {
  enabled: true, activated: false, activatedDate: null, oneClick: true,
  sender: null, effectiveSender: 'operator', operatorAccount: false, operatorName: 'Yusuf Wijaya', groupState: 'ready',
};

describe('ChatAppBannerComponent', () => {
  let fixture: ComponentFixture<ChatAppBannerComponent>;
  let chatApp: { status: WritableSignal<ChatAppStatus | null>; refresh: jasmine.Spy; activateUrl: () => string };
  let toast: jasmine.SpyObj<ToastService>;
  let query$: BehaviorSubject<any>;
  let router: jasmine.SpyObj<Router>;

  const el = () => fixture.nativeElement as HTMLElement;
  const banner = () => el().querySelector('.chat-app-banner');

  function setup(status: ChatAppStatus | null, params: Record<string, string> = {}) {
    try { localStorage.removeItem('nudge.chatAppBanner.snoozedUntil'); } catch { /* ignore */ }
    chatApp = { status: signal(status), refresh: jasmine.createSpy('refresh'), activateUrl: () => '/auth/chat-app/activate' };
    toast = jasmine.createSpyObj('ToastService', ['show']);
    router = jasmine.createSpyObj('Router', ['navigate']);
    query$ = new BehaviorSubject(convertToParamMap(params));
    TestBed.configureTestingModule({
      imports: [ChatAppBannerComponent],
      providers: [
        { provide: ChatAppService, useValue: chatApp },
        { provide: ToastService, useValue: toast },
        { provide: Router, useValue: router },
        { provide: ActivatedRoute, useValue: { queryParamMap: query$ } },
      ],
    });
    fixture = TestBed.createComponent(ChatAppBannerComponent);
    fixture.detectChanges();
  }

  it('invites a user who has not activated nor chosen a sender', () => {
    setup(BASE);
    expect(banner()?.textContent).toContain('Yusuf Wijaya');
    expect(el().querySelector('[data-activate]')).not.toBeNull();
    expect(el().querySelector('[data-snooze]')).not.toBeNull();
  });

  it('hides once activated, when a sender was chosen, or when the app is off', () => {
    for (const s of [{ ...BASE, activated: true }, { ...BASE, sender: 'operator' as const }, { ...BASE, enabled: false }]) {
      TestBed.resetTestingModule();
      setup(s);
      expect(banner()).toBeNull();
    }
  });

  it('cannot be dismissed by the operator account', () => {
    setup({ ...BASE, operatorAccount: true, sender: null });
    expect(banner()?.classList).toContain('required');
    expect(el().querySelector('[data-snooze]')).toBeNull();
  });

  it('"Nanti saja" hides it', () => {
    setup(BASE);
    (el().querySelector('[data-snooze]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(banner()).toBeNull();
  });

  it('offers "Buka Google Chat" when one-click is off', () => {
    setup({ ...BASE, oneClick: false });
    expect(el().querySelector('[data-activate]')).toBeNull();
    expect(el().querySelector('[data-open-chat]')).not.toBeNull();
  });

  it('keeps the manual steps and explains the Google unverified-app screen next to one-click', () => {
    setup(BASE);
    expect(el().querySelector('[data-manual]')).not.toBeNull();
    expect(el().querySelector('[data-unverified-note]')?.textContent).toContain('Go to labcup.tech (unsafe)');
    TestBed.resetTestingModule();
    setup({ ...BASE, oneClick: false });
    expect(el().querySelector('[data-unverified-note]')).toBeNull();
  });

  it('explains the wait while the group change propagates', () => {
    setup({ ...BASE, groupState: 'pending' });
    expect(el().querySelector('[data-group-pending]')?.textContent).toContain('App Nudge sedang disiapkan untuk akunmu (maks. 24 jam)');
    expect(el().querySelector('[data-activate]')).not.toBeNull();
  });

  it('does not promise a DM fallback to the operator account while pending', () => {
    setup({ ...BASE, operatorAccount: true, groupState: 'pending' });
    const text = el().querySelector('[data-group-pending]')?.textContent ?? '';
    expect(text).toContain('maks. 24 jam');
    expect(text).not.toContain('lewat DM');
  });

  it('tells the user to contact the admin when joining the group failed', () => {
    setup({ ...BASE, groupState: 'failed' });
    expect(el().querySelector('[data-group-failed]')?.textContent).toContain('Hubungi admin Nudge');
  });

  it('turns ?chatApp= into a toast, clears it and refreshes', () => {
    setup(BASE, { chatApp: 'activated' });
    expect(toast.show).toHaveBeenCalledWith('success', 'Nudge App aktif', jasmine.any(String));
    expect(router.navigate).toHaveBeenCalled();
    expect(chatApp.refresh).toHaveBeenCalledTimes(2);
  });
});
