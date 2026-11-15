import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ChatAppService, ChatAppStatus } from './chat-app.service';

const STATUS: ChatAppStatus = {
  enabled: true, activated: false, activatedDate: null, oneClick: true,
  sender: null, effectiveSender: 'operator', operatorAccount: false, operatorName: 'Yusuf Wijaya',
};

describe('ChatAppService', () => {
  let service: ChatAppService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
    service = TestBed.inject(ChatAppService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('loads the own status into the signal', () => {
    service.refresh();
    http.expectOne('/api/dashboard/chat-app/status').flush(STATUS);
    expect(service.status()).toEqual(STATUS);
  });

  it('uses the admin endpoints for another user without touching the own signal', () => {
    service.setSender('operator', 42).subscribe();
    const req = http.expectOne('/api/dashboard/admin/users/42/settings/chat-sender');
    expect(req.request.method).toBe('PUT');
    expect(req.request.body).toEqual({ sender: 'operator' });
    req.flush({ ...STATUS, sender: 'operator' });
    expect(service.status()).toBeNull();
  });

  it('builds the activation URL', () => {
    expect(service.activateUrl()).toBe('/auth/chat-app/activate');
    expect(service.activateUrl('sender')).toBe('/auth/chat-app/activate?then=sender');
  });
});
