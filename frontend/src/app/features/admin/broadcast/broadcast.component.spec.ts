import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { BroadcastComponent, chatPreviewHtml } from './broadcast.component';
import { AdminService } from '../../../core/services/admin.service';
import { ConfirmService } from '../../../core/ui/confirm.service';

describe('BroadcastComponent', () => {
  let fixture: ComponentFixture<BroadcastComponent>;
  let adminService: jasmine.SpyObj<AdminService>;
  let confirmService: jasmine.SpyObj<ConfirmService>;

  beforeEach(async () => {
    adminService = jasmine.createSpyObj('AdminService', ['broadcast', 'listUsers']);
    adminService.listUsers.and.returnValue(of({
      users: [
        { userId: 1, email: 'a@x.com', userName: '', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatAppActivated: true, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'nudge' as const },
        { userId: 2, email: 'b@x.com', userName: '', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const },
        { userId: 3, email: 'c@x.com', userName: '', flagActive: false, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const },
      ],
    }));
    confirmService = jasmine.createSpyObj('ConfirmService', ['ask']);

    await TestBed.configureTestingModule({
      imports: [BroadcastComponent],
      providers: [
        { provide: AdminService, useValue: adminService },
        { provide: ConfirmService, useValue: confirmService },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(BroadcastComponent);
    fixture.detectChanges();
  });

  it('disables the Kirim button when the textarea is empty or only whitespace', () => {
    fixture.componentInstance['message'] = '   ';
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button').disabled).toBe(true);
  });

  it('enables the Kirim button when there is non-whitespace text', () => {
    fixture.componentInstance['message'] = 'Update baru!';
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button').disabled).toBe(false);
  });

  it('does nothing if the confirmation dialog is cancelled', async () => {
    fixture.componentInstance['message'] = 'Update baru!';
    confirmService.ask.and.resolveTo(false);

    await fixture.componentInstance['onSend']();

    expect(adminService.broadcast).not.toHaveBeenCalled();
  });

  it('shows the sent count and clears the textarea on full success', async () => {
    fixture.componentInstance['message'] = 'Update baru!';
    confirmService.ask.and.resolveTo(true);
    adminService.broadcast.and.returnValue(of({ sentCount: 5, failedEmails: [] }));

    await fixture.componentInstance['onSend']();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('Terkirim ke 5 user');
    expect(fixture.componentInstance['message']).toBe('');
  });

  it('shows the failed email list and marks the result as an error when some sends fail', async () => {
    fixture.componentInstance['message'] = 'Update baru!';
    confirmService.ask.and.resolveTo(true);
    adminService.broadcast.and.returnValue(of({ sentCount: 3, failedEmails: ['a@x.com', 'b@y.com'] }));

    await fixture.componentInstance['onSend']();
    fixture.detectChanges();

    const msg = fixture.nativeElement.querySelector('.msg');
    expect(msg.textContent).toContain('Terkirim ke 3 user');
    expect(msg.textContent).toContain('a@x.com, b@y.com');
    expect(msg.classList).toContain('error');
  });

  it('lists skipped recipients without marking the result as an error', async () => {
    fixture.componentInstance['message'] = 'Update baru!';
    confirmService.ask.and.resolveTo(true);
    adminService.broadcast.and.returnValue(of({ sentCount: 2, failedEmails: [], skippedEmails: ['op@x.com'] }));

    await fixture.componentInstance['onSend']();
    fixture.detectChanges();

    const msg = fixture.nativeElement.querySelector('.msg');
    expect(msg.textContent).toContain('dilewati (belum bisa dihubungi): op@x.com');
    expect(msg.classList).not.toContain('error');
  });

  it('shows the DM-not-configured message on a 503 response and keeps the textarea', async () => {
    fixture.componentInstance['message'] = 'Update baru!';
    confirmService.ask.and.resolveTo(true);
    adminService.broadcast.and.returnValue(throwError(() => ({ status: 503 })));

    await fixture.componentInstance['onSend']();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('personal DM (OAuth) belum pernah di-setup di server ini');
    expect(fixture.componentInstance['message']).toBe('Update baru!');
  });

  it('shows a generic error message on any other failure and keeps the textarea', async () => {
    fixture.componentInstance['message'] = 'Update baru!';
    confirmService.ask.and.resolveTo(true);
    adminService.broadcast.and.returnValue(throwError(() => ({ status: 500 })));

    await fixture.componentInstance['onSend']();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('gagal mengirim broadcast, coba lagi');
    expect(fixture.componentInstance['message']).toBe('Update baru!');
  });

  it('summarises the recipients from the active users', () => {
    const text = fixture.nativeElement.querySelector('[data-recipients]').textContent;
    expect(text).toContain('Dikirim ke 2 user aktif');
    expect(text).toContain('1 sudah aktivasi Nudge App');
  });

  it('falls back to a note when the user list cannot be loaded', () => {
    adminService.listUsers.and.returnValue(throwError(() => ({ status: 500 })));
    fixture = TestBed.createComponent(BroadcastComponent);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[data-recipients]').textContent).toContain('Jumlah penerima belum bisa dimuat');
  });

  it('previews the message with Google Chat markup and escapes HTML', () => {
    fixture.componentInstance['message'] = '*Baru* _cek_ <b>x</b>' + String.fromCharCode(10) + 'baris 2';
    fixture.detectChanges();
    const preview = fixture.nativeElement.querySelector('[data-preview]') as HTMLElement;
    expect(preview.querySelector('strong')?.textContent).toBe('Baru');
    expect(preview.querySelector('em')?.textContent).toBe('cek');
    expect(preview.querySelector('br')).not.toBeNull();
    expect(preview.querySelector('b')).toBeNull();
    expect(preview.textContent).toContain('<b>x</b>');
  });

  it('keeps word-internal underscores and asterisks as plain text', () => {
    expect(chatPreviewHtml('snake_case_name 2*3*4')).toBe('snake_case_name 2*3*4');
  });

  it('turns the counter to warning and danger near the limit', () => {
    fixture.componentInstance['message'] = 'x'.repeat(1700);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.bc-meter').getAttribute('data-level')).toBe('warn');
    fixture.componentInstance['message'] = 'x'.repeat(1950);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.bc-meter').getAttribute('data-level')).toBe('danger');
    expect(fixture.nativeElement.querySelector('.char-count').textContent).toContain('1950 / 2000');
  });
});
