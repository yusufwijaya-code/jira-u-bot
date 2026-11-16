import { Injectable, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable, tap } from 'rxjs';
import { environment } from '../../../environments/environment';
import { ToastTone } from '../ui/toast.service';

export type ChatSender = 'nudge' | 'operator';

/** GET /api/dashboard/chat-app/status (Tech Spec "Nudge sebagai Google Chat App"). */
export interface ChatAppStatus {
  enabled: boolean;
  activated: boolean;
  activatedDate: string | null;
  oneClick: boolean;
  sender: ChatSender | null;
  effectiveSender: ChatSender;
  operatorAccount: boolean;
  operatorName: string;
  /** Visibility through the Nudge Google Group (Tech Spec "Auto Join Google Group Nudge"). */
  groupState: 'none' | 'pending' | 'ready' | 'failed';
}

/** Toasts for the ?chatApp=<code> the activation callback redirects with. */
export const CHAT_APP_RESULTS: Record<string, { tone: ToastTone; title: string; message: string }> = {
  activated: { tone: 'success', title: 'Nudge App aktif', message: 'Notifikasi bisa datang langsung dari app Nudge.' },
  cancelled: { tone: 'info', title: 'Aktivasi dibatalkan', message: 'Kamu bisa mengaktifkan lewat cara manual kapan saja.' },
  blocked: { tone: 'error', title: 'Diblokir kebijakan ESB', message: 'Gunakan cara manual: cari "Nudge" di Google Chat lalu Install.' },
  not_allowed: { tone: 'error', title: 'Akunmu belum diizinkan', message: 'Hubungi admin Nudge supaya emailmu ditambahkan ke app Nudge.' },
  mismatch: { tone: 'error', title: 'Akun Google berbeda', message: 'Pilih akun Google yang sama dengan akun login dashboard.' },
  failed: { tone: 'error', title: 'Aktivasi gagal', message: 'Coba lagi, atau gunakan cara manual.' },
};

/** The caller's (or, for admins, another user's) Nudge App status and sender choice. */
@Injectable({ providedIn: 'root' })
export class ChatAppService {
  private http = inject(HttpClient);

  /** The logged-in user's status; null until loaded. */
  readonly status = signal<ChatAppStatus | null>(null);

  private base(userId: number | null): string {
    return userId === null
      ? `${environment.apiBasePath}/api/dashboard`
      : `${environment.apiBasePath}/api/dashboard/admin/users/${userId}`;
  }

  load(userId: number | null = null): Observable<ChatAppStatus> {
    return this.http
      .get<ChatAppStatus>(`${this.base(userId)}/chat-app/status`)
      .pipe(tap((s) => userId === null && this.status.set(s)));
  }

  refresh(): void {
    this.load().subscribe({ error: () => undefined });
  }

  setSender(sender: ChatSender, userId: number | null = null): Observable<ChatAppStatus> {
    return this.http
      .put<ChatAppStatus>(`${this.base(userId)}/settings/chat-sender`, { sender })
      .pipe(tap((s) => userId === null && this.status.set(s)));
  }

  activateUrl(then?: 'sender'): string {
    return `${environment.apiBasePath}/auth/chat-app/activate${then ? `?then=${then}` : ''}`;
  }
}
