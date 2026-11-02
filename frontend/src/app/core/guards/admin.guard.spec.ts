import { TestBed } from '@angular/core/testing';
import { Router } from '@angular/router';
import { BehaviorSubject } from 'rxjs';
import { AdminGuard } from './admin.guard';
import { AuthService, CurrentUser } from '../services/auth.service';

describe('AdminGuard', () => {
  let guard: AdminGuard;
  let currentUser$: BehaviorSubject<CurrentUser | null>;
  let router: jasmine.SpyObj<Router>;

  beforeEach(() => {
    currentUser$ = new BehaviorSubject<CurrentUser | null>(null);
    router = jasmine.createSpyObj('Router', ['navigate']);

    TestBed.configureTestingModule({
      providers: [AdminGuard, { provide: AuthService, useValue: { currentUser$ } }, { provide: Router, useValue: router }],
    });

    guard = TestBed.inject(AdminGuard);
  });

  it('allows navigation when the current user is admin', (done) => {
    currentUser$.next({ email: 'admin@esb.co.id', isAdmin: true });

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(true);
      expect(router.navigate).not.toHaveBeenCalled();
      done();
    });
  });

  it('redirects to My Board and denies navigation when the current user is not admin', (done) => {
    currentUser$.next({ email: 'dev@esb.co.id', isAdmin: false });

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(false);
      expect(router.navigate).toHaveBeenCalledWith(['/dashboard']);
      done();
    });
  });
});
