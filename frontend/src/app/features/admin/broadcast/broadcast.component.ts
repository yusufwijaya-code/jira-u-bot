import { Component, inject } from '@angular/core';
import { NgIf } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { AdminService } from '../../../core/services/admin.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';

@Component({
  selector: 'app-broadcast',
  standalone: true,
  imports: [NgIf, FormsModule, IconComponent],
  templateUrl: './broadcast.component.html',
})
export class BroadcastComponent {
  protected message = '';
  protected submitting = false;
  protected result: { text: string; isError: boolean } | null = null;

  private adminService = inject(AdminService);
  private confirm = inject(ConfirmService);
  private toast = inject(ToastService);

  protected get canSend(): boolean {
    return this.message.trim().length > 0 && !this.submitting;
  }

  protected async onSend(): Promise<void> {
    if (!this.canSend) {
      return;
    }
    const confirmed = await this.confirm.ask({
      title: 'Kirim broadcast?',
      message: 'Pengumuman ini akan dikirim lewat DM Google Chat ke semua user aktif.',
      confirmText: 'Kirim sekarang',
      icon: 'send',
    });
    if (!confirmed) {
      return;
    }
    this.submitting = true;

    this.adminService.broadcast(this.message).subscribe({
      next: (res) => {
        this.submitting = false;
        this.message = '';
        const failedPart = res.failedEmails.length ? `, gagal ke: ${res.failedEmails.join(', ')}` : '';
        this.result = { text: `Terkirim ke ${res.sentCount} user${failedPart}`, isError: res.failedEmails.length > 0 };
        if (!res.failedEmails.length) {
          this.toast.success('Broadcast terkirim', `Terkirim ke ${res.sentCount} user`);
        }
      },
      error: (err) => {
        this.submitting = false;
        this.result = {
          text:
            err?.status === 503
              ? 'personal DM (OAuth) belum pernah di-setup di server ini, broadcast tidak tersedia'
              : err?.status === 400
                ? 'pesan tidak boleh kosong'
                : 'gagal mengirim broadcast, coba lagi',
          isError: true,
        };
      },
    });
  }
}
