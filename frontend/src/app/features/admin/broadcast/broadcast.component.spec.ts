import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { BroadcastComponent } from './broadcast.component';
import { AdminService } from '../../../core/services/admin.service';
import { ConfirmService } from '../../../core/ui/confirm.service';

describe('BroadcastComponent', () => {
  let fixture: ComponentFixture<BroadcastComponent>;
  let adminService: jasmine.SpyObj<AdminService>;
  let confirmService: jasmine.SpyObj<ConfirmService>;

  beforeEach(async () => {
    adminService = jasmine.createSpyObj('AdminService', ['broadcast']);
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
});
