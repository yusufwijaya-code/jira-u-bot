import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';
import { BoardsResponse } from './board-context.service';

export interface UserRow {
  userId: number;
  email: string;
  userName: string;
  flagActive: boolean;
  createdDate: string;
  jiraAccountId: string;
  hasCredential: boolean;
  credentialMasked: string;
  chatAppActivated: boolean;
  chatAppActivatedDate: string | null;
  chatSender: 'nudge' | 'operator' | null;
  effectiveChatSender: 'nudge' | 'operator';
}

export interface ListUsersResponse {
  users: UserRow[];
}

export interface BroadcastResponse {
  sentCount: number;
  failedEmails: string[];
  /** Users no sender can reach yet (e.g. not activated) — not failures. */
  skippedEmails?: string[] | null;
}

/** A Nudge App activation date as "15 Nov 2026" (WIB), or "—" (AC4.1). */
export function formatActivationDate(iso: string | null): string {
  if (!iso) return '—';
  return new Date(iso)
    .toLocaleDateString('id-ID', { timeZone: 'Asia/Jakarta', day: '2-digit', month: 'short', year: 'numeric' })
    .replace('.', '');
}

@Injectable({ providedIn: 'root' })
export class AdminService {
  private http = inject(HttpClient);

  listUsers(): Observable<ListUsersResponse> {
    return this.http.get<ListUsersResponse>(`${environment.apiBasePath}/api/dashboard/admin/users`);
  }

  /** One user (404 when unknown) — Tech Spec "Admin Panel Revamp". */
  getUser(userId: number): Observable<UserRow> {
    return this.http.get<UserRow>(`${environment.apiBasePath}/api/dashboard/admin/users/${userId}`);
  }

  /** The user's boards, same shape as the navbar picker's GET /boards. */
  listUserBoards(userId: number): Observable<BoardsResponse> {
    return this.http.get<BoardsResponse>(`${environment.apiBasePath}/api/dashboard/admin/users/${userId}/boards`);
  }

  toggleActive(userId: number, active: boolean): Observable<unknown> {
    return this.http.post(`${environment.apiBasePath}/dashboard/admin/users/${userId}/active`, { active });
  }

  editJiraAccountId(userId: number, jiraAccountId: string): Observable<unknown> {
    return this.http.post(`${environment.apiBasePath}/dashboard/admin/users/${userId}/jira-account-id`, { jiraAccountId });
  }

  deleteCredential(userId: number): Observable<unknown> {
    return this.http.delete(`${environment.apiBasePath}/dashboard/admin/users/${userId}/credential`);
  }

  deleteUser(userId: number): Observable<unknown> {
    return this.http.delete(`${environment.apiBasePath}/dashboard/admin/users/${userId}`);
  }

  broadcast(message: string): Observable<BroadcastResponse> {
    return this.http.post<BroadcastResponse>(`${environment.apiBasePath}/dashboard/admin/broadcast`, { message });
  }
}
