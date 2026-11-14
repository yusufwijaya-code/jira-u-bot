import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
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

export interface SettingsResponse {
  notification: NotificationSettings;
  mute: MuteStatus;
  credential: { hasCredential: boolean; maskedPreview: string };
}

/** 400 body of PUT /settings/notification. */
export interface SettingsValidationError {
  error: string;
  fields?: Partial<Record<keyof NotificationSettings, string>>;
}

export type MuteRequest = { minutes: number } | { until: 'tomorrow' | 'forever' };

@Injectable({ providedIn: 'root' })
export class BoardService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/dashboard`;

  getSettings(): Observable<SettingsResponse> {
    return this.http.get<SettingsResponse>(`${this.base}/settings`);
  }

  saveNotificationSettings(settings: NotificationSettings): Observable<NotificationSettings> {
    return this.http.put<NotificationSettings>(`${this.base}/settings/notification`, settings);
  }

  mute(request: MuteRequest): Observable<MuteStatus> {
    return this.http.post<MuteStatus>(`${this.base}/settings/mute`, request);
  }

  unmute(): Observable<MuteStatus> {
    return this.http.delete<MuteStatus>(`${this.base}/settings/mute`);
  }

  saveCredential(jiraEmail: string, apiToken: string): Observable<{ status: string }> {
    return this.http.post<{ status: string }>(`${this.base}/credential`, { jiraEmail, apiToken });
  }

  deleteCredential(): Observable<{ status: string }> {
    return this.http.delete<{ status: string }>(`${this.base}/credential`);
  }
}
