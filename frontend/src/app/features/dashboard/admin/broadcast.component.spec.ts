import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { BroadcastComponent } from './broadcast.component';
import { AdminService } from '../../../core/services/admin.service';

describe('BroadcastComponent', () => {
  let fixture: ComponentFixture<BroadcastComponent>;
  let adminService: jasmine.SpyObj<AdminService>;

  beforeEach(async () => {
    adminService = jasmine.createSpyObj('AdminService', ['broadcast']);

    await TestBed.configureTestingModule({
      imports: [BroadcastComponent],
      providers: [{ provide: AdminService, useValue: adminService }],
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

  it('does nothing if the confirmation dialog is cancelled', () => {
    fixture.componentInstance['message'] = 'Update baru!';
    spyOn(window, 'confirm').and.returnValue(false);

    fixture.componentInstance['onSend']();

    expect(adminService.broadcast).not.toHaveBeenCalled();
  });

  it('shows the sent count and clears the textarea on full success', () => {
    fixture.componentInstance['message'] = 'Update baru!';
    spyOn(window, 'confirm').and.returnValue(true);
    adminService.broadcast.and.returnValue(of({ sentCount: 5, failedEmails: [] }));

    fixture.componentInstance['onSend']();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('Terkirim ke 5 user');
    expect(fixture.componentInstance['message']).toBe('');
  });

  it('shows the failed email list and marks the result as an error when some sends fail', () => {
    fixture.componentInstance['message'] = 'Update baru!';
    spyOn(window, 'confirm').and.returnValue(true);
    adminService.broadcast.and.returnValue(of({ sentCount: 3, failedEmails: ['a@x.com', 'b@y.com'] }));

    fixture.componentInstance['onSend']();
    fixture.detectChanges();

    const msg = fixture.nativeElement.querySelector('.msg');
    expect(msg.textContent).toContain('Terkirim ke 3 user');
    expect(msg.textContent).toContain('a@x.com, b@y.com');
    expect(msg.classList).toContain('error');
  });

  it('shows the DM-not-configured message on a 503 response and keeps the textarea', () => {
    fixture.componentInstance['message'] = 'Update baru!';
    spyOn(window, 'confirm').and.returnValue(true);
    adminService.broadcast.and.returnValue(throwError(() => ({ status: 503 })));

    fixture.componentInstance['onSend']();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('personal DM (OAuth) belum pernah di-setup di server ini');
    expect(fixture.componentInstance['message']).toBe('Update baru!');
  });

  it('shows a generic error message on any other failure and keeps the textarea', () => {
    fixture.componentInstance['message'] = 'Update baru!';
    spyOn(window, 'confirm').and.returnValue(true);
    adminService.broadcast.and.returnValue(throwError(() => ({ status: 500 })));

    fixture.componentInstance['onSend']();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('gagal mengirim broadcast, coba lagi');
    expect(fixture.componentInstance['message']).toBe('Update baru!');
  });
});
