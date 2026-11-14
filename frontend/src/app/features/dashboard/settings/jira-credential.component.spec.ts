import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { JiraCredentialComponent } from './jira-credential.component';
import { BoardService } from '../../../core/services/board.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';

describe('JiraCredentialComponent', () => {
  let fixture: ComponentFixture<JiraCredentialComponent>;
  let boardService: jasmine.SpyObj<BoardService>;
  let confirmService: jasmine.SpyObj<ConfirmService>;
  let toastService: jasmine.SpyObj<ToastService>;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', ['saveCredential', 'deleteCredential']);
    confirmService = jasmine.createSpyObj('ConfirmService', ['ask']);
    toastService = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);

    await TestBed.configureTestingModule({
      imports: [JiraCredentialComponent],
      providers: [
        { provide: BoardService, useValue: boardService },
        { provide: ConfirmService, useValue: confirmService },
        { provide: ToastService, useValue: toastService },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(JiraCredentialComponent);
  });

  it('shows the form when there is no credential yet', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('form')).not.toBeNull();
    expect(fixture.nativeElement.querySelector('.cred-status')).toBeNull();
  });

  it('shows the masked credential and Hapus button when a credential already exists', () => {
    fixture.componentRef.setInput('hasCredential', true);
    fixture.componentRef.setInput('credentialMasked', '••••ab3f');
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('form')).toBeNull();
    expect(fixture.nativeElement.querySelector('.cred-status').textContent).toContain('••••ab3f');
  });

  it('emits credentialChanged after a successful save', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();
    boardService.saveCredential.and.returnValue(of({ status: 'ok' }));
    const emitted = jasmine.createSpy('emitted');
    fixture.componentInstance.credentialChanged.subscribe(emitted);

    fixture.componentInstance['form'].setValue({ jiraEmail: 'a@esb.co.id', apiToken: 'tok123' });
    fixture.componentInstance['onSubmit']();

    expect(boardService.saveCredential).toHaveBeenCalledWith('a@esb.co.id', 'tok123');
    expect(emitted).toHaveBeenCalled();
    expect(toastService.success).toHaveBeenCalled();
  });

  it('shows the encryption-not-configured message on a 503 response', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();
    boardService.saveCredential.and.returnValue(throwError(() => ({ status: 503 })));

    fixture.componentInstance['form'].setValue({ jiraEmail: 'a@esb.co.id', apiToken: 'tok123' });
    fixture.componentInstance['onSubmit']();
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('fitur ini belum dikonfigurasi server, hubungi admin');
  });

  it('shows a generic error message on any other save failure', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();
    boardService.saveCredential.and.returnValue(throwError(() => ({ status: 500 })));

    fixture.componentInstance['form'].setValue({ jiraEmail: 'a@esb.co.id', apiToken: 'tok123' });
    fixture.componentInstance['onSubmit']();
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('failed to save credential');
  });

  it('emits credentialChanged after a confirmed delete', async () => {
    fixture.componentRef.setInput('hasCredential', true);
    fixture.componentRef.setInput('credentialMasked', '••••ab3f');
    fixture.detectChanges();
    confirmService.ask.and.resolveTo(true);
    boardService.deleteCredential.and.returnValue(of({ status: 'ok' }));
    const emitted = jasmine.createSpy('emitted');
    fixture.componentInstance.credentialChanged.subscribe(emitted);

    await fixture.componentInstance['onDelete']();

    expect(confirmService.ask).toHaveBeenCalledWith(jasmine.objectContaining({ tone: 'danger' }));
    expect(emitted).toHaveBeenCalled();
    expect(toastService.success).toHaveBeenCalledWith('Credential dihapus');
  });

  it('does not delete anything when the confirmation dialog is cancelled', async () => {
    fixture.componentRef.setInput('hasCredential', true);
    fixture.detectChanges();
    confirmService.ask.and.resolveTo(false);

    await fixture.componentInstance['onDelete']();

    expect(boardService.deleteCredential).not.toHaveBeenCalled();
  });

  it('does not show field errors when an empty field is merely focused and left', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();

    fixture.componentInstance['form'].controls.jiraEmail.markAsTouched();
    fixture.componentInstance['form'].controls.apiToken.markAsTouched();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelectorAll('.field-error').length).toBe(0);
  });

  it('shows the field errors after a submit attempt with an empty form, without calling the API', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();

    fixture.componentInstance['onSubmit']();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelectorAll('.field-error').length).toBe(2);
    expect(boardService.saveCredential).not.toHaveBeenCalled();
  });
});
