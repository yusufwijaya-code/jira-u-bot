import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

export interface UserRow {
  userId: number;
  email: string;
  userName: string;
  flagActive: boolean;
  createdDate: string;
  jiraAccountId: string;
  hasCredential: boolean;
  credentialMasked: string;
}

export interface ListUsersResponse {
  users: UserRow[];
}

@Injectable({ providedIn: 'root' })
export class AdminService {
  private http = inject(HttpClient);

  listUsers(): Observable<ListUsersResponse> {
    return this.http.get<ListUsersResponse>(`${environment.apiBasePath}/api/dashboard/admin/users`);
  }
}
