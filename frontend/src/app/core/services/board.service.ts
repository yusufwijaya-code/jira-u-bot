import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

export interface QuietHours {
  start: string; // "HH:MM" WIB
  end: string;
}

/** Per-user notification settings (Tech Spec "Dynamic User Notification Settings"). */
export interface NotificationSettings {
  /** Pre-budget tiers, e.g. ["2h", "45m"] — largest first. */
  reminderTiers: string[];
  /** Post-budget tiers (offset after the budget ran out), e.g. ["15m", "1h"]. */
  overageTiers: string[];
  /** Repeat every N minutes after the last overage tier; null = stop. */
  overageRepeatMinutes: number | null;
  notifyEarlyFinish: boolean;
  notifyReturnedToTodo: boolean;
  quietHours: QuietHours | null;
}

export interface MuteStatus {
  active: boolean;
  /** ISO timestamp, null when not muted. */
  until: string | null;
  /** Muted until the user turns notifications back on ("Selamanya"). */
  forever?: boolean;
}

/**
 * GET /settings?board= (Tech Spec "Per-Board Settings & Active Sprint
 * Dashboard"): scope "default" = every board, "board" = one board.
 * `notification` is always the EFFECTIVE value for the scope.
 */
export interface SettingsResponse {
  scope: 'default' | 'board';
  boardId: number;
  /** Board scope only: the board follows the default settings. */
  useDefault: boolean;
  notification: NotificationSettings;
  mute: { global: MuteStatus; board: MuteStatus | null };
  credential: { hasCredential: boolean; maskedPreview: string };
  /** Master on/off switch (Tech Spec "Notification On-Off Consent"). */
  notifications?: { enabled: boolean; chosen: boolean };
}

/** 400 body of PUT /settings/notification. */
export interface SettingsValidationError {
  error: string;
  fields?: Partial<Record<keyof NotificationSettings, string>>;
}

export type MuteRequest = { minutes: number } | { until: 'tomorrow' | 'forever' };

/** `?board=` for a board scope; nothing for the default / all-boards scope. */
export function boardParams(boardId: number | null | undefined): HttpParams {
  return boardId ? new HttpParams().set('board', String(boardId)) : new HttpParams();
}

@Injectable({ providedIn: 'root' })
export class BoardService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/dashboard`;

  getSettings(boardId: number | null = null): Observable<SettingsResponse> {
    return this.http.get<SettingsResponse>(`${this.base}/settings`, { params: boardParams(boardId) });
  }

  saveNotificationSettings(settings: NotificationSettings, boardId: number | null = null): Observable<NotificationSettings> {
    return this.http.put<NotificationSettings>(`${this.base}/settings/notification`, settings, { params: boardParams(boardId) });
  }

  /** Board only: follow the default settings again. */
  resetNotificationSettings(boardId: number): Observable<NotificationSettings> {
    return this.http.delete<NotificationSettings>(`${this.base}/settings/notification`, { params: boardParams(boardId) });
  }

  /** Without a board = global mute. */
  mute(request: MuteRequest, boardId: number | null = null): Observable<MuteStatus> {
    return this.http.post<MuteStatus>(`${this.base}/settings/mute`, request, { params: boardParams(boardId) });
  }

  unmute(boardId: number | null = null): Observable<MuteStatus> {
    return this.http.delete<MuteStatus>(`${this.base}/settings/mute`, { params: boardParams(boardId) });
  }

  saveCredential(jiraEmail: string, apiToken: string): Observable<{ status: string }> {
    return this.http.post<{ status: string }>(`${this.base}/credential`, { jiraEmail, apiToken });
  }

  deleteCredential(): Observable<{ status: string }> {
    return this.http.delete<{ status: string }>(`${this.base}/credential`);
  }
}
