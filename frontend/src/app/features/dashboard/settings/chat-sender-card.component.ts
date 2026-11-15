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
                <small>Pesan datang dari app Nudge di Google Chat. Pesan berupa card dengan tombol (Geser status, Buka menu, dll.).</small>
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
                  <small data-operator-limits>Hanya teks, tanpa tombol: command tetap bisa diketik, mis. /n-mytickets atau /n-tiket CORE-123 review.</small>
                </span>
              </label>
            }
          </div>
          @if (userId === null && !s.activated && s.operatorAccount) {
            <div class="sender-activate" data-activate-operator>
              <p>Aktifkan app Nudge dulu supaya pesan bisa dikirim dari sana.</p>
              @if (s.oneClick) {
                <a class="btn btn-primary btn-sm" [href]="chatApp.activateUrl('sender')">Aktifkan Nudge</a>
              }
              <button type="button" class="btn btn-ghost btn-sm" (click)="manualOpen.set(true)">Cara manual</button>
            </div>
          }
        </section>
        @if (activateFirst() && !s.activated) {
          <div class="modal-backdrop" (click)="activateFirst.set(false)">
            <div
              class="modal"
              role="dialog"
              aria-modal="true"
              aria-labelledby="activate-first-title"
              data-activate-first
              (click)="$event.stopPropagation()"
            >
              <div class="modal-icon"><app-icon name="send" [size]="22" /></div>
              <h3 id="activate-first-title">Aktifkan Nudge dulu</h3>
              <p>Pesan baru bisa dikirim dari app Nudge setelah app-nya diaktifkan di Google Chat kamu.</p>
              <div class="modal-actions">
                @if (s.oneClick) {
                  <button type="button" class="btn btn-ghost" (click)="openManual()">Cara manual</button>
                  <a class="btn btn-primary" [href]="chatApp.activateUrl('sender')">Aktifkan Nudge</a>
                } @else {
                  <button type="button" class="btn btn-ghost" (click)="activateFirst.set(false)">Batal</button>
                  <button type="button" class="btn btn-primary" (click)="openManual()">Cara manual</button>
                }
              </div>
            </div>
          </div>
        }
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

  @HostListener('document:keydown.escape')
  protected onEscape(): void {
    this.activateFirst.set(false);
  }

  /** "Aktifkan Nudge dulu" → the manual steps (one dialog at a time). */
  protected openManual(): void {
    this.activateFirst.set(false);
    this.manualOpen.set(true);
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
