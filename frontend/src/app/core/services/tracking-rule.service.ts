import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

/** A Story Point field with its Jira name. */
export interface FieldRef {
  id: string;
  name: string;
}

/** One board rule (Tech Spec "Aturan Board (Admin CRUD)"). */
export interface TrackingRule {
  projectKey: string;
  projectName: string;
  trackedStatuses: string[];
  doneStatus: string;
  reviewStatus: string | null;
  /** Read priority order. */
  storyPointFields: FieldRef[];
  storyPointWriteField: string;
  storyPointWriteFieldSubtask: string;
  isActive: boolean;
  editedBy: string;
  editedDate: string;
}

export interface TrackingRuleInput {
  /** POST only; the path carries the key on PUT. */
  projectKey?: string;
  trackedStatuses: string[];
  doneStatus: string;
  reviewStatus: string | null;
  storyPointFields: string[];
  storyPointWriteField: string;
  storyPointWriteFieldSubtask: string;
}

export interface NumberField {
  id: string;
  name: string;
  storyPointLike: boolean;
}

export interface JiraProjectLookup {
  projectKey: string;
  projectName: string;
  avatarUrl: string;
  statuses: string[];
  /** Story-point-like fields first. */
  numberFields: NumberField[];
}

export type TrackingRuleField =
  | 'projectKey'
  | 'trackedStatuses'
  | 'doneStatus'
  | 'reviewStatus'
  | 'storyPointFields'
  | 'storyPointWriteField'
  | 'storyPointWriteFieldSubtask';

export interface TrackingRuleError {
  error: string;
  message?: string;
  fields?: Partial<Record<TrackingRuleField, string>>;
}

/** Same rule as the server (AC3.8). */
export const PROJECT_KEY_PATTERN = /^[A-Z][A-Z0-9_]{1,19}$/;

export function normalizeProjectKey(raw: string): string {
  return (raw ?? '').trim().toUpperCase();
}

@Injectable({ providedIn: 'root' })
export class TrackingRuleService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/dashboard/admin/tracking-rules`;

  list(): Observable<{ rules: TrackingRule[] }> {
    return this.http.get<{ rules: TrackingRule[] }>(this.base);
  }

  lookup(projectKey: string): Observable<JiraProjectLookup> {
    return this.http.get<JiraProjectLookup>(`${this.base}/jira/${encodeURIComponent(projectKey)}`);
  }

  create(input: TrackingRuleInput): Observable<TrackingRule> {
    return this.http.post<TrackingRule>(this.base, input);
  }

  update(projectKey: string, input: TrackingRuleInput): Observable<TrackingRule> {
    return this.http.put<TrackingRule>(`${this.base}/${encodeURIComponent(projectKey)}`, input);
  }

  setActive(projectKey: string, active: boolean): Observable<TrackingRule> {
    return this.http.put<TrackingRule>(`${this.base}/${encodeURIComponent(projectKey)}/active`, { active });
  }

  /** Starts the "every user" recalculation; 409 while one is running. */
  recalculate(): Observable<{ started: boolean }> {
    return this.http.post<{ started: boolean }>(`${this.base}/recalculate`, {});
  }

  /** The board project's Jira workflow statuses (Pengaturan, AC5.5). */
  boardStatuses(boardId: number): Observable<{ statuses: string[] }> {
    return this.http.get<{ statuses: string[] }>(`${environment.apiBasePath}/api/dashboard/boards/${boardId}/statuses`);
  }
}
