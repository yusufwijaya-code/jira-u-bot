import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

export interface ReportRow {
  sprintName: string;
  issueKey: string;
  task: string;
  estimatedHours: number;
  actualHours: number;
  deviationPercent: number;
  within: boolean;
}

export interface Report {
  rows: ReportRow[];
  sumEstimatedHours: number;
  sumActualHours: number;
  aggregateDeviationPercent: number;
  achievementPercent: number;
  capPercent: number;
  withinCount: number;
  totalCount: number;
}

export interface GetReportResponse {
  availableSprints: string[];
  capPercent: number;
  report: Report | null;
}

@Injectable({ providedIn: 'root' })
export class ReportService {
  private http = inject(HttpClient);

  getReport(sprints: string[] = [], capPercent?: number): Observable<GetReportResponse> {
    let params = new HttpParams();
    for (const sprint of sprints) {
      params = params.append('sprint', sprint);
    }
    if (capPercent !== undefined) {
      params = params.set('cap', String(capPercent));
    }
    return this.http.get<GetReportResponse>(`${environment.apiBasePath}/api/dashboard/report`, { params });
  }
}
