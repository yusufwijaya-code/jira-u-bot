import { ComponentFixture, TestBed } from '@angular/core/testing';
import { WritableSignal, signal } from '@angular/core';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import { BehaviorSubject } from 'rxjs';
import { ChatAppBannerComponent } from './chat-app-banner.component';
import { ChatAppService, ChatAppStatus } from '../../core/services/chat-app.service';
import { ToastService } from '../../core/ui/toast.service';

const BASE: ChatAppStatus = {
  enabled: true, activated: false, activatedDate: null, oneClick: true,
  sender: null, effectiveSender: 'operator', operatorAccount: false, operatorName: 'Yusuf Wijaya',
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

  it('turns ?chatApp= into a toast, clears it and refreshes', () => {
    setup(BASE, { chatApp: 'activated' });
    expect(toast.show).toHaveBeenCalledWith('success', 'Nudge App aktif', jasmine.any(String));
    expect(router.navigate).toHaveBeenCalled();
    expect(chatApp.refresh).toHaveBeenCalledTimes(2);
  });
});
