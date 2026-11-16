import { Component, HostListener, OnInit, computed, inject, signal } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { CHAT_APP_RESULTS, ChatAppService } from '../../core/services/chat-app.service';
import { ToastService } from '../../core/ui/toast.service';
import { IconComponent } from './icon.component';
import { GoogleUnverifiedNoteComponent } from './google-unverified-note.component';
import { ChatAppManualDialogComponent } from './chat-app-manual-dialog.component';

const SNOOZE_KEY = 'nudge.chatAppBanner.snoozedUntil';
const SNOOZE_MS = 7 * 24 * 60 * 60 * 1000;

function readSnooze(): number {
  try {
    return Number(localStorage.getItem(SNOOZE_KEY) ?? 0) || 0;
  } catch {
    return 0;
  }
}

/**
 * Invites the user to activate the Nudge Chat App (Tech Spec "Nudge sebagai
 * Google Chat App", AC3.5) and turns the activation callback's ?chatApp=
 * into a toast on any dashboard page.
 */
@Component({
  selector: 'app-chat-app-banner',
  standalone: true,
  imports: [IconComponent, GoogleUnverifiedNoteComponent, ChatAppManualDialogComponent],
  template: `
    @if (visible(); as s) {
      <section class="chat-app-banner" [class.required]="s.operatorAccount" role="status">
        <app-icon name="zap" [size]="20" />
        <p>
          @if (s.groupState === 'pending') {
            <span data-group-pending>App Nudge sedang disiapkan untuk akunmu (maks. 24 jam).{{ s.operatorAccount ? '' : ' Sementara itu notifikasi tetap dikirim lewat DM.' }}</span>
          } @else if (s.groupState === 'failed') {
            <span data-group-failed>Akunmu belum bisa ditambahkan ke app Nudge. Hubungi admin Nudge.</span>
          } @else if (s.operatorAccount) {
            Akunmu hanya bisa menerima notifikasi lewat app <strong>Nudge</strong>. Aktifkan sekarang.
          } @else {
            Notifikasi bisa datang langsung dari app <strong>Nudge</strong> di Google Chat, bukan dari akun {{ s.operatorName }}.
          }
        </p>
        <div class="chat-app-banner-actions">
          @if (s.oneClick) {
            <a class="btn btn-primary btn-sm" data-activate [href]="chatApp.activateUrl()">Aktifkan Nudge di Google Chat</a>
          } @else {
            <a class="btn btn-primary btn-sm" data-open-chat href="https://mail.google.com/chat/" target="_blank" rel="noopener">Buka Google Chat</a>
          }
          <button type="button" class="btn btn-ghost btn-sm" data-manual (click)="manualOpen.set(true)">Cara manual</button>
          @if (!s.operatorAccount) {
            <button type="button" class="btn btn-ghost btn-sm" data-snooze (click)="snooze()">Nanti saja</button>
          }
        </div>
        @if (s.oneClick) {
          <app-google-unverified-note />
        }
      </section>
    }
    <app-chat-app-manual-dialog [open]="manualOpen()" (closed)="manualOpen.set(false)" />
  `,
})
export class ChatAppBannerComponent implements OnInit {
  protected chatApp = inject(ChatAppService);
  private toast = inject(ToastService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);

  protected manualOpen = signal(false);
  private snoozedUntil = signal(readSnooze());

  protected visible = computed(() => {
    const s = this.chatApp.status();
    if (!s || !s.enabled || s.activated) return null;
    if (s.operatorAccount) return s;
    if (s.sender !== null || this.snoozedUntil() > Date.now()) return null;
    return s;
  });

  ngOnInit(): void {
    this.chatApp.refresh();
    this.route.queryParamMap.subscribe((params) => {
      const code = params.get('chatApp');
      if (!code) return;
      const result = CHAT_APP_RESULTS[code] ?? CHAT_APP_RESULTS['failed'];
      this.toast.show(result.tone, result.title, result.message);
      this.router.navigate([], { queryParams: { chatApp: null }, queryParamsHandling: 'merge', replaceUrl: true });
      this.chatApp.refresh();
    });
  }

  /** Coming back from Google Chat (install there) refreshes the status. */
  @HostListener('window:focus')
  protected onFocus(): void {
    if (this.chatApp.status() && !this.chatApp.status()!.activated) this.chatApp.refresh();
  }

  protected snooze(): void {
    const until = Date.now() + SNOOZE_MS;
    this.snoozedUntil.set(until);
    try {
      localStorage.setItem(SNOOZE_KEY, String(until));
    } catch {
      /* private mode: snooze for this page only */
    }
  }
}
