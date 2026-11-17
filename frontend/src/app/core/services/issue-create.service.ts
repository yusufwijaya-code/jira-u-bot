import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { environment } from '../../../environments/environment';

import { CascadeValue, JiraField, JiraFieldKind, JiraFieldOption } from '../../shared/ui/jira-field-input.component';

export type { CascadeValue };
export type CreateFieldKind = Exclude<JiraFieldKind, 'unsupported'>;
export type CreateOption = JiraFieldOption;
/** A createmeta field the form fills (value shapes: see JiraField). */
export type CreateField = JiraField;
export interface CreateType { id: string; name: string; subtask: boolean; supported: boolean; parentAllowed: boolean; storyPointField?: string; fields: CreateField[] }
export interface CreateMeta { projectKey: string; types: CreateType[]; subtaskTypeId: string }
export interface JiraUser { accountId: string; displayName: string; avatarUrl: string }
/** Jira status category key: new (To Do), indeterminate (In Progress), done. */
export type StatusCategory = 'new' | 'indeterminate' | 'done' | string;
export interface ParentInfo {
  key: string; summary: string; subtasks: { key: string; summary: string }[];
  type?: string; typeIconUrl?: string; status?: string; statusCategory?: StatusCategory;
}
export interface ParentCandidate { key: string; summary: string; type: string; typeIconUrl: string; status: string; statusCategory: StatusCategory; subtaskCount: number }
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
  /** Open-sprint tickets of the board's project that can hold subtasks (no epics/subtasks). */
  parents(boardId: number, q = '') { return this.http.get<{ parents: ParentCandidate[] }>(`${this.base}/parents`, { params: this.board(boardId, { q }) }); }
  createIssue(body: CreateIssueBody) { return this.http.post<{ key: string; url: string }>(`${this.base}/issue`, body); }
  normalize(boardId: number, text: string) { return this.http.post<NormalizeResult>(`${this.base}/normalize`, { boardId, text }); }
  createSubtasks(boardId: number, parentKey: string, rows: BulkRowRequest[]) {
    return this.http.post<{ results: BulkResult[] }>(`${this.base}/subtasks`, { boardId, parentKey, rows });
  }
}
