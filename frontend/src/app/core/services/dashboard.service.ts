import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';
import { boardParams } from './board.service';
import { MoveRequirement } from '../ui/move-fields.service';

/** One of the caller's Jira tickets (budget fields only when tracked in Code Process). */
export interface TicketCard {
  issueKey: string;
  summary: string;
  status: string;
  issueURL: string;
  /** Jira issue type and its icon (shown before the key); absent on older API answers. */
  issueType?: string;
  issueTypeIconUrl?: string;
  /** Board of the ticket's active sprint (0 = none). */
  boardId: number;
  hasBudgetDetail: boolean;
  /** Clock stopped (finished / back to To Do) with recorded time: elapsed/budget hold the final tally. */
  timeRecorded?: boolean;
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

export type TransitionTarget = 'todo' | 'code-review' | 'code-completed';

/** One move the ticket's Jira workflow allows from its current status. */
export interface TransitionOption {
  id: string;
  name: string;
  toStatus: string;
}

export interface TicketTransitions {
  issueKey: string;
  currentStatus: string;
  transitions: TransitionOption[];
}

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

  /**
   * Rebuilds the caller's untracked active-sprint time from Jira's changelog
   * (Tech Spec "Active Sprint Tracking & Changelog Backfill").
   */
  backfillSprint(): Observable<{ added: number; updated: number }> {
    return this.http.post<{ added: number; updated: number }>(`${this.base}/report/backfill`, {});
  }

  /** Moves the ticket's Jira workflow allows right now — CORE and OMS differ (Papan Tiket). */
  getTransitions(issueKey: string): Observable<TicketTransitions> {
    return this.http.get<TicketTransitions>(`${this.base}/tickets/${encodeURIComponent(issueKey)}/transitions`);
  }

  /**
   * Runs one of getTransitions' moves; returns the new status. A 422
   * "fields_required" (see fieldsRequired) means Jira needs fields filled
   * first: send them in `fields` (saved before moving).
   */
  moveTicket(issueKey: string, transitionId: string, fields?: Record<string, unknown>): Observable<{ message: string; status: string }> {
    return this.http.post<{ message: string; status: string }>(
      `${this.base}/tickets/${encodeURIComponent(issueKey)}/transition`,
      fields ? { transitionId, fields } : { transitionId },
    );
  }

  /** The still-empty fields a move requires (fields [] when it can go ahead). */
  getMoveRequirements(issueKey: string, transitionId: string): Observable<MoveRequirement> {
    return this.http.get<MoveRequirement>(`${this.base}/tickets/${encodeURIComponent(issueKey)}/move-requirements`, {
      params: new HttpParams().set('transitionId', transitionId),
    });
  }

  transitionTicket(issueKey: string, target: TransitionTarget): Observable<{ message: string }> {
    return this.http.post<{ message: string }>(
      `${this.base}/tickets/${encodeURIComponent(issueKey)}/transition`,
      { target },
    );
  }
}
