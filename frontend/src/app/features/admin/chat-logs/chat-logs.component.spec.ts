import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter, Router } from '@angular/router';
import { ChatLogsComponent } from './chat-logs.component';
import { ChatLogEntry } from '../../../core/services/admin.service';

const entry = (id: number, over: Partial<ChatLogEntry> = {}): ChatLogEntry => ({
  id,
  email: 'dev@esb.co.id',
  direction: 'out',
  kind: 'reminder',
  channel: 'app',
  status: 'sent',
  body: 'Reminder CORE-' + id,
  createdDate: '2026-11-15T11:44:00Z',
  ...over,
});

describe('ChatLogsComponent', () => {
  let fixture: ComponentFixture<ChatLogsComponent>;
  let http: HttpTestingController;
  const el = () => fixture.nativeElement as HTMLElement;

  async function create(url = '/admin/chat-logs'): Promise<void> {
    TestBed.configureTestingModule({
      imports: [ChatLogsComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([{ path: 'admin/chat-logs', children: [] }])],
    });
    http = TestBed.inject(HttpTestingController);
    await TestBed.inject(Router).navigateByUrl(url);
    fixture = TestBed.createComponent(ChatLogsComponent);
  }

  function flushUsers(): void {
    http.expectOne((r) => r.url.endsWith('/api/dashboard/admin/users')).flush({
      users: [{ userId: 1, email: 'dev@esb.co.id', userName: 'Dev' }],
    });
  }

  afterEach(() => http.verify());

  it('lists entries newest first with labels and loads older pages', async () => {
    await create();
    fixture.detectChanges();
    flushUsers();
    const req = http.expectOne((r) => r.url.endsWith('/api/dashboard/admin/chat-logs'));
    expect(req.request.params.get('limit')).toBe('50');
    req.flush({ items: [entry(3), entry(2, { direction: 'in', kind: 'command', channel: 'app', status: 'received', body: '/n-help' })], nextBeforeID: 2 });
    fixture.detectChanges();

    const items = el().querySelectorAll('.log-item');
    expect(items.length).toBe(2);
    expect(items[0].textContent).toContain('Keluar');
    expect(items[0].textContent).toContain('Reminder');
    expect(items[0].textContent).toContain('Nudge app');
    expect(items[0].textContent).toContain('Terkirim');
    expect(items[1].textContent).toContain('Masuk');
    expect(items[1].textContent).toContain('/n-help');
    expect(items[0].textContent).toContain('Dev');

    (el().querySelector('[data-load-more]') as HTMLButtonElement).click();
    const more = http.expectOne((r) => r.url.endsWith('/chat-logs'));
    expect(more.request.params.get('beforeID')).toBe('2');
    more.flush({ items: [entry(1)], nextBeforeID: 0 });
    fixture.detectChanges();
    expect(el().querySelectorAll('.log-item').length).toBe(3);
    expect(el().querySelector('[data-load-more]')).toBeNull();
  });

  it('reads the email filter from the URL', async () => {
    await create('/admin/chat-logs?email=dev@esb.co.id');
    fixture.detectChanges();
    flushUsers();
    const req = http.expectOne((r) => r.url.endsWith('/chat-logs'));
    expect(req.request.params.get('email')).toBe('dev@esb.co.id');
    req.flush({ items: [], nextBeforeID: 0 });
    fixture.detectChanges();
    expect(el().textContent).toContain('Belum ada riwayat pesan');
  });

  it('shows an error when loading fails', async () => {
    await create();
    fixture.detectChanges();
    flushUsers();
    http.expectOne((r) => r.url.endsWith('/chat-logs')).flush({}, { status: 500, statusText: 'err' });
    fixture.detectChanges();
    expect(el().textContent).toContain('Gagal memuat riwayat pesan');
  });

  it('shows a failed send with its error', async () => {
    await create();
    fixture.detectChanges();
    flushUsers();
    http.expectOne((r) => r.url.endsWith('/chat-logs')).flush({ items: [entry(5, { status: 'failed', error: 'oauth down' })], nextBeforeID: 0 });
    fixture.detectChanges();
    expect(el().textContent).toContain('Gagal');
    expect(el().textContent).toContain('oauth down');
  });
});
