import { Injectable, inject } from '@angular/core';
import { CanActivate, Router } from '@angular/router';
import { Observable, of } from 'rxjs';
import { catchError, map } from 'rxjs/operators';
import { AuthService } from '../services/auth.service';

@Injectable({ providedIn: 'root' })
export class AuthGuard implements CanActivate {
  private authService = inject(AuthService);
  private router = inject(Router);

  canActivate(): Observable<boolean> {
    if (this.authService.checked) {
      if (this.authService.currentUser$.value) {
        return of(true);
      }
      this.router.navigate(['/']);
      return of(false);
    }

    return this.authService.checkSession().pipe(
      map(() => true),
      catchError(() => {
        this.router.navigate(['/']);
        return of(false);
      }),
    );
  }
}
