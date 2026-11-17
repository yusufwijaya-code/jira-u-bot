import { Component, ElementRef, OnInit, ViewChild, inject } from '@angular/core';
import { NgIf } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { AdminService } from '../../../core/services/admin.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';

/** Max broadcast length (same as the textarea's maxlength). */
export const BROADCAST_MAX = 2000;

/**
 * Renders the basic Google Chat markup (*bold*, _italic_, ~strike~, `code`)
 * and line breaks as HTML for the live preview. Input is escaped first.
 */
export function chatPreviewHtml(text: string): string {
  const escaped = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
  return escaped
    .replace(/`([^`\n]+)`/g, '<code>$1</code>')
    .replace(/(^|[^\w*])\*([^*\n]+)\*(?=[^\w*]|$)/g, '$1<strong>$2</strong>')
    .replace(/(^|[^\w_])_([^_\n]+)_(?=[^\w_]|$)/g, '$1<em>$2</em>')
    .replace(/(^|[^\w~])~([^~\n]+)~(?=[^\w~]|$)/g, '$1<s>$2</s>')
    .replace(/\n/g, '<br>');
}

@Component({
  selector: 'app-broadcast',
  standalone: true,
  imports: [NgIf, FormsModule, IconComponent],
  templateUrl: './broadcast.component.html',
  styleUrl: './broadcast.component.scss',
})
export class BroadcastComponent implements OnInit {
  protected readonly max = BROADCAST_MAX;
  protected message = '';
  protected submitting = false;
  protected result: { text: string; isError: boolean } | null = null;
  /** Recipient summary from the user list; null while loading, 'error' when unavailable. */
  protected recipients: { active: number; activated: number } | null | 'error' = null;

  @ViewChild('editor') private editor?: ElementRef<HTMLTextAreaElement>;

  private adminService = inject(AdminService);
  private confirm = inject(ConfirmService);
  private toast = inject(ToastService);

  ngOnInit(): void {
    this.adminService.listUsers().subscribe({
      next: (res) => {
        const active = (res.users ?? []).filter((u) => u.flagActive);
        this.recipients = { active: active.length, activated: active.filter((u) => u.chatAppActivated).length };
      },
      error: () => (this.recipients = 'error'),
    });
  }

  protected get canSend(): boolean {
    return this.message.trim().length > 0 && !this.submitting;
  }

  protected get usage(): number {
    return Math.min(1, this.message.length / this.max);
  }

  protected get usageLevel(): 'ok' | 'warn' | 'danger' {
    return this.usage >= 0.95 ? 'danger' : this.usage >= 0.8 ? 'warn' : 'ok';
  }

  protected get previewHtml(): string {
    return chatPreviewHtml(this.message);
  }

  protected get recipientSummary(): { active: number; activated: number } | null {
    return this.recipients && this.recipients !== 'error' ? this.recipients : null;
  }

  /** Grows the textarea with its content (up to the CSS max-height). */
  protected autoGrow(): void {
    const el = this.editor?.nativeElement;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${el.scrollHeight + 2}px`;
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
    this.result = null;

    this.adminService.broadcast(this.message).subscribe({
      next: (res) => {
        this.submitting = false;
        this.message = '';
        setTimeout(() => this.autoGrow());
        const failed = res.failedEmails ?? [];
        const failedPart = failed.length ? `, gagal ke: ${failed.join(', ')}` : '';
        const skipped = res.skippedEmails ?? [];
        const skippedPart = skipped.length ? `, dilewati (belum bisa dihubungi): ${skipped.join(', ')}` : '';
        this.result = { text: `Terkirim ke ${res.sentCount} user${failedPart}${skippedPart}`, isError: failed.length > 0 };
        if (!failed.length) {
          this.toast.success('Broadcast terkirim', `Terkirim ke ${res.sentCount} user`);
        } else {
          this.toast.error('Sebagian broadcast gagal', `Gagal ke ${failed.length} user`);
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
