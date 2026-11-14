import { Component, inject, signal } from '@angular/core';
import { BoardService, MuteRequest } from '../../../core/services/board.service';
import { MuteStateService } from '../../../core/services/mute-state.service';
import { ToastService } from '../../../core/ui/toast.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { IconComponent } from '../../../shared/ui/icon.component';

interface MuteOption {
  label: string;
  request: MuteRequest;
  forever?: boolean;
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

/** Instant mute actions — no save button (Tech Spec AC4.4). */
@Component({
  selector: 'app-mute-panel',
  standalone: true,
  imports: [IconComponent],
  template: `
    @if (muteState.until(); as until) {
      <div class="mute-banner" [class.forever]="muteState.forever()">
        <span class="mute-banner-icon"><app-icon name="bell-off" [size]="20" /></span>
        <div>
          @if (muteState.forever()) {
            <strong>Notifikasi dimatikan</strong>
            <span>Tidak ada notifikasi sampai kamu aktifkan lagi.</span>
          } @else {
            <strong>Notifikasi sedang dimute</strong>
            <span>Sampai {{ formatUntil(until) }} WIB</span>
          }
        </div>
        <button type="button" class="btn btn-ghost btn-sm" [class.loading]="busy()" [disabled]="busy()" (click)="unmute()">
          <span class="spinner" aria-hidden="true"></span>
          <span class="btn-label">{{ muteState.forever() ? 'Aktifkan lagi' : 'Batalkan' }}</span>
        </button>
      </div>
    } @else {
      <div class="mute-actions">
        @for (option of options; track option.label) {
          <button
            type="button"
            class="btn btn-sm"
            [class.btn-ghost]="!option.forever"
            [class.btn-danger-ghost]="option.forever"
            [disabled]="busy()"
            (click)="mute(option)"
          >
            <app-icon [name]="option.forever ? 'power' : 'bell-off'" [size]="14" /> {{ option.label }}
          </button>
        }
      </div>
    }
  `,
})
export class MutePanelComponent {
  protected muteState = inject(MuteStateService);
  private boardService = inject(BoardService);
  private toast = inject(ToastService);
  private confirm = inject(ConfirmService);

  protected busy = signal(false);
  protected readonly options: MuteOption[] = [
    { label: '1 jam', request: { minutes: 60 } },
    { label: '2 jam', request: { minutes: 120 } },
    { label: 'Sampai besok', request: { until: 'tomorrow' } },
    { label: 'Selamanya', request: { until: 'forever' }, forever: true },
  ];

  protected formatUntil(until: Date): string {
    return formatWib(until);
  }

  protected async mute(option: MuteOption): Promise<void> {
    if (option.forever) {
      const confirmed = await this.confirm.ask({
        title: 'Matikan semua notifikasi?',
        message: 'Nudge tidak akan mengirim reminder apa pun sampai kamu mengaktifkannya lagi dari halaman ini.',
        confirmText: 'Matikan',
        tone: 'danger',
        icon: 'bell-off',
      });
      if (!confirmed) return;
    }
    this.busy.set(true);
    this.boardService.mute(option.request).subscribe({
      next: (status) => {
        this.busy.set(false);
        this.muteState.set(status);
        if (option.forever) {
          this.toast.success('Notifikasi dimatikan', 'Aktifkan lagi kapan saja dari Pengaturan.');
        } else {
          this.toast.success('Notifikasi dimute', `Mute ${option.label.toLowerCase()} aktif.`);
        }
      },
      error: () => {
        this.busy.set(false);
        this.toast.error('Gagal mute notifikasi', 'Coba lagi sebentar.');
      },
    });
  }

  protected unmute(): void {
    this.busy.set(true);
    this.boardService.unmute().subscribe({
      next: (status) => {
        this.busy.set(false);
        this.muteState.set(status);
        this.toast.success('Notifikasi aktif kembali', 'Reminder akan dikirim seperti biasa.');
      },
      error: () => {
        this.busy.set(false);
        this.toast.error('Gagal membatalkan mute', 'Coba lagi sebentar.');
      },
    });
  }
}
