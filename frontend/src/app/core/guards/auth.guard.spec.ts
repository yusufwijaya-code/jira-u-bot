import { TestBed } from '@angular/core/testing';
import { Router } from '@angular/router';
import { of, throwError } from 'rxjs';
import { AuthGuard } from './auth.guard';
import { AuthService, CurrentUser } from '../services/auth.service';

class FakeAuthService {
  checked = false;
  currentUser$ = { value: null as CurrentUser | null };
  checkSession = jasmine.createSpy('checkSession');
}

describe('AuthGuard', () => {
  let guard: AuthGuard;
  let authService: FakeAuthService;
  let router: jasmine.SpyObj<Router>;

  beforeEach(() => {
    authService = new FakeAuthService();
    router = jasmine.createSpyObj('Router', ['navigate']);

    TestBed.configureTestingModule({
      providers: [
        AuthGuard,
        { provide: AuthService, useValue: authService },
        { provide: Router, useValue: router },
      ],
    });

    guard = TestBed.inject(AuthGuard);
  });

  it('calls checkSession and allows navigation on success when not yet checked', (done) => {
    authService.checkSession.and.returnValue(of({ email: 'a@esb.co.id', isAdmin: false }));

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(true);
      expect(router.navigate).not.toHaveBeenCalled();
      done();
    });
  });

  it('redirects to Login and denies navigation when checkSession fails', (done) => {
    authService.checkSession.and.returnValue(throwError(() => ({ status: 401 })));

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(false);
      expect(router.navigate).toHaveBeenCalledWith(['/']);
      done();
    });
  });

  it('allows navigation without re-calling checkSession when already checked and logged in', (done) => {
    authService.checked = true;
    authService.currentUser$.value = { email: 'a@esb.co.id', isAdmin: false };

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(true);
      expect(authService.checkSession).not.toHaveBeenCalled();
      done();
    });
  });

  it('redirects to Login when already checked but not logged in', (done) => {
    authService.checked = true;
    authService.currentUser$.value = null;

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(false);
      expect(router.navigate).toHaveBeenCalledWith(['/']);
      done();
    });
  });
});
