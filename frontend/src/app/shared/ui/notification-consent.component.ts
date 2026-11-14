import { Component, computed, inject, signal } from '@angular/core';
import { NotificationSwitchService } from '../../core/services/notification-switch.service';
import { ToastService } from '../../core/ui/toast.service';
import { IconComponent } from './icon.component';

/**
 * Asks once, right after login, whether Nudge may send automatic
 * notifications (Tech Spec "Notification On-Off Consent"). The switch starts
 * OFF and the dialog only closes by saving a choice — no ×, backdrop click
 * or Esc.
 */
@Component({
  selector: 'app-notification-consent',
  standalone: true,
  imports: [IconComponent],
  template: `
    @if (open()) {
      <div class="modal-backdrop consent-backdrop">
        <div class="modal consent-modal" role="dialog" aria-modal="true" aria-labelledby="consent-title" aria-describedby="consent-desc">
          <div class="modal-icon"><app-icon name="bell" [size]="22" /></div>
          <h3 id="consent-title">Aktifkan notifikasi Nudge?</h3>
          <p id="consent-desc">
            Nudge bisa mengirim DM Google Chat saat budget waktu tiket kamu hampir habis, sudah lewat, selesai lebih
            awal, atau tiket balik ke To Do. Kamu bisa mengubahnya kapan saja di Pengaturan.
          </p>

          <label class="toggle-row consent-toggle">
            <span class="toggle-text">
              <strong>{{ enabled() ? 'Notifikasi aktif' : 'Notifikasi mati' }}</strong>
              <small>{{ enabled() ? 'Reminder otomatis akan dikirim ke DM kamu.' : 'Tidak ada reminder otomatis yang dikirim.' }}</small>
            </span>
            <input
              type="checkbox"
              class="switch"
              data-consent-switch
              [checked]="enabled()"
              (change)="enabled.set($any($event.target).checked)"
              aria-label="Notifikasi otomatis"
            />
          </label>

          @if (error()) {
            <p class="msg error consent-error" role="alert">{{ error() }}</p>
          }

          <button type="button" class="btn btn-primary btn-block" data-consent-save [class.loading]="saving()" [disabled]="saving()" (click)="save()">
            <span class="spinner" aria-hidden="true"></span>
            <span class="btn-label">Simpan pilihan</span>
          </button>
        </div>
      </div>
    }
  `,
})
export class NotificationConsentComponent {
  private notifSwitch = inject(NotificationSwitchService);
  private toast = inject(ToastService);

  /** Starts OFF for everyone, including existing users (spec AC1.3). */
  protected enabled = signal(false);
  protected saving = signal(false);
  protected error = signal<string | null>(null);

  protected open = computed(() => {
    const state = this.notifSwitch.state();
    return !!state && !state.chosen;
  });

  protected save(): void {
    if (this.saving()) return;
    this.saving.set(true);
    this.error.set(null);
    const enabled = this.enabled();
    this.notifSwitch.save(enabled).subscribe({
      next: () => {
        this.saving.set(false);
        this.toast.success(
          enabled ? 'Notifikasi aktif' : 'Notifikasi dimatikan',
          enabled ? 'Nudge akan mengingatkan kamu lewat DM.' : 'Kamu bisa menyalakannya kapan saja di Pengaturan.',
        );
      },
      error: () => {
        this.saving.set(false);
        this.error.set('Gagal menyimpan pilihan, coba lagi.');
      },
    });
  }
}
