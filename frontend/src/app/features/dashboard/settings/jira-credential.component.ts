import { Component, OnInit, inject, input, output } from '@angular/core';
import { FormBuilder, FormGroup, ReactiveFormsModule, Validators, FormControl } from '@angular/forms';
import { NgIf } from '@angular/common';
import { BoardService } from '../../../core/services/board.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';

@Component({
  selector: 'app-jira-credential',
  standalone: true,
  imports: [ReactiveFormsModule, NgIf, IconComponent],
  templateUrl: './jira-credential.component.html',
})
export class JiraCredentialComponent implements OnInit {
  hasCredential = input<boolean>(false);
  credentialMasked = input<string | null>(null);
  credentialChanged = output<void>();

  protected form!: FormGroup<{ jiraEmail: FormControl<string>; apiToken: FormControl<string> }>;
  protected submitting = false;
  protected deleting = false;
  protected showToken = false;
  protected error: string | null = null;

  private fb = inject(FormBuilder);
  private boardService = inject(BoardService);
  private confirm = inject(ConfirmService);
  private toast = inject(ToastService);

  ngOnInit(): void {
    this.form = this.fb.group({
      jiraEmail: ['', [Validators.required, Validators.email]],
      apiToken: ['', Validators.required],
    }) as FormGroup<{ jiraEmail: FormControl<string>; apiToken: FormControl<string> }>;
  }

  protected onSubmit(): void {
    if (this.form.invalid || this.submitting) {
      this.form.markAllAsTouched();
      return;
    }
    this.submitting = true;
    this.error = null;

    const value = this.form.getRawValue();
    this.boardService.saveCredential(value.jiraEmail, value.apiToken).subscribe({
      next: () => {
        this.submitting = false;
        this.toast.success('Credential tersimpan', 'Aksi Jira sekarang memakai akun kamu sendiri.');
        this.credentialChanged.emit();
      },
      error: (err) => {
        this.submitting = false;
        this.error = err?.status === 503 ? 'fitur ini belum dikonfigurasi server, hubungi admin' : 'failed to save credential';
      },
    });
  }

  protected async onDelete(): Promise<void> {
    const confirmed = await this.confirm.ask({
      title: 'Hapus credential?',
      message: 'Aksi Jira kamu akan kembali memakai akun bot pusat. Kamu bisa menambahkannya lagi kapan saja.',
      confirmText: 'Hapus',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }

    this.deleting = true;
    this.boardService.deleteCredential().subscribe({
      next: () => {
        this.deleting = false;
        this.toast.success('Credential dihapus');
        this.credentialChanged.emit();
      },
      error: () => {
        this.deleting = false;
        this.error = 'failed to delete credential';
      },
    });
  }
}
