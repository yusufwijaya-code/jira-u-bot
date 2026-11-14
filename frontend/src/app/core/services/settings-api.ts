import { Injectable, InjectionToken, Signal } from '@angular/core';
import { Observable } from 'rxjs';
import { BoardService, MuteRequest, MuteStatus, NotificationSettings, SettingsResponse } from './board.service';
import { BoardContextService, BoardSummary } from './board-context.service';

/**
 * The HTTP calls the Pengaturan components make. By default the caller's own
 * settings (BoardService → /api/dashboard/...); Admin › User detail provides
 * an implementation for another user (Tech Spec "Admin Panel Revamp").
 */
@Injectable({ providedIn: 'root', useExisting: BoardService })
export abstract class SettingsApi {
  abstract getSettings(boardId?: number | null): Observable<SettingsResponse>;
  abstract saveNotificationSettings(settings: NotificationSettings, boardId?: number | null): Observable<NotificationSettings>;
  abstract resetNotificationSettings(boardId: number): Observable<NotificationSettings>;
  abstract mute(request: MuteRequest, boardId?: number | null): Observable<MuteStatus>;
  abstract unmute(boardId?: number | null): Observable<MuteStatus>;
  abstract saveCredential(jiraEmail: string, apiToken: string): Observable<{ status: string }>;
  abstract deleteCredential(): Observable<{ status: string }>;
}

/**
 * Which board the Pengaturan page is scoped to and which boards it can pick.
 * By default the navbar picker (BoardContextService); Admin › User detail
 * provides a page-local scope over the target user's boards.
 */
@Injectable({ providedIn: 'root', useExisting: BoardContextService })
export abstract class SettingsScope {
  abstract readonly boards: Signal<BoardSummary[]>;
  abstract readonly selectedId: Signal<number | null>;
  abstract select(boardId: number | null): void;
  abstract load(): void;
}

/** 'self' = the user's own Pengaturan; 'admin' = an admin editing another user. */
export type SettingsMode = 'self' | 'admin';

export const SETTINGS_MODE = new InjectionToken<SettingsMode>('SETTINGS_MODE', {
  providedIn: 'root',
  factory: () => 'self',
});
