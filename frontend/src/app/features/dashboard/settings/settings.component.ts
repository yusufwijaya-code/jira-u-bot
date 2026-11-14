import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { NgTemplateOutlet } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import {
  BoardService,
  NotificationSettings,
  SettingsValidationError,
} from '../../../core/services/board.service';
import { MuteStateService } from '../../../core/services/mute-state.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { JiraCredentialComponent } from './jira-credential.component';
import { MutePanelComponent } from './mute-panel.component';
import { TierPickerComponent } from './tier-picker.component';
import { sortTiers, tierDuration } from './tier-codes';

/** Which card's Simpan button is running — every card saves the whole draft. */
export type SettingsCard = 'reminder' | 'overage' | 'events' | 'quiet';

const CARD_FIELDS: Record<SettingsCard, (keyof NotificationSettings)[]> = {
  reminder: ['reminderTiers'],
  overage: ['overageTiers', 'overageRepeatMinutes'],
  events: ['notifyEarlyFinish', 'notifyReturnedToTodo'],
  quiet: ['quietHours'],
};

/**
 * Personal settings (Tech Spec "Dynamic User Notification Settings"):
 * pre-budget tiers, post-budget tiers + repeat, event toggles, mute & quiet
 * hours, and the personal Jira API key.
 */
@Component({
  selector: 'app-settings',
  standalone: true,
  imports: [FormsModule, NgTemplateOutlet, IconComponent, JiraCredentialComponent, MutePanelComponent, TierPickerComponent],
  templateUrl: './settings.component.html',
})
export class SettingsComponent implements OnInit {
  protected readonly reminderPresets = ['2h', '1h', '30m', '15m', '5m', '1m'];
  protected readonly overagePresets = ['5m', '15m', '30m', '1h', '2h'];
  protected readonly repeatOptions: { value: number | null; label: string }[] = [
    { value: 5, label: 'Ulang tiap 5 menit' },
    { value: 10, label: 'Ulang tiap 10 menit' },
    { value: 15, label: 'Ulang tiap 15 menit' },
    { value: 30, label: 'Ulang tiap 30 menit' },
    { value: 60, label: 'Ulang tiap 60 menit' },
    { value: null, label: 'Berhenti' },
  ];

  protected loading = true;
  protected error: string | null = null;
  protected hasCredential = false;
  protected credentialMasked: string | null = null;

  protected draft = signal<NotificationSettings | null>(null);
  protected saving = signal<SettingsCard | null>(null);
  protected fieldErrors = signal<Partial<Record<keyof NotificationSettings, string>>>({});
  /** Values for the quiet hours inputs, kept while the toggle is off. */
  protected quietStart = '12:00';
  protected quietEnd = '13:00';

  /** "Budget habis → +15 menit → +1 jam → tiap 30 menit". */
  protected overagePreview = computed(() => {
    const d = this.draft();
    if (!d) return '';
    const steps = ['Budget habis', ...sortTiers(d.overageTiers, 'asc').map((code) => `+${tierDuration(code)}`)];
    steps.push(d.overageRepeatMinutes === null ? 'berhenti' : `tiap ${d.overageRepeatMinutes} menit`);
    return steps.join(' → ');
  });

  private boardService = inject(BoardService);
  private muteState = inject(MuteStateService);
  private toast = inject(ToastService);

  ngOnInit(): void {
    this.loadSettings();
  }

  protected loadSettings(): void {
    this.loading = true;
    this.error = null;

    this.boardService.getSettings().subscribe({
      next: (res) => {
        this.draft.set({ ...res.notification });
        if (res.notification.quietHours) {
          this.quietStart = res.notification.quietHours.start;
          this.quietEnd = res.notification.quietHours.end;
        }
        this.muteState.set(res.mute);
        this.hasCredential = res.credential.hasCredential;
        this.credentialMasked = res.credential.maskedPreview;
        this.loading = false;
      },
      error: () => {
        this.error = 'Gagal memuat pengaturan, coba lagi';
        this.loading = false;
      },
    });
  }

  protected patch(changes: Partial<NotificationSettings>): void {
    const current = this.draft();
    if (!current) return;
    this.draft.set({ ...current, ...changes });
    // Editing a field clears its stale server error.
    const errors = { ...this.fieldErrors() };
    for (const key of Object.keys(changes) as (keyof NotificationSettings)[]) {
      delete errors[key];
    }
    this.fieldErrors.set(errors);
  }

  protected setQuietEnabled(enabled: boolean): void {
    this.patch({ quietHours: enabled ? { start: this.quietStart, end: this.quietEnd } : null });
  }

  protected setQuietTime(which: 'start' | 'end', value: string): void {
    if (which === 'start') this.quietStart = value;
    else this.quietEnd = value;
    if (this.draft()?.quietHours) {
      this.patch({ quietHours: { start: this.quietStart, end: this.quietEnd } });
    }
  }

  protected errorFor(card: SettingsCard): string | null {
    const errors = this.fieldErrors();
    for (const field of CARD_FIELDS[card]) {
      if (errors[field]) return errors[field]!;
    }
    return null;
  }

  protected save(card: SettingsCard): void {
    const draft = this.draft();
    if (!draft || this.saving()) return;
    this.saving.set(card);

    this.boardService.saveNotificationSettings(draft).subscribe({
      next: (saved) => {
        this.saving.set(null);
        this.fieldErrors.set({});
        this.draft.set({ ...saved });
        this.toast.success('Pengaturan tersimpan', 'Perubahan langsung berlaku untuk notifikasi berikutnya.');
      },
      error: (err: HttpErrorResponse) => {
        this.saving.set(null);
        const body = err?.error as SettingsValidationError | undefined;
        if (err?.status === 400 && body?.fields) {
          this.fieldErrors.set(body.fields);
          return;
        }
        this.toast.error('Gagal menyimpan pengaturan', body?.error ?? 'Coba lagi sebentar.');
      },
    });
  }

  protected onCredentialChanged(): void {
    this.loadSettings();
  }
}
