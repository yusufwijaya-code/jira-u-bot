import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { ChatSenderCardComponent } from './chat-sender-card.component';
import { ChatAppService, ChatAppStatus } from '../../../core/services/chat-app.service';
import { ToastService } from '../../../core/ui/toast.service';

const BASE: ChatAppStatus = {
  enabled: true, activated: true, activatedDate: '2026-11-15T02:00:00Z', oneClick: true,
  sender: null, effectiveSender: 'nudge', operatorAccount: false, operatorName: 'Yusuf Wijaya',
};

describe('ChatSenderCardComponent', () => {
  let fixture: ComponentFixture<ChatSenderCardComponent>;
  let service: jasmine.SpyObj<ChatAppService>;
  let toast: jasmine.SpyObj<ToastService>;
  const el = () => fixture.nativeElement as HTMLElement;
  const radio = (v: string) => el().querySelector(`input[value="${v}"]`) as HTMLInputElement;

  function setup(status: ChatAppStatus, userId: number | null = null) {
    service = jasmine.createSpyObj('ChatAppService', ['load', 'setSender', 'activateUrl']);
    service.load.and.returnValue(of(status));
    service.activateUrl.and.returnValue('/auth/chat-app/activate?then=sender');
    toast = jasmine.createSpyObj('ToastService', ['success', 'error']);
    TestBed.configureTestingModule({
      imports: [ChatSenderCardComponent],
      providers: [{ provide: ChatAppService, useValue: service }, { provide: ToastService, useValue: toast }],
    });
    fixture = TestBed.createComponent(ChatSenderCardComponent);
    fixture.componentRef.setInput('userId', userId);
    fixture.detectChanges();
  }

  it('shows both senders with the effective one selected', () => {
    setup(BASE);
    expect(radio('nudge').checked).toBeTrue();
    expect(el().textContent).toContain('Yusuf Wijaya');
  });

  it('saves the operator choice', () => {
    setup(BASE);
    service.setSender.and.returnValue(of({ ...BASE, sender: 'operator', effectiveSender: 'operator' }));
    radio('operator').click();
    fixture.detectChanges();
    expect(service.setSender).toHaveBeenCalledWith('operator', null);
    expect(radio('operator').checked).toBeTrue();
    expect(toast.success).toHaveBeenCalled();
  });

  it('asks to activate first when picking Nudge before activation', () => {
    setup({ ...BASE, activated: false, effectiveSender: 'operator' });
    radio('nudge').click();
    fixture.detectChanges();
    expect(service.setSender).not.toHaveBeenCalled();
    expect(radio('operator').checked).toBeTrue();
    expect(el().querySelector('[data-activate-first]')).not.toBeNull();
  });

  it('locks the operator account to Nudge', () => {
    setup({ ...BASE, operatorAccount: true });
    expect(radio('operator')).toBeNull();
    expect(radio('nudge').disabled).toBeTrue();
  });

  it('admin mode loads and saves for the target user and cannot pick Nudge before activation', () => {
    setup({ ...BASE, activated: false, effectiveSender: 'operator' }, 42);
    expect(service.load).toHaveBeenCalledWith(42);
    expect(radio('nudge').disabled).toBeTrue();
  });

  it('shows the error and keeps the old choice when saving fails', () => {
    setup(BASE);
    service.setSender.and.returnValue(throwError(() => ({ error: { error: 'gagal' } })));
    radio('operator').click();
    fixture.detectChanges();
    expect(toast.error).toHaveBeenCalled();
    expect(radio('nudge').checked).toBeTrue();
  });

  it('renders nothing when the app is off', () => {
    setup({ ...BASE, enabled: false });
    expect(el().querySelector('[data-card="chat-sender"]')).toBeNull();
  });
});
