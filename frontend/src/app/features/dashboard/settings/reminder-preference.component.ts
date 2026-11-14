import { Component, OnInit, inject, input } from '@angular/core';
import { FormBuilder, FormGroup, ReactiveFormsModule } from '@angular/forms';
import { NgFor } from '@angular/common';
import { BoardService } from '../../../core/services/board.service';
import { IconComponent } from '../../../shared/ui/icon.component';

interface TierOption {
  code: string;
  label: string;
  hint: string;
}

const TIER_OPTIONS: TierOption[] = [
  { code: '2h', label: 'T-2 jam', hint: '2 jam sebelum budget habis' },
  { code: '1h', label: 'T-1 jam', hint: '1 jam sebelum budget habis' },
  { code: '30m', label: 'T-30 menit', hint: '30 menit sebelum habis' },
  { code: '15m', label: 'T-15 menit', hint: '15 menit sebelum habis' },
  { code: '5m', label: 'T-5 menit', hint: '5 menit sebelum habis' },
  { code: '1m', label: 'T-1 menit', hint: 'Colekan terakhir' },
];

@Component({
  selector: 'app-reminder-preference',
  standalone: true,
  imports: [ReactiveFormsModule, NgFor, IconComponent],
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
    // Any edit makes a previous "saved" note stale.
    this.form.valueChanges.subscribe(() => {
      if (this.message && !this.message.isError) {
        this.message = null;
      }
    });
  }

  protected get selectedCount(): number {
    return TIER_OPTIONS.filter((opt) => this.form.value[opt.code]).length;
  }

  protected setAll(checked: boolean): void {
    this.form.setValue(Object.fromEntries(TIER_OPTIONS.map((opt) => [opt.code, checked])));
  }

  protected onSubmit(): void {
    this.submitting = true;
    const selected = TIER_OPTIONS.filter((opt) => this.form.value[opt.code]).map((opt) => opt.code);

    this.boardService.updatePreference(selected).subscribe({
      next: () => {
        this.submitting = false;
        this.message = { text: 'Preference tersimpan', isError: false };
      },
      error: (err) => {
        this.submitting = false;
        this.message = { text: err?.error?.error ?? 'Gagal menyimpan', isError: true };
      },
    });
  }
}
