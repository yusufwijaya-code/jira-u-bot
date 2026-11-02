import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

export interface Card {
  issueKey: string;
  summary: string;
  status: string;
  hasBudgetDetail: boolean;
  storyPoints: number;
  elapsedLabel: string;
  remainingLabel: string;
  deadlineLabel: string;
}

export interface BoardColumn {
  status: string;
  cards: Card[];
}

export interface BoardResponse {
  columns: BoardColumn[];
  tierPreference: { tiers: string[] };
  credential: { hasCredential: boolean; maskedPreview: string };
}

export interface TransitionResponse {
  message: string;
}

@Injectable({ providedIn: 'root' })
export class BoardService {
  private http = inject(HttpClient);

  updatePreference(tiers: string[]): Observable<{ status: string }> {
    return this.http.post<{ status: string }>(`${environment.apiBasePath}/api/dashboard/preference`, { tiers });
  }

  saveCredential(jiraEmail: string, apiToken: string): Observable<{ status: string }> {
    return this.http.post<{ status: string }>(`${environment.apiBasePath}/api/dashboard/credential`, {
      jiraEmail,
      apiToken,
    });
  }

  deleteCredential(): Observable<{ status: string }> {
    return this.http.delete<{ status: string }>(`${environment.apiBasePath}/api/dashboard/credential`);
  }

  getBoard(): Observable<BoardResponse> {
    return this.http.get<BoardResponse>(`${environment.apiBasePath}/api/dashboard/board`);
  }

  transitionTicket(issueKey: string, target: 'todo' | 'code-review'): Observable<TransitionResponse> {
    return this.http.post<TransitionResponse>(
      `${environment.apiBasePath}/api/dashboard/tickets/${encodeURIComponent(issueKey)}/transition`,
      { target },
    );
  }
}
