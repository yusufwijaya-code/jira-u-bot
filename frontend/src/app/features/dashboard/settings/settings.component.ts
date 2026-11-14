import { Component, computed, effect, inject, signal, untracked } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { NgTemplateOutlet } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import {
  MuteStatus,
  NotificationSettings,
  SettingsValidationError,
} from '../../../core/services/board.service';
import { MuteStateService } from '../../../core/services/mute-state.service';
import { SETTINGS_MODE, SettingsApi, SettingsScope } from '../../../core/services/settings-api';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { BoardAvatarComponent } from '../../../shared/ui/board-avatar.component';
import { SelectComponent } from '../../../shared/ui/select.component';
import { TimePickerComponent } from '../../../shared/ui/time-picker.component';
import { IconComponent } from '../../../shared/ui/icon.component';
import { JiraCredentialComponent } from './jira-credential.component';
import { MutePanelComponent, activeUntil, formatWib } from './mute-panel.component';
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
 * Personal settings for the board picked in the navbar (Tech Spec "Per-Board
 * Settings & Active Sprint Dashboard"): "Semua board" edits the default for
 * every board; a board either follows that default (read-only until "Atur
 * khusus") or has its own settings ("Kembalikan ke default"). Mute has two
 * levels: global and per board. The Jira API key is per user.
 */
@Component({
  selector: 'app-settings',
  standalone: true,
  imports: [
    FormsModule,
    NgTemplateOutlet,
    IconComponent,
    BoardAvatarComponent,
    SelectComponent,
    TimePickerComponent,
    JiraCredentialComponent,
    MutePanelComponent,
    TierPickerComponent,
  ],
  templateUrl: './settings.component.html',
})
export class SettingsComponent {
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
  /** Scope of the loaded settings (board id, null = default). */
  protected scopeBoardId = signal<number | null>(null);
  protected useDefault = signal(false);
  /** "Atur khusus board ini" was clicked: editable until saved / cancelled. */
  protected editingCustom = signal(false);
  protected boardMute = signal<MuteStatus | null>(null);
  protected globalMute = signal<MuteStatus | null>(null);
  protected resetting = signal(false);
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

  private boardService = inject(SettingsApi);
  private muteState = inject(MuteStateService);
  /** Navbar board picker, or the page-local scope on Admin › User detail. */
  protected boardContext = inject(SettingsScope);
  /** An admin editing another user (Admin › User detail). */
  protected readonly isAdminMode = inject(SETTINGS_MODE) === 'admin';
  private confirm = inject(ConfirmService);
  private toast = inject(ToastService);

  /** The board being configured (null = default for every board). */
  protected scopeBoard = computed(() => {
    const id = this.scopeBoardId();
    return id === null ? null : this.boardContext.boards().find((b) => b.boardId === id) ?? null;
  });
  protected scopeName = computed(() => this.scopeBoard()?.name ?? (this.scopeBoardId() ? `Board #${this.scopeBoardId()}` : ''));
  /** Notification cards are read-only while a board follows the default. */
  protected readOnly = computed(() => this.scopeBoardId() !== null && this.useDefault() && !this.editingCustom());
  protected globalMuteUntil = computed(() => activeUntil(this.globalMute()));

  constructor() {
    // Reload whenever the navbar board changes.
    effect(() => {
      const boardId = this.boardContext.selectedId();
      untracked(() => this.loadSettings(boardId));
    });
  }

  protected loadSettings(boardId: number | null = this.boardContext.selectedId()): void {
    this.loading = true;
    this.error = null;
    this.editingCustom.set(false);
    this.fieldErrors.set({});

    this.boardService.getSettings(boardId).subscribe({
      next: (res) => {
        this.scopeBoardId.set(res.scope === 'board' ? res.boardId : null);
        this.useDefault.set(res.scope === 'board' && res.useDefault);
        this.draft.set({ ...res.notification });
        if (res.notification.quietHours) {
          this.quietStart = res.notification.quietHours.start;
          this.quietEnd = res.notification.quietHours.end;
        }
        this.globalMute.set(res.mute.global);
        this.boardMute.set(res.mute.board);
        // The top bar badge shows the caller's own mute only.
        if (!this.isAdminMode) this.muteState.set(res.mute.global);
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

  protected formatWib(date: Date): string {
    return formatWib(date);
  }

  /** Start customizing a board that follows the default. */
  protected startCustom(): void {
    this.editingCustom.set(true);
  }

  /** Drop unsaved custom edits and go back to showing the default. */
  protected cancelCustom(): void {
    this.loadSettings(this.scopeBoardId());
  }

  protected async resetToDefault(): Promise<void> {
    const boardId = this.scopeBoardId();
    if (boardId === null) return;
    const confirmed = await this.confirm.ask({
      title: 'Kembalikan ke default?',
      message: `Pengaturan khusus ${this.scopeName()} dihapus dan board ini kembali mengikuti pengaturan default semua board. Mute board ini tetap.`,
      confirmText: 'Kembalikan',
      icon: 'refresh',
    });
    if (!confirmed) return;

    this.resetting.set(true);
    this.boardService.resetNotificationSettings(boardId).subscribe({
      next: () => {
        this.resetting.set(false);
        this.toast.success('Kembali ke default', `${this.scopeName()} sekarang mengikuti pengaturan default.`);
        this.boardContext.load();
        this.loadSettings(boardId);
      },
      error: () => {
        this.resetting.set(false);
        this.toast.error('Gagal mengembalikan ke default', 'Coba lagi sebentar.');
      },
    });
  }

  protected save(card: SettingsCard): void {
    const draft = this.draft();
    if (!draft || this.saving() || this.readOnly()) return;
    this.saving.set(card);
    const boardId = this.scopeBoardId();

    this.boardService.saveNotificationSettings(draft, boardId).subscribe({
      next: (saved) => {
        this.saving.set(null);
        this.fieldErrors.set({});
        this.draft.set({ ...saved });
        if (boardId !== null) {
          const becameCustom = this.useDefault();
          this.useDefault.set(false);
          this.editingCustom.set(false);
          if (becameCustom) this.boardContext.load(); // custom dot in the picker
        }
        this.toast.success(
          'Pengaturan tersimpan',
          boardId === null
            ? 'Berlaku untuk semua board yang mengikuti default.'
            : `Berlaku khusus untuk ${this.scopeName()}.`,
        );
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
    this.loadSettings(this.scopeBoardId());
  }
}
