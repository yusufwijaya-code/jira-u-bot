import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, TestRequest, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter, Router } from '@angular/router';
import { ChatLogsComponent } from './chat-logs.component';
import { ChatLogEntry } from '../../../core/services/admin.service';
import { formatChatBody } from './chat-format';

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

const USERS = [
  { userId: 1, email: 'dev@esb.co.id', userName: 'Dev' },
  { userId: 2, email: 'ana@esb.co.id', userName: 'Ana' },
];
const CONVS = [{ email: 'dev@esb.co.id', total: 2, lastDirection: 'out', lastKind: 'reply', lastStatus: 'sent', lastBody: 'halo', lastDate: '2026-11-15T11:44:00Z' }];

describe('ChatLogsComponent', () => {
  let fixture: ComponentFixture<ChatLogsComponent>;
  let http: HttpTestingController;
  const el = () => fixture.nativeElement as HTMLElement;
  const isLogs = (r: { url: string }) => r.url.endsWith('/api/dashboard/admin/chat-logs');
  const isConvs = (r: { url: string }) => r.url.endsWith('/chat-logs/conversations');

  async function create(url = '/admin/chat-logs'): Promise<void> {
    TestBed.configureTestingModule({
      imports: [ChatLogsComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([{ path: 'admin/chat-logs', children: [] }])],
    });
    http = TestBed.inject(HttpTestingController);
    await TestBed.inject(Router).navigateByUrl(url);
    fixture = TestBed.createComponent(ChatLogsComponent);
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/admin/users')).flush({ users: USERS });
  }

  function flushLogs(items: ChatLogEntry[], nextBeforeID = 0): TestRequest {
    const req = http.expectOne(isLogs);
    req.flush({ items, nextBeforeID });
    fixture.detectChanges();
    return req;
  }

  afterEach(() => {
    fixture?.destroy();
    http.verify();
  });

  it('opens the newest conversation as a chat: Nudge left, user right, day separator', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    // Newest first from the API; shown oldest → newest.
    const req = flushLogs([
      entry(3, { kind: 'reply', body: '*Bantuan* lihat https://nudge.labcup.tech' }),
      entry(2, { direction: 'in', kind: 'command', status: 'received', body: '/n-help' }),
    ]);
    expect(req.request.params.get('email')).toBe('dev@esb.co.id');

    const msgs = el().querySelectorAll('.chat-msg');
    expect(msgs.length).toBe(2);
    expect(msgs[0].classList).toContain('in');
    expect(msgs[0].textContent).toContain('/n-help');
    expect(msgs[1].classList).toContain('out');
    expect(msgs[1].querySelector('strong')?.textContent).toContain('Nudge');
    expect(msgs[1].querySelector('.chat-text strong')?.textContent).toBe('Bantuan');
    expect(msgs[1].querySelector('.chat-text a')?.getAttribute('href')).toBe('https://nudge.labcup.tech');
    expect(el().querySelector('.chat-day')).not.toBeNull();
    expect(el().querySelector('[data-user-picker]')?.textContent).toContain('Dev');
    expect(el().querySelector('.chat-live')).not.toBeNull();
  });

  it('uses the email from the URL and shows a failed send', async () => {
    await create('/admin/chat-logs?email=ana@esb.co.id');
    const req = http.expectOne(isLogs);
    expect(req.request.params.get('email')).toBe('ana@esb.co.id');
    req.flush({ items: [entry(5, { email: 'ana@esb.co.id', status: 'failed', error: 'oauth down' })], nextBeforeID: 0 });
    http.expectOne(isConvs).flush({ conversations: CONVS });
    fixture.detectChanges();
    expect(el().querySelector('.chat-msg')?.classList).toContain('failed');
    expect(el().textContent).toContain('Gagal terkirim');
    expect(el().textContent).toContain('oauth down');
  });

  it('picks exactly one user from the custom picker', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    flushLogs([entry(1)]);

    (el().querySelector('[data-user-picker]') as HTMLButtonElement).click();
    fixture.detectChanges();
    const options = el().querySelectorAll('.chat-user-option');
    expect(options.length).toBe(2);
    expect(options[1].textContent).toContain('Belum ada pesan');

    (el().querySelector('[data-email="ana@esb.co.id"]') as HTMLElement).click();
    fixture.detectChanges();
    const req = http.expectOne(isLogs);
    expect(req.request.params.get('email')).toBe('ana@esb.co.id');
    req.flush({ items: [], nextBeforeID: 0 });
    fixture.detectChanges();
    expect(el().querySelector('.chat-user-popup')).toBeNull();
    expect(el().textContent).toContain('Belum ada pesan untuk filter ini');
  });

  it('live: appends messages newer than the last one', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    flushLogs([entry(4)]);

    (fixture.componentInstance as unknown as { poll(): void }).poll();
    const req = http.expectOne(isLogs);
    expect(req.request.params.get('afterID')).toBe('4');
    req.flush({ items: [entry(6, { body: 'baru 6' }), entry(5, { body: 'baru 5' })], nextBeforeID: 0 });
    http.expectOne(isConvs).flush({ conversations: CONVS });
    fixture.detectChanges();

    const texts = Array.from(el().querySelectorAll('.chat-text')).map((t) => t.textContent);
    expect(texts).toEqual(['Reminder CORE-4', 'baru 5', 'baru 6']);
  });

  it('sends a message to the selected user and shows it', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    flushLogs([entry(4)]);

    const box = el().querySelector('[data-composer]') as HTMLTextAreaElement;
    box.value = '  Halo Dev  ';
    box.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    box.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));

    const req = http.expectOne((r) => r.url.endsWith('/chat-logs/send'));
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ email: 'dev@esb.co.id', text: 'Halo Dev' });
    req.flush({ status: 'sent' });
    const after = http.expectOne(isLogs);
    expect(after.request.params.get('afterID')).toBe('4');
    after.flush({ items: [entry(5, { kind: 'admin', body: 'Halo Dev' })], nextBeforeID: 0 });
    fixture.detectChanges();

    const last = Array.from(el().querySelectorAll('.chat-msg')).pop()!;
    expect(last.classList).toContain('out');
    expect(last.textContent).toContain('Halo Dev');
    expect(last.textContent).toContain('Pesan admin');
    // The live timer keeps the zone busy, so check the draft state itself.
    expect((fixture.componentInstance as unknown as { draft(): string }).draft()).toBe('');
  });

  it('does not send on Shift+Enter or an empty draft', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    flushLogs([]);
    const box = el().querySelector('[data-composer]') as HTMLTextAreaElement;
    box.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
    box.value = 'baris 1';
    box.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    box.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', shiftKey: true }));
    http.expectNone((r) => r.url.endsWith('/chat-logs/send'));
    expect((el().querySelector('[data-send]') as HTMLButtonElement).disabled).toBeFalse();
  });

  it('loads older messages above', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    flushLogs([entry(9)], 9);
    (el().querySelector('[data-load-older]') as HTMLButtonElement).click();
    const req = http.expectOne(isLogs);
    expect(req.request.params.get('beforeID')).toBe('9');
    req.flush({ items: [entry(8, { body: 'lama' })], nextBeforeID: 0 });
    fixture.detectChanges();
    expect(el().querySelector('.chat-text')?.textContent).toBe('lama');
    expect(el().querySelector('[data-load-older]')).toBeNull();
  });
});

describe('formatChatBody', () => {
  it('escapes HTML before adding markup', () => {
    expect(formatChatBody('<img src=x onerror=alert(1)> *tebal*')).toBe('&lt;img src=x onerror=alert(1)&gt; <strong>tebal</strong>');
  });

  it('formats italic, code and links without breaking URLs', () => {
    expect(formatChatBody('_miring_ `kode` https://a.b/c_d_e.')).toBe(
      '<em>miring</em> <code>kode</code> <a href="https://a.b/c_d_e" target="_blank" rel="noopener noreferrer">https://a.b/c_d_e</a>.',
    );
  });
});
