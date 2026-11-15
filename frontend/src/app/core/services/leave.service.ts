import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

export interface Leave {
  /** 2006-01-02 */
  date: string;
  note: string;
}

export interface AddLeaveResult {
  added: string[];
  /** Weekends and national holidays inside the range. */
  skipped: string[];
  /** A past day changed: tracked time is being rebuilt from Jira. */
  recalculating: boolean;
}

/** The session user's own leave days (Tech Spec "Papan Tiket & Cuti", US-2). */
@Injectable({ providedIn: 'root' })
export class LeaveService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/dashboard/leaves`;

  list(): Observable<{ leaves: Leave[] }> {
    return this.http.get<{ leaves: Leave[] }>(this.base);
  }

  add(from: string, to: string, note: string): Observable<AddLeaveResult> {
    return this.http.post<AddLeaveResult>(this.base, { from, to, note });
  }

  remove(date: string): Observable<{ deleted: string; recalculating: boolean }> {
    return this.http.delete<{ deleted: string; recalculating: boolean }>(`${this.base}/${date}`);
  }
}
