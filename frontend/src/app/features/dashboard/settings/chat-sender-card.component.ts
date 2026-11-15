import { Component, HostListener, Input, OnInit, inject, signal } from '@angular/core';
import { ChatAppService, ChatAppStatus, ChatSender } from '../../../core/services/chat-app.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { ChatAppManualDialogComponent } from '../../../shared/ui/chat-app-manual-dialog.component';

/**
 * Pengaturan › Pengirim pesan (Tech Spec "Nudge sebagai Google Chat App",
 * US-7): Nudge app or the operator account, one at a time. userId null =
 * the logged-in user; a number = Admin › User detail.
 */
@Component({
  selector: 'app-chat-sender-card',
  standalone: true,
  imports: [IconComponent, ChatAppManualDialogComponent],
  template: `
    @if (status(); as s) {
      @if (s.enabled) {
        <section class="card" data-card="chat-sender">
          <header class="card-header">
            <span class="card-icon"><app-icon name="send" [size]="20" /></span>
            <div>
              <h2>Pengirim pesan</h2>
              <p>Dari akun mana notifikasi Nudge dikirim ke Google Chat {{ userId === null ? 'kamu' : 'user ini' }}.</p>
            </div>
          </header>
          <div class="sender-choices" role="radiogroup" aria-label="Pengirim pesan">
            <label class="sender-choice" [class.selected]="s.effectiveSender === 'nudge'" [class.disabled]="nudgeDisabled(s)">
              <input type="radio" name="chat-sender" value="nudge" [checked]="s.effectiveSender === 'nudge'" [disabled]="nudgeDisabled(s)" (click)="pick('nudge', $event)" />
              <span>
                <strong>Nudge (app)</strong>
                <small>Pesan datang dari app Nudge di Google Chat. Mendukung command &amp; tombol aksi.</small>
                @if (s.operatorAccount) {
                  <small>Akun operator tidak bisa menerima DM dari dirinya sendiri.</small>
                }
                @if (!s.activated) {
                  <small>Belum diaktifkan{{ userId === null ? '' : ' oleh user ini' }}.</small>
                }
              </span>
            </label>
            @if (!s.operatorAccount) {
              <label class="sender-choice" [class.selected]="s.effectiveSender === 'operator'">
                <input type="radio" name="chat-sender" value="operator" [checked]="s.effectiveSender === 'operator'" [disabled]="saving()" (click)="pick('operator', $event)" />
                <span>
                  <strong>{{ s.operatorName }}</strong>
                  <small>Pesan datang dari akun {{ s.operatorName }} seperti sebelumnya.</small>
                </span>
              </label>
            }
          </div>
          @if (userId === null && !s.activated && (activateFirst() || s.operatorAccount)) {
            <div class="sender-activate" data-activate-first>
              <p>Aktifkan app Nudge dulu supaya pesan bisa dikirim dari sana.</p>
              @if (s.oneClick) {
                <a class="btn btn-primary btn-sm" [href]="chatApp.activateUrl('sender')">Aktifkan Nudge</a>
              }
              <button type="button" class="btn btn-ghost btn-sm" (click)="manualOpen.set(true)">Cara manual</button>
            </div>
          }
        </section>
        <app-chat-app-manual-dialog [open]="manualOpen()" (closed)="manualOpen.set(false)" />
      }
    }
  `,
})
export class ChatSenderCardComponent implements OnInit {
  @Input() userId: number | null = null;

  protected chatApp = inject(ChatAppService);
  private toast = inject(ToastService);

  protected status = signal<ChatAppStatus | null>(null);
  protected saving = signal(false);
  protected activateFirst = signal(false);
  protected manualOpen = signal(false);

  ngOnInit(): void {
    this.reload();
  }

  /** Coming back from Google Chat (manual install) refreshes the status. */
  @HostListener('window:focus')
  protected onFocus(): void {
    this.reload();
  }

  protected reload(): void {
    this.chatApp.load(this.userId).subscribe({ next: (s) => this.status.set(s), error: () => undefined });
  }

  protected nudgeDisabled(s: ChatAppStatus): boolean {
    return this.saving() || s.operatorAccount || (this.userId !== null && !s.activated);
  }

  protected pick(sender: ChatSender, event: Event): void {
    const s = this.status();
    if (!s || this.saving()) return;
    if (sender === s.effectiveSender) return;
    if (sender === 'nudge' && !s.activated) {
      event.preventDefault();
      this.activateFirst.set(true);
      return;
    }
    event.preventDefault();
    this.saving.set(true);
    this.chatApp.setSender(sender, this.userId).subscribe({
      next: (next) => {
        this.status.set(next);
        this.saving.set(false);
        this.toast.success('Pengirim pesan tersimpan', sender === 'nudge' ? 'Notifikasi dikirim dari app Nudge.' : `Notifikasi dikirim dari akun ${s.operatorName}.`);
      },
      error: (err) => {
        this.saving.set(false);
        this.toast.error('Gagal menyimpan pengirim pesan', err?.error?.error ?? 'Coba lagi.');
      },
    });
  }
}
