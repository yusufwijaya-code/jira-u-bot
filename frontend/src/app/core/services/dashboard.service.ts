import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';
import { boardParams } from './board.service';

/** One of the caller's Jira tickets (budget fields only when tracked in Code Process). */
export interface TicketCard {
  issueKey: string;
  summary: string;
  status: string;
  issueURL: string;
  /** Board of the ticket's active sprint (0 = none). */
  boardId: number;
  hasBudgetDetail: boolean;
  storyPoints: number;
  elapsedSeconds: number;
  budgetSeconds: number;
  elapsedLabel: string;
  remainingLabel: string;
  deadlineLabel: string;
  /** RFC3339: when the tracked session started (entered Code Process). */
  startedAt?: string;
  /** RFC3339: when the budget runs out in business hours. */
  deadlineAt?: string;
  /** Set on a branch: the active-sprint ticket this subtask/linked item hangs under. */
  parentKey?: string;
  parentSummary?: string;
  parentStatus?: string;
  /** "subtask" or the Jira link wording ("relates to", "blocks", ...). */
  relation?: string;
}

export interface BoardResponse {
  columns: { status: string; cards: TicketCard[] }[];
  /** Subtasks/linked items (tracked status) outside the sprint — only for the active/review cards. */
  related?: TicketCard[];
}

export interface SprintReportRow {
  sprintName: string;
  issueKey: string;
  task: string;
  estimatedHours: number;
  actualHours: number;
  deviationPercent: number;
  within: boolean;
}

export interface SprintReport {
  rows: SprintReportRow[];
  sumEstimatedHours: number;
  sumActualHours: number;
  aggregateDeviationPercent: number;
  achievementPercent: number;
  capPercent: number;
  withinCount: number;
  totalCount: number;
}

export interface SprintReportResponse {
  availableSprints: string[] | null;
  capPercent: number;
  report: SprintReport | null;
}

export type TransitionTarget = 'todo' | 'code-review';

/** Read-only views of the caller's own tickets — every endpoint is scoped to the session server-side. */
@Injectable({ providedIn: 'root' })
export class DashboardService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/dashboard`;

  /** Tickets in active sprints — one board's, or every board's when null. */
  getBoard(boardId: number | null = null): Observable<BoardResponse> {
    return this.http.get<BoardResponse>(`${this.base}/board`, { params: boardParams(boardId) });
  }

  /** Without sprints: just the sprint list. With sprints: the report for them. */
  getSprintReport(sprints: string[] = []): Observable<SprintReportResponse> {
    let params = new HttpParams();
    for (const sprint of sprints) {
      params = params.append('sprint', sprint);
    }
    return this.http.get<SprintReportResponse>(`${this.base}/report`, { params });
  }

  transitionTicket(issueKey: string, target: TransitionTarget): Observable<{ message: string }> {
    return this.http.post<{ message: string }>(
      `${this.base}/tickets/${encodeURIComponent(issueKey)}/transition`,
      { target },
    );
  }
}
