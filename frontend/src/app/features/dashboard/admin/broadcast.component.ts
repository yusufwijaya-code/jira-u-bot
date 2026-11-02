import { Component, inject } from '@angular/core';
import { NgIf } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { AdminService } from '../../../core/services/admin.service';

@Component({
  selector: 'app-broadcast',
  standalone: true,
  imports: [NgIf, FormsModule],
  templateUrl: './broadcast.component.html',
})
export class BroadcastComponent {
  protected message = '';
  protected submitting = false;
  protected result: { text: string; isError: boolean } | null = null;

  private adminService = inject(AdminService);

  protected get canSend(): boolean {
    return this.message.trim().length > 0 && !this.submitting;
  }

  protected onSend(): void {
    if (!this.canSend) {
      return;
    }
    if (!confirm('Kirim pengumuman ini ke semua user aktif?')) {
      return;
    }
    this.submitting = true;

    this.adminService.broadcast(this.message).subscribe({
      next: (res) => {
        this.submitting = false;
        this.message = '';
        const failedPart = res.failedEmails.length ? `, gagal ke: ${res.failedEmails.join(', ')}` : '';
        this.result = { text: `Terkirim ke ${res.sentCount} user${failedPart}`, isError: res.failedEmails.length > 0 };
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
