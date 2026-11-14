import { Injectable, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable, tap } from 'rxjs';
import { environment } from '../../../environments/environment';

/** The master switch for automatic notifications (Tech Spec "Notification On-Off Consent"). */
export interface NotificationSwitchState {
  enabled: boolean;
  /** False until the user picked on/off — the shell then asks right after login. */
  chosen: boolean;
}

/**
 * The caller's notification on/off switch, shared by the shell (consent pop
 * up), Pengaturan (switch card) and the Dashboard banner. Seeded from
 * GET /settings by whoever loads it first.
 */
@Injectable({ providedIn: 'root' })
export class NotificationSwitchService {
  private http = inject(HttpClient);
  private url = `${environment.apiBasePath}/api/dashboard/settings/notifications`;

  /** null until GET /settings answered. */
  readonly state = signal<NotificationSwitchState | null>(null);

  set(state: NotificationSwitchState | null | undefined): void {
    if (state) this.state.set({ ...state });
  }

  save(enabled: boolean): Observable<NotificationSwitchState> {
    return this.http.put<NotificationSwitchState>(this.url, { enabled }).pipe(tap((res) => this.state.set(res)));
  }
}
