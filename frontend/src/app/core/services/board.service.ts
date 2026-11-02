import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

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
}
