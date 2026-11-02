import { Component, OnInit, inject, input, output } from '@angular/core';
import { FormBuilder, FormGroup, ReactiveFormsModule, Validators, FormControl } from '@angular/forms';
import { NgIf } from '@angular/common';
import { BoardService } from '../../../core/services/board.service';

@Component({
  selector: 'app-jira-credential',
  standalone: true,
  imports: [ReactiveFormsModule, NgIf],
  templateUrl: './jira-credential.component.html',
})
export class JiraCredentialComponent implements OnInit {
  hasCredential = input<boolean>(false);
  credentialMasked = input<string | null>(null);
  credentialChanged = output<void>();

  protected form!: FormGroup<{ jiraEmail: FormControl<string>; apiToken: FormControl<string> }>;
  protected submitting = false;
  protected error: string | null = null;

  private fb = inject(FormBuilder);
  private boardService = inject(BoardService);

  ngOnInit(): void {
    this.form = this.fb.group({
      jiraEmail: ['', [Validators.required, Validators.email]],
      apiToken: ['', Validators.required],
    }) as FormGroup<{ jiraEmail: FormControl<string>; apiToken: FormControl<string> }>;
  }

  protected onSubmit(): void {
    if (this.form.invalid || this.submitting) {
      return;
    }
    this.submitting = true;
    this.error = null;

    const value = this.form.getRawValue();
    this.boardService.saveCredential(value.jiraEmail, value.apiToken).subscribe({
      next: () => {
        this.submitting = false;
        this.credentialChanged.emit();
      },
      error: (err) => {
        this.submitting = false;
        this.error = err?.status === 503 ? 'fitur ini belum dikonfigurasi server, hubungi admin' : 'failed to save credential';
      },
    });
  }

  protected onDelete(): void {
    this.boardService.deleteCredential().subscribe({
      next: () => this.credentialChanged.emit(),
      error: () => {
        this.error = 'failed to delete credential';
      },
    });
  }
}
