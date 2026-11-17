import { Component, computed, inject, input, output, signal } from '@angular/core';
import { MuteRequest, MuteStatus } from '../../../core/services/board.service';
import { MuteStateService } from '../../../core/services/mute-state.service';
import { SETTINGS_MODE, SettingsApi, SettingsScope } from '../../../core/services/settings-api';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';

interface MuteOption {
  label: string;
  request: MuteRequest;
}

/** "Sen, 17 Nov 08.30" in WIB, regardless of the browser's time zone. */
export function formatWib(date: Date): string {
  return date.toLocaleString('id-ID', {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    timeZone: 'Asia/Jakarta',
  });
}

/** The running mute's end, or null when not muted (expired counts as not muted). */
export function activeUntil(status: MuteStatus | null | undefined): Date | null {
  if (!status?.active || !status.until) return null;
  const until = new Date(status.until);
  return until.getTime() > Date.now() ? until : null;
}

/**
 * Instant mute actions — no save button. `boardId` null = the global mute
 * (shared with the top bar badge); a board id = that board's own mute
 * (Tech Spec "Per-Board Settings & Active Sprint Dashboard", AC4.1–AC4.2).
 */
@Component({
  selector: 'app-mute-panel',
  standalone: true,
  imports: [IconComponent],
  template: `
    @if (until(); as until) {
      <div class="mute-banner" [class.forever]="forever()">
        <span class="mute-banner-icon"><app-icon name="bell-off" [size]="20" /></span>
        <div>
          @if (forever()) {
            <strong>{{ isBoard() ? 'Notifikasi board ini dimatikan' : 'Notifikasi dimatikan' }}</strong>
            <span>Tidak ada notifikasi{{ isBoard() ? ' dari board ini' : '' }} sampai kamu aktifkan lagi.</span>
          } @else {
            <strong>{{ isBoard() ? 'Board ini sedang dimute' : 'Notifikasi sedang dimute' }}</strong>
            <span>Sampai {{ formatUntil(until) }} WIB</span>
          }
        </div>
        <button type="button" class="btn btn-ghost btn-sm" [class.loading]="busy()" [disabled]="busy()" (click)="unmute()">
          <span class="spinner" aria-hidden="true"></span>
          <span class="btn-label">{{ forever() ? 'Aktifkan lagi' : 'Batalkan' }}</span>
        </button>
      </div>
    } @else {
      <div class="mute-actions">
        @for (option of options; track option.label) {
          <button type="button" class="btn btn-sm btn-ghost" [disabled]="busy()" (click)="mute(option)">
            <app-icon name="bell-off" [size]="14" /> {{ option.label }}
          </button>
        }
      </div>
    }
  `,
})
export class MutePanelComponent {
  /** null = global mute; otherwise this board's mute. */
  boardId = input<number | null>(null);
  /** Board scope: the board's mute as loaded by the page. */
  boardStatus = input<MuteStatus | null>(null);
  /**
   * Admin mode, global level: the target user's global mute as loaded by
   * the page (self mode reads the shared MuteStateService instead).
   */
  globalStatus = input<MuteStatus | null>(null);
  statusChange = output<MuteStatus>();

  private muteState = inject(MuteStateService);
  private scope = inject(SettingsScope);
  private api = inject(SettingsApi);
  private mode = inject(SETTINGS_MODE);
  private toast = inject(ToastService);

  protected busy = signal(false);
  /** The status after the last action here (overrides the input). */
  private changed = signal<{ boardId: number | null; status: MuteStatus } | null>(null);

  protected isBoard = computed(() => this.boardId() !== null);
  /** The caller's own global mute, shared with the top bar badge. */
  private selfGlobal = computed(() => !this.isBoard() && this.mode === 'self');
  private status = computed<MuteStatus | null>(() => {
    const last = this.changed();
    if (last && last.boardId === this.boardId()) return last.status;
    return this.isBoard() ? this.boardStatus() : this.globalStatus();
  });
  protected until = computed<Date | null>(() => (this.selfGlobal() ? this.muteState.until() : activeUntil(this.status())));
  protected forever = computed(() => {
    if (this.selfGlobal()) return this.muteState.forever();
    return this.until() !== null && !!this.status()?.forever;
  });

  /**
   * Timed mutes only: "Selamanya" is no longer offered here. A forever mute
   * set earlier (or via the chat command) still shows its banner and can be
   * switched back on with "Aktifkan lagi".
   */
  protected readonly options: MuteOption[] = [
    { label: '1 jam', request: { minutes: 60 } },
    { label: '2 jam', request: { minutes: 120 } },
    { label: 'Sampai besok', request: { until: 'tomorrow' } },
  ];

  protected formatUntil(until: Date): string {
    return formatWib(until);
  }

  protected mute(option: MuteOption): void {
    this.busy.set(true);
    this.api.mute(option.request, this.boardId()).subscribe({
      next: (status) => {
        this.busy.set(false);
        this.apply(status);
        this.toast.success(this.isBoard() ? 'Board dimute' : 'Notifikasi dimute', `Mute ${option.label.toLowerCase()} aktif.`);
      },
      error: () => {
        this.busy.set(false);
        this.toast.error('Gagal mute notifikasi', 'Coba lagi sebentar.');
      },
    });
  }

  protected unmute(): void {
    this.busy.set(true);
    this.api.unmute(this.boardId()).subscribe({
      next: (status) => {
        this.busy.set(false);
        this.apply(status);
        this.toast.success('Notifikasi aktif kembali', 'Reminder akan dikirim seperti biasa.');
      },
      error: () => {
        this.busy.set(false);
        this.toast.error('Gagal membatalkan mute', 'Coba lagi sebentar.');
      },
    });
  }

  private apply(status: MuteStatus): void {
    if (this.selfGlobal()) {
      this.muteState.set(status);
    } else {
      this.changed.set({ boardId: this.boardId(), status });
      if (this.isBoard()) this.scope.load(); // refresh the picker's muted dot
    }
    this.statusChange.emit(status);
  }
}
