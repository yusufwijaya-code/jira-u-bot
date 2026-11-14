import { Injectable, InjectionToken, computed, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Router } from '@angular/router';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';
import { MuteRequest, MuteStatus, NotificationSettings, SettingsResponse, boardParams } from './board.service';
import { BoardSummary } from './board-context.service';
import { AdminService } from './admin.service';
import { SettingsApi, SettingsScope } from './settings-api';

/** The user an Admin › User detail page is editing. */
export const ADMIN_TARGET_USER_ID = new InjectionToken<number>('ADMIN_TARGET_USER_ID');

/**
 * SettingsApi for another user — the admin endpoints mirror the user's own
 * /api/dashboard ones (Tech Spec "Admin Panel Revamp"). Provided by the
 * user detail page, never at root.
 */
@Injectable()
export class AdminUserSettingsApi extends SettingsApi {
  private http = inject(HttpClient);
  private userId = inject(ADMIN_TARGET_USER_ID);
  private get base(): string {
    return `${environment.apiBasePath}/api/dashboard/admin/users/${this.userId}`;
  }

  getSettings(boardId: number | null = null): Observable<SettingsResponse> {
    return this.http.get<SettingsResponse>(`${this.base}/settings`, { params: boardParams(boardId) });
  }

  saveNotificationSettings(settings: NotificationSettings, boardId: number | null = null): Observable<NotificationSettings> {
    return this.http.put<NotificationSettings>(`${this.base}/settings/notification`, settings, { params: boardParams(boardId) });
  }

  resetNotificationSettings(boardId: number): Observable<NotificationSettings> {
    return this.http.delete<NotificationSettings>(`${this.base}/settings/notification`, { params: boardParams(boardId) });
  }

  mute(request: MuteRequest, boardId: number | null = null): Observable<MuteStatus> {
    return this.http.post<MuteStatus>(`${this.base}/settings/mute`, request, { params: boardParams(boardId) });
  }

  unmute(boardId: number | null = null): Observable<MuteStatus> {
    return this.http.delete<MuteStatus>(`${this.base}/settings/mute`, { params: boardParams(boardId) });
  }

  saveCredential(jiraEmail: string, apiToken: string): Observable<{ status: string }> {
    return this.http.put<{ status: string }>(`${this.base}/credential`, { jiraEmail, apiToken });
  }

  /** The pre-existing admin action (also DMs the user). */
  deleteCredential(): Observable<{ status: string }> {
    return this.http.delete<{ status: string }>(`${environment.apiBasePath}/dashboard/admin/users/${this.userId}/credential`);
  }
}

/**
 * Page-local board scope over the target user's boards: ?board= in the
 * detail page URL, no localStorage — it must never change the admin's own
 * navbar board.
 */
@Injectable()
export class AdminUserScope extends SettingsScope {
  private adminService = inject(AdminService);
  private router = inject(Router);
  private userId = inject(ADMIN_TARGET_USER_ID);

  readonly boards = signal<BoardSummary[]>([]);
  readonly loaded = signal(false);
  readonly selectedId = signal<number | null>(this.fromUrl());
  readonly selected = computed(() => this.boards().find((b) => b.boardId === this.selectedId()) ?? null);

  load(): void {
    this.adminService.listUserBoards(this.userId).subscribe({
      next: (res) => {
        this.boards.set(res.boards ?? []);
        this.loaded.set(true);
        const id = this.selectedId();
        if (id !== null && !this.boards().some((b) => b.boardId === id)) {
          this.select(null);
        }
      },
      error: () => this.loaded.set(true),
    });
  }

  select(boardId: number | null): void {
    this.selectedId.set(boardId);
    this.router.navigate([], { queryParams: { board: boardId ?? null }, queryParamsHandling: 'merge', replaceUrl: true });
  }

  private fromUrl(): number | null {
    const raw = this.router.parseUrl(this.router.url).queryParamMap.get('board');
    const id = Number(raw);
    return raw !== null && Number.isInteger(id) && id > 0 ? id : null;
  }
}
