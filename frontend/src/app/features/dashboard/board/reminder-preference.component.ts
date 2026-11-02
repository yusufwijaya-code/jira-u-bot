import { Component, OnInit, inject, input } from '@angular/core';
import { FormBuilder, FormGroup, ReactiveFormsModule } from '@angular/forms';
import { NgFor } from '@angular/common';
import { BoardService } from '../../../core/services/board.service';

interface TierOption {
  code: string;
  label: string;
}

const TIER_OPTIONS: TierOption[] = [
  { code: '2h', label: 'T-2 jam' },
  { code: '1h', label: 'T-1 jam' },
  { code: '30m', label: 'T-30 menit' },
  { code: '15m', label: 'T-15 menit' },
  { code: '5m', label: 'T-5 menit' },
  { code: '1m', label: 'T-1 menit' },
];

@Component({
  selector: 'app-reminder-preference',
  standalone: true,
  imports: [ReactiveFormsModule, NgFor],
  templateUrl: './reminder-preference.component.html',
})
export class ReminderPreferenceComponent implements OnInit {
  tiers = input<string[]>([]);

  protected tierOptions = TIER_OPTIONS;
  protected submitting = false;
  protected message: { text: string; isError: boolean } | null = null;
  protected form!: FormGroup;

  private fb = inject(FormBuilder);
  private boardService = inject(BoardService);

  ngOnInit(): void {
    this.form = this.fb.group(
      Object.fromEntries(TIER_OPTIONS.map((opt) => [opt.code, this.tiers().includes(opt.code)])),
    );
  }

  protected onSubmit(): void {
    this.submitting = true;
    const selected = TIER_OPTIONS.filter((opt) => this.form.value[opt.code]).map((opt) => opt.code);

    this.boardService.updatePreference(selected).subscribe({
      next: () => {
        this.submitting = false;
        this.message = { text: 'Preference tersimpan ✅', isError: false };
      },
      error: (err) => {
        this.submitting = false;
        this.message = { text: err?.error?.error ?? 'Gagal menyimpan', isError: true };
      },
    });
  }
}
