import { Injectable, inject } from '@angular/core';
import { CanActivate, Router, UrlTree } from '@angular/router';
import { Observable, of } from 'rxjs';
import { catchError, map } from 'rxjs/operators';
import { AuthService } from '../services/auth.service';

// GuestGuard keeps the Login screen for logged-out visitors only: a fresh tab
// opened on "/" with a live session cookie goes straight to the dashboard.
@Injectable({ providedIn: 'root' })
export class GuestGuard implements CanActivate {
  private authService = inject(AuthService);
  private router = inject(Router);

  canActivate(): Observable<boolean | UrlTree> {
    if (this.authService.checked) {
      return of(this.authService.currentUser$.value ? this.router.parseUrl('/dashboard') : true);
    }
    return this.authService.checkSession().pipe(
      map(() => this.router.parseUrl('/dashboard')),
      catchError(() => of(true)),
    );
  }
}
