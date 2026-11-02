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
}
