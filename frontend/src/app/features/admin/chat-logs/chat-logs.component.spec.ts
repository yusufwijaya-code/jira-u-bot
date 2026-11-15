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
  { userId: 1, email: 'dev@esb.co.id', userName: 'Dev', chatAppActivated: true, effectiveChatSender: 'nudge' },
  { userId: 2, email: 'ana@esb.co.id', userName: 'Ana', chatAppActivated: false, effectiveChatSender: 'operator' },
];
const CONVS = [{ email: 'dev@esb.co.id', total: 2, lastDirection: 'out', lastKind: 'reply', lastStatus: 'sent', lastBody: 'halo', lastDate: '2026-11-15T11:44:00Z' }];

describe('ChatLogsComponent', () => {
  let fixture: ComponentFixture<ChatLogsComponent>;
  let http: HttpTestingController;
  const el = () => fixture.nativeElement as HTMLElement;
  const isLogs = (r: { url: string }) => r.url.endsWith('/api/dashboard/admin/chat-logs');
  const isConvs = (r: { url: string }) => r.url.endsWith('/chat-logs/conversations');
  const isSend = (r: { url: string }) => r.url.endsWith('/chat-logs/send');

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

  function type(text: string): HTMLTextAreaElement {
    const box = el().querySelector('[data-composer]') as HTMLTextAreaElement;
    box.value = text;
    box.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    return box;
  }

  const component = () => fixture.componentInstance as unknown as { poll(): void; draft(): string };

  afterEach(() => {
    fixture?.destroy();
    http.verify();
  });

  it('lists conversations and opens the newest: user left, Nudge right', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    const req = flushLogs([
      entry(3, { kind: 'reply', body: '*Bantuan* lihat https://nudge.labcup.tech' }),
      entry(2, { direction: 'in', kind: 'command', status: 'received', body: '/n-help' }),
    ]);
    expect(req.request.params.get('email')).toBe('dev@esb.co.id');

    const convs = el().querySelectorAll('.chat-conv');
    expect(convs.length).toBe(2);
    expect(convs[0].classList).toContain('active');
    expect(convs[0].textContent).toContain('Nudge: halo');
    expect(convs[1].textContent).toContain('Belum ada pesan');

    const head = el().querySelector('.chat-head')!;
    expect(head.textContent).toContain('Dev');
    expect(head.textContent).toContain('Nudge aktif');
    expect(head.textContent).toContain('Pesan lewat app Nudge');

    const msgs = el().querySelectorAll('.chat-msg');
    expect(msgs[0].classList).toContain('in');
    expect(msgs[0].textContent).toContain('/n-help');
    expect(msgs[1].classList).toContain('out');
    expect(msgs[1].querySelector('.chat-text strong')?.textContent).toBe('Bantuan');
    expect(msgs[1].querySelector('.chat-text a')?.getAttribute('href')).toBe('https://nudge.labcup.tech');
    expect(el().querySelector('.chat-day')).not.toBeNull();
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
    expect(el().querySelector('.chat-head')?.textContent).toContain('Nudge belum aktif');
  });

  it('switches to another user from the list and filters it by search', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    flushLogs([entry(1)]);

    const search = el().querySelector('[data-search]') as HTMLInputElement;
    search.value = 'ana';
    search.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(el().querySelectorAll('.chat-conv').length).toBe(1);

    (el().querySelector('[data-email="ana@esb.co.id"]') as HTMLElement).click();
    fixture.detectChanges();
    const req = http.expectOne(isLogs);
    expect(req.request.params.get('email')).toBe('ana@esb.co.id');
    req.flush({ items: [], nextBeforeID: 0 });
    fixture.detectChanges();
    expect(el().querySelector('[data-email="ana@esb.co.id"]')?.classList).toContain('active');
    expect(el().textContent).toContain('Belum ada pesan');
  });

  it('live: appends messages newer than the last one', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    flushLogs([entry(4)]);

    component().poll();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    const req = http.expectOne(isLogs);
    expect(req.request.params.get('afterID')).toBe('4');
    req.flush({ items: [entry(6, { body: 'baru 6' }), entry(5, { body: 'baru 5' })], nextBeforeID: 0 });
    fixture.detectChanges();

    const texts = Array.from(el().querySelectorAll('.chat-text')).map((t) => t.textContent);
    expect(texts).toEqual(['Reminder CORE-4', 'baru 5', 'baru 6']);
  });

  it('sends: the bubble shows at once, then becomes the logged message', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    flushLogs([entry(4)]);

    const box = type('  Halo Dev  ');
    box.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
    fixture.detectChanges();
    const pending = el().querySelector('.chat-msg.pending');
    expect(pending?.textContent).toContain('Halo Dev');
    expect(pending?.textContent).toContain('Mengirim');
    expect(component().draft()).toBe('');

    const req = http.expectOne(isSend);
    expect(req.request.body).toEqual({ email: 'dev@esb.co.id', text: 'Halo Dev' });
    req.flush({ status: 'sent' });
    const after = http.expectOne(isLogs);
    expect(after.request.params.get('afterID')).toBe('4');
    after.flush({ items: [entry(5, { kind: 'admin', body: 'Halo Dev' })], nextBeforeID: 0 });
    fixture.detectChanges();

    expect(el().querySelector('.chat-msg.pending')).toBeNull();
    const last = Array.from(el().querySelectorAll('.chat-msg')).pop()!;
    expect(last.classList).toContain('out');
    expect(last.textContent).toContain('Pesan admin');
  });

  it('a failed send keeps the bubble with a retry', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    flushLogs([]);

    type('Tes');
    (el().querySelector('[data-send]') as HTMLButtonElement).click();
    http.expectOne(isSend).flush({ error: 'pesan gagal terkirim' }, { status: 502, statusText: 'Bad Gateway' });
    fixture.detectChanges();
    expect(el().querySelector('.chat-msg.failed')?.textContent).toContain('pesan gagal terkirim');

    (el().querySelector('[data-retry]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(http.expectOne(isSend).request.body).toEqual({ email: 'dev@esb.co.id', text: 'Tes' });
  });

  it('does not send on Shift+Enter or an empty draft', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    flushLogs([]);
    const box = el().querySelector('[data-composer]') as HTMLTextAreaElement;
    box.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
    type('baris 1').dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', shiftKey: true }));
    http.expectNone(isSend);
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

  it('opens the filters and shows the custom date and kind pickers', async () => {
    await create();
    http.expectOne(isConvs).flush({ conversations: CONVS });
    flushLogs([entry(1)]);
    expect(el().querySelector('app-date-range-picker')).toBeNull();
    (el().querySelector('[data-filter-toggle]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(el().querySelector('app-date-range-picker')).not.toBeNull();
    expect(el().querySelector('app-select')).not.toBeNull();
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
