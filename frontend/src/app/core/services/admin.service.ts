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

/** One "Pesan" entry (a message Nudge sent or an event it got). */
export interface ChatLogEntry {
  id: number;
  email: string;
  direction: 'in' | 'out';
  kind: 'reminder' | 'message' | 'reply' | 'command' | 'click' | 'added' | 'removed';
  channel: 'app' | 'operator' | 'fallback' | '';
  status: 'received' | 'sent' | 'failed' | 'skipped';
  body: string;
  error?: string;
  createdDate: string;
}

export interface ChatLogPage {
  items: ChatLogEntry[];
  /** Pass as beforeID for the next (older) page; 0 = no more. */
  nextBeforeID: number;
}

export interface ChatLogQuery {
  email?: string;
  /** YYYY-MM-DD (WIB), inclusive. */
  from?: string;
  to?: string;
  beforeID?: number;
  /** Only entries newer than this ID (live updates). */
  afterID?: number;
  /** Comma list of kinds; '' = all. */
  kind?: string;
  limit?: number;
}

/** One user's thread with Nudge in the "Pesan". */
export interface ChatConversation {
  email: string;
  total: number;
  lastDirection: 'in' | 'out';
  lastKind: string;
  lastStatus: string;
  lastBody: string;
  lastDate: string;
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

  chatLogs(q: ChatLogQuery): Observable<ChatLogPage> {
    const params: Record<string, string> = {};
    if (q.email) params['email'] = q.email;
    if (q.from) params['from'] = q.from;
    if (q.to) params['to'] = q.to;
    if (q.beforeID) params['beforeID'] = String(q.beforeID);
    if (q.afterID) params['afterID'] = String(q.afterID);
    if (q.kind) params['kind'] = q.kind;
    if (q.limit) params['limit'] = String(q.limit);
    return this.http.get<ChatLogPage>(`${environment.apiBasePath}/api/dashboard/admin/chat-logs`, { params });
  }

  chatConversations(): Observable<{ conversations: ChatConversation[] }> {
    return this.http.get<{ conversations: ChatConversation[] }>(`${environment.apiBasePath}/api/dashboard/admin/chat-logs/conversations`);
  }

  broadcast(message: string): Observable<BroadcastResponse> {
    return this.http.post<BroadcastResponse>(`${environment.apiBasePath}/dashboard/admin/broadcast`, { message });
  }
}
