import { TestBed } from '@angular/core/testing';
import { Router, UrlTree } from '@angular/router';
import { of, throwError } from 'rxjs';
import { GuestGuard } from './guest.guard';
import { AuthService, CurrentUser } from '../services/auth.service';

class FakeAuthService {
  checked = false;
  currentUser$ = { value: null as CurrentUser | null };
  checkSession = jasmine.createSpy('checkSession');
}

describe('GuestGuard', () => {
  let guard: GuestGuard;
  let authService: FakeAuthService;
  let router: jasmine.SpyObj<Router>;
  const dashboardTree = {} as UrlTree;

  beforeEach(() => {
    authService = new FakeAuthService();
    router = jasmine.createSpyObj('Router', ['parseUrl']);
    router.parseUrl.and.returnValue(dashboardTree);

    TestBed.configureTestingModule({
      providers: [
        GuestGuard,
        { provide: AuthService, useValue: authService },
        { provide: Router, useValue: router },
      ],
    });

    guard = TestBed.inject(GuestGuard);
  });

  it('redirects a user with a live session (fresh tab) to the dashboard', (done) => {
    authService.checkSession.and.returnValue(of({ email: 'a@esb.co.id', isAdmin: false }));

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(dashboardTree);
      expect(router.parseUrl).toHaveBeenCalledWith('/dashboard');
      done();
    });
  });

  it('shows Login when there is no session', (done) => {
    authService.checkSession.and.returnValue(throwError(() => ({ status: 401 })));

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(true);
      done();
    });
  });

  it('redirects without re-checking when already known to be logged in', (done) => {
    authService.checked = true;
    authService.currentUser$.value = { email: 'a@esb.co.id', isAdmin: false };

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(dashboardTree);
      expect(authService.checkSession).not.toHaveBeenCalled();
      done();
    });
  });

  it('shows Login without re-checking when already known to be logged out', (done) => {
    authService.checked = true;

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(true);
      expect(authService.checkSession).not.toHaveBeenCalled();
      done();
    });
  });
});
