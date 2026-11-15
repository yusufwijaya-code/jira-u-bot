import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { environment } from '../../../environments/environment';

export interface CreateField { id: string; name: string; kind: 'text' | 'number' | 'select'; multi: boolean; required: boolean; options?: { id: string; label: string }[] }
export interface CreateType { id: string; name: string; subtask: boolean; supported: boolean; parentAllowed: boolean; storyPointField?: string; fields: CreateField[] }
export interface CreateMeta { projectKey: string; types: CreateType[]; subtaskTypeId: string }
export interface JiraUser { accountId: string; displayName: string; avatarUrl: string }
export interface ParentInfo { key: string; summary: string; subtasks: { key: string; summary: string }[] }
export interface NormalizedRow { title: string; storyPoints: number | null; details: string }
export interface NormalizeResult { rows: NormalizedRow[]; warnings: string[]; cached: boolean }
export interface BulkRowRequest { clientId: string; title: string; storyPoints: number | null; assigneeAccountId: string | null; details: string }
export interface BulkResult { clientId: string; key?: string; error?: string }
export interface CreateStatus { tokenReady: boolean; aiReady: boolean }
export type CreateIssueBody = { boardId: number; typeId: string; summary: string; description: string; storyPoints: number | null; assigneeAccountId: string | null; parentKey: string | null; fields: Record<string, unknown> };

/** Buat Card endpoints (/api/dashboard/create/*); Jira calls run with the user's own token server-side. */
@Injectable({ providedIn: 'root' })
export class IssueCreateService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/dashboard/create`;
  private board(id: number, extra: Record<string, string> = {}): HttpParams {
    return new HttpParams({ fromObject: { boardId: String(id), ...extra } });
  }
  status() { return this.http.get<CreateStatus>(`${this.base}/status`); }
  meta(boardId: number) { return this.http.get<CreateMeta>(`${this.base}/meta`, { params: this.board(boardId) }); }
  users(boardId: number, q: string) { return this.http.get<{ users: JiraUser[] }>(`${this.base}/users`, { params: this.board(boardId, { q }) }); }
  parent(boardId: number, key: string) { return this.http.get<ParentInfo>(`${this.base}/parent/${encodeURIComponent(key)}`, { params: this.board(boardId) }); }
  createIssue(body: CreateIssueBody) { return this.http.post<{ key: string; url: string }>(`${this.base}/issue`, body); }
  normalize(boardId: number, text: string) { return this.http.post<NormalizeResult>(`${this.base}/normalize`, { boardId, text }); }
  createSubtasks(boardId: number, parentKey: string, rows: BulkRowRequest[]) {
    return this.http.post<{ results: BulkResult[] }>(`${this.base}/subtasks`, { boardId, parentKey, rows });
  }
}
