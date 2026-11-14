import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';
import { BoardSummary } from './board-context.service';

export interface UserRow {
  userId: number;
  email: string;
  userName: string;
  flagActive: boolean;
  createdDate: string;
  jiraAccountId: string;
  hasCredential: boolean;
  credentialMasked: string;
}

export interface ListUsersResponse {
  users: UserRow[];
}

export interface BroadcastResponse {
  sentCount: number;
  failedEmails: string[];
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
  listUserBoards(userId: number): Observable<{ boards: BoardSummary[] | null }> {
    return this.http.get<{ boards: BoardSummary[] | null }>(`${environment.apiBasePath}/api/dashboard/admin/users/${userId}/boards`);
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
