# Angular Dashboard Revamp — Frontend (Plan B) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the new Angular 19 standalone-component SPA (`frontend/`) that will replace jira-u-bot's server-rendered HTML dashboard — login, My Board, Team, Overdue, Admin User Management, Sprint Report — consuming the 6 JSON endpoints Plan A already shipped plus the existing POST/DELETE action endpoints.

**Architecture:** One new Angular 19 project at `frontend/` (root of the repo, sibling to `modules/`), standalone components (no `NgModule`), Angular Router with class-based guards (`AuthGuard`, `AdminGuard`), a shared `BoardService`/`AdminService`/`ReportService`/`AuthService` layer wrapping `HttpClient`. The app is built and tested in isolation in this plan (`ng build`, `ng test`, manual `ng serve` against the real backend via a dev proxy) — it is **not wired into the Go binary yet**. Plan C (not yet written) embeds the built `dist/` into the Go binary via `go:embed`, adds the catch-all shell route, and deletes the old HTML routes/templates. Until Plan C ships, none of this plan's work is reachable in production — the existing HTML pages keep serving real users exactly as today.

**Tech Stack:** Angular 19, TypeScript, standalone components (`input()`/`output()` signals, `inject()`-based DI — matching `oms-setting-angular`'s current convention), RxJS (`HttpClient` returns `Observable`), Karma + Jasmine (Angular CLI default, confirmed as `oms-setting-angular`'s test stack) for `.spec.ts` unit/component tests.

**Spec:** `docs/tech-spec/angular-dashboard-revamp/Tech Spec - Angular Dashboard Revamp.md` — Repository 2 section (and the AC table in Background, and Repository 1's Chapter 1 for the exact `/api/auth/me` contract this plan consumes).

## Global Constraints

- No backend changes in this plan — every endpoint this plan calls already exists (6 new JSON GETs from Plan A, plus the pre-existing POST/DELETE action endpoints). If any task's implementer thinks a backend change is needed, that's a signal the task brief or this plan has a defect — stop and flag it, don't modify Go code.
- Standalone components only — no `NgModule` anywhere in `frontend/src/app/`.
- `inject()` for dependency injection in components/services/guards (not constructor injection) — matches `oms-setting-angular`'s current files.
- All HTTP calls go through `HttpClient`, through one of three services (`BoardService`, `AdminService`, `ReportService`) or `AuthService` — components never call `HttpClient` directly.
- API paths are built from `environment.apiBasePath` (`'/jira-notif'`) + the path documented in each chapter's Integration section — never hardcode `/jira-notif` inline in a component or service method body.
- Error message strings shown to the user are copied verbatim from the spec's Response Handling / Integration sections for each chapter — do not paraphrase or retranslate them.
- Every component/service/guard file this plan creates gets a `.spec.ts` with at minimum the scenarios listed in its chapter's Task Breakdown `[N].u`/`[N].i` rows.
- `ng build` and `ng test --watch=false --browsers=ChromeHeadless` must both pass with zero errors after every task — zero tolerance for regressions, same bar as Plan A's `go build/vet/test`.
- Router `base href` is `/jira-notif/` for the whole SPA (set in `frontend/src/index.html`) — this is the only value under which both the bare root (`/jira-notif/`, Login) and the nested dashboard paths (`/jira-notif/dashboard`, `/jira-notif/dashboard/admin`, etc.) resolve correctly for Angular's relative routing. Dashboard child route paths (`dashboard`, `dashboard/team`, `dashboard/overdue`, `dashboard/admin`, `dashboard/report`) are chosen to be byte-identical to the OLD HTML page paths, so existing user bookmarks keep landing on the right page once Plan C's catch-all goes live (per the spec's "Impact on Existing Features" row on bookmarks).
- Angular build output path and `baseHref` are configured now (`angular.json` → `"baseHref": "/jira-notif/"`) so Plan C's embed step is a drop-in, not a rework.

## File Structure

```
frontend/
  angular.json, package.json, tsconfig*.json       — CLI scaffold (Task 0)
  proxy.conf.json                                  — dev-only proxy to the real Go backend (Task 0)
  src/
    index.html                                     — <base href="/jira-notif/">
    main.ts                                        — bootstrapApplication(AppComponent, appConfig)
    styles.scss                                    — global styles (ported from login.html/dashboard.html's shared CSS variables)
    environments/
      environment.ts                               — { apiBasePath: '/jira-notif' }
    app/
      app.component.ts                              — root shell, just <router-outlet>
      app.config.ts                                 — provideRouter, provideHttpClient(withInterceptors([authInterceptor]))
      app.routes.ts                                  — route table (Chapter 1, extended Chapter 7a)
      core/
        services/
          auth.service.ts                           — Chapter 1
          board.service.ts                           — Chapter 2/3/4/5/6 (getBoard/getTeam/getOverdue/transitionTicket/updatePreference/saveCredential/deleteCredential)
          admin.service.ts                           — Chapter 7a/7b/8 (listUsers/toggleActive/editJiraAccountId/deleteCredential/deleteUser/broadcast)
          report.service.ts                          — Chapter 9 (getReport)
        guards/
          auth.guard.ts, auth.guard.spec.ts           — Chapter 1
          admin.guard.ts, admin.guard.spec.ts         — Chapter 7a
        interceptors/
          auth.interceptor.ts, auth.interceptor.spec.ts — Chapter 1 (global 401 → redirect to Login, per spec Notes C4)
      layouts/
        dashboard-shell/
          dashboard-shell.component.ts/.html/.spec.ts — Chapter 1 (nav + <router-outlet>, gap-filled — see Chapter 1 note)
      features/
        login/
          login.component.ts/.html/.spec.ts           — Chapter 1
        dashboard/
          board/
            board.component.ts/.html/.spec.ts                      — Chapter 4
            reminder-preference.component.ts/.html/.spec.ts        — Chapter 2
            jira-credential.component.ts/.html/.spec.ts             — Chapter 3
          team/
            team.component.ts/.html/.spec.ts                        — Chapter 5
          overdue/
            overdue.component.ts/.html/.spec.ts                     — Chapter 6
          admin/
            admin.component.ts/.html/.spec.ts                       — Chapter 7a/7b
            broadcast.component.ts/.html/.spec.ts                   — Chapter 8
          report/
            report.component.ts/.html/.spec.ts                     — Chapter 9
```

**Gap found during plan authoring (self-review, documented here per the writing-plans skill's "add the task if the spec requirement has no task" rule):** the spec's AC1.5 ("Jika sudah login, tampilkan Dashboard Shell dengan navigasi ke semua halaman") and the UI Flowchart's node H ("Tampilkan Dashboard Shell plus Nav") require a shell/nav component, but no chapter's Task Breakdown creates one. Folded into Chapter 1 as Task 1.4, since the shell is exactly what `AuthGuard` reveals once authenticated — the same point in the flow Chapter 1 already owns.

---

### Task 0: Project Scaffolding

**Files:**
- Create: `frontend/` (entire Angular CLI project)
- Create: `frontend/proxy.conf.json`
- Create: `frontend/src/environments/environment.ts`

**Interfaces:**
- Consumes: nothing (first task).
- Produces: a working, empty Angular 19 standalone project at `frontend/` that `ng build` and `ng test` run clean against, plus `environment.apiBasePath` that every later service imports.

- [ ] **Step 1: Generate the Angular project**

From the repo root (`C:\Apache24\htdocs\jira-u-bot`):

```bash
npx -y @angular/cli@19 new frontend --standalone --routing --style=scss --skip-git --package-manager=npm
```

When prompted (non-interactive flags above should skip prompts, but if the CLI still asks): choose **No** for SSR/SSG ("Do you want to enable Server-Side Rendering (SSR)...") — this is a pure client-rendered SPA served from the Go binary, no Node server at runtime.

- [ ] **Step 2: Set `base href` to `/jira-notif/`**

Open `frontend/src/index.html`, find:

```html
<base href="/">
```

Replace with:

```html
<base href="/jira-notif/">
```

- [ ] **Step 3: Set the build `baseHref` in `angular.json`**

In `frontend/angular.json`, under `projects.frontend.architect.build.options`, add:

```json
"baseHref": "/jira-notif/",
```

(Keep every other existing key in that `options` object — this only adds one new key.)

- [ ] **Step 4: Create the environment file**

Create `frontend/src/environments/environment.ts`:

```typescript
export const environment = {
  apiBasePath: '/jira-notif',
};
```

- [ ] **Step 5: Create the dev proxy config**

Create `frontend/proxy.conf.json` — lets `ng serve` forward API/auth calls to the real running Go backend during manual dev-testing (`ng serve --proxy-config proxy.conf.json`), so cookies/sessions behave identically to production instead of hitting a dead `localhost:4200/jira-notif/api/...`:

```json
{
  "/jira-notif/api": {
    "target": "http://localhost:8080",
    "secure": false,
    "changeOrigin": true
  },
  "/jira-notif/auth": {
    "target": "http://localhost:8080",
    "secure": false,
    "changeOrigin": true
  }
}
```

(`http://localhost:8080` assumes the Go binary's default dev port — adjust to whatever port the implementer's local `jira-u-bot` instance actually listens on; check `config/config.go` or the running process if `8080` doesn't match.)

- [ ] **Step 6: Verify the empty scaffold builds and tests clean**

```bash
cd frontend
npm run build
npm test -- --watch=false --browsers=ChromeHeadless
```

Expected: both commands exit 0. `npm run build` produces `frontend/dist/`. `npm test` runs the CLI's default `app.component.spec.ts` and passes.

- [ ] **Step 7: Commit**

```bash
git add frontend/
git commit -m "chore: scaffold Angular 19 standalone frontend project"
```

(This is a large commit — the entire generated project. That's expected and correct for a scaffolding task; do not try to hand-pick files out of a `ng new` output.)

---

## Chapter 1 — Custom Flow: Login / Session Guard

**Category:** Custom Flow
**AC covered:** AC1.1, AC1.2, AC1.3, AC1.4, AC1.5, AC1.6

This chapter also includes two additions beyond the spec's own Task Breakdown for this chapter, found during plan authoring (documented in the File Structure section above): **Task 1.4 (Dashboard Shell + Nav)** fills a gap — AC1.5 and the UI Flowchart require a shell+nav component that no chapter's Task Breakdown creates — and **Task 1.5 (global 401 interceptor)** implements the spec's Notes & Recommendations **C4**, which explicitly says to build it "sebagai bagian Chapter 1 saat eksekusi plan, tidak perlu chapter terpisah."

A note on the `?error=` query param (AC1.3, Open Question #1): the Go backend does not yet redirect to `{BasePath}/?error=<message>` on OAuth failure — it still renders the old `login.html` directly (`auth_handler_impl.go`'s `renderLoginPage`). Wiring that redirect is Plan C's job (the same commit that deletes `login.html` and wires the catch-all shell route) — this chapter builds `LoginComponent` to correctly read and display `?error=` now, so Plan C's backend change is a drop-in with no Angular-side follow-up needed.

### Task 1.1 — Interface + Logic: Login component

**Depends on:** Task 0
**Files:**
- Create: `frontend/src/app/features/login/login.component.ts`
- Create: `frontend/src/app/features/login/login.component.html`

- [ ] Create `frontend/src/app/features/login/login.component.ts`:

```typescript
import { Component, inject } from '@angular/core';
import { NgIf } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { environment } from '../../../environments/environment';

@Component({
  selector: 'app-login',
  standalone: true,
  imports: [NgIf],
  templateUrl: './login.component.html',
})
export class LoginComponent {
  private route = inject(ActivatedRoute);

  protected errorMessage = this.route.snapshot.queryParamMap.get('error');
  protected googleLoginHref = `${environment.apiBasePath}/auth/google/login`;
}
```

- [ ] Create `frontend/src/app/features/login/login.component.html`:

```html
<div class="login-page">
  <div class="login-card">
    <div class="brand-badge">🤖</div>
    <h1>Jira U Bot Dashboard</h1>
    <p class="subtitle">Login pakai akun Google kantor (@esb.co.id) untuk lihat tiket, atur reminder, dan lainnya.</p>

    <div class="alert alert-error" *ngIf="errorMessage">{{ errorMessage }}</div>

    <a class="google-signin-btn" [href]="googleLoginHref">Sign in with Google</a>

    <p class="footer-note">Khusus email @esb.co.id</p>
  </div>
</div>
```

### Task 1.u — UI Test: Login component

**Depends on:** Task 1.1
**Files:**
- Create: `frontend/src/app/features/login/login.component.spec.ts`

- [ ] Create `frontend/src/app/features/login/login.component.spec.ts`:

```typescript
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { LoginComponent } from './login.component';

async function setup(queryParams: Record<string, string>) {
  await TestBed.configureTestingModule({
    imports: [LoginComponent],
    providers: [
      {
        provide: ActivatedRoute,
        useValue: { snapshot: { queryParamMap: convertToParamMap(queryParams) } },
      },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(LoginComponent);
  fixture.detectChanges();
  return fixture;
}

describe('LoginComponent', () => {
  it('does not show an error alert when no error query param is present', async () => {
    const fixture = await setup({});
    expect(fixture.nativeElement.querySelector('.alert-error')).toBeNull();
  });

  it('shows the error message verbatim when an error query param is present', async () => {
    const fixture = await setup({ error: 'Sesi login tidak valid atau kedaluwarsa, coba lagi' });
    const alert = fixture.nativeElement.querySelector('.alert-error');
    expect(alert?.textContent).toContain('Sesi login tidak valid atau kedaluwarsa, coba lagi');
  });

  it('points the Sign in with Google link at /jira-notif/auth/google/login', async () => {
    const fixture = await setup({});
    const link: HTMLAnchorElement = fixture.nativeElement.querySelector('.google-signin-btn');
    expect(link.getAttribute('href')).toBe('/jira-notif/auth/google/login');
  });

  it('renders the Sign in button as a plain anchor element, not a button with a click handler', async () => {
    const fixture = await setup({});
    const link = fixture.nativeElement.querySelector('.google-signin-btn');
    expect(link.tagName).toBe('A');
  });
});
```

- [ ] Run: `cd frontend && npm test -- --watch=false --browsers=ChromeHeadless --include='**/login.component.spec.ts'`
Expected: 4 tests pass.

### Task 1.2 — State + Integration: `AuthService` + `AuthGuard`

**Depends on:** Task 0
**Files:**
- Create: `frontend/src/app/core/services/auth.service.ts`
- Create: `frontend/src/app/core/guards/auth.guard.ts`

**Interfaces:**
- Produces: `AuthService.currentUser$: BehaviorSubject<{email: string, isAdmin: boolean} | null>` (default `null`), `AuthService.checked: boolean` (default `false`), `AuthService.checkSession(): Observable<{email, isAdmin}>`, `AuthService.clearSession(): void` — every later chapter's guard/interceptor/shell reads `currentUser$`/`checked` by these exact names.

- [ ] Create `frontend/src/app/core/services/auth.service.ts`:

```typescript
import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { BehaviorSubject, Observable, tap } from 'rxjs';
import { environment } from '../../../environments/environment';

export interface CurrentUser {
  email: string;
  isAdmin: boolean;
}

@Injectable({ providedIn: 'root' })
export class AuthService {
  private http = inject(HttpClient);

  readonly currentUser$ = new BehaviorSubject<CurrentUser | null>(null);
  checked = false;

  checkSession(): Observable<CurrentUser> {
    return this.http.get<CurrentUser>(`${environment.apiBasePath}/api/auth/me`).pipe(
      tap((user) => {
        this.currentUser$.next(user);
        this.checked = true;
      }),
    );
  }

  clearSession(): void {
    this.currentUser$.next(null);
    this.checked = true;
  }
}
```

- [ ] Create `frontend/src/app/core/guards/auth.guard.ts`:

```typescript
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
```

### Task 1.i — Integration Test: `AuthGuard`

**Depends on:** Task 1.2
**Files:**
- Create: `frontend/src/app/core/guards/auth.guard.spec.ts`

- [ ] Create `frontend/src/app/core/guards/auth.guard.spec.ts`:

```typescript
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
```

- [ ] Run: `cd frontend && npm test -- --watch=false --browsers=ChromeHeadless --include='**/auth.guard.spec.ts'`
Expected: 4 tests pass.

### Task 1.3 — Routing: `app.routes.ts`

**Depends on:** Task 1.1, Task 1.2, Task 1.4
**Files:**
- Modify: `frontend/src/app/app.routes.ts`

**Interfaces:**
- Produces: the `routes` array's shape — `dashboard` is a parent route guarded by `AuthGuard` with a `children: []` array that Chapters 4, 5, 6, 7a, 9 each append exactly one entry to, in that order. Any later chapter that edits this file must find the children array **exactly as the immediately preceding chapter left it** — do not assume a different starting state.

- [ ] Replace the full contents of `frontend/src/app/app.routes.ts` (generated by `ng new` as an empty `export const routes: Routes = [];`):

```typescript
import { Routes } from '@angular/router';
import { LoginComponent } from './features/login/login.component';
import { DashboardShellComponent } from './layouts/dashboard-shell/dashboard-shell.component';
import { AuthGuard } from './core/guards/auth.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [],
  },
  { path: '**', redirectTo: '' },
];
```

(`children: []` is intentional and temporary — Chapter 4 (My Board) adds the first child route. Until then, `/dashboard` renders the shell with an empty content area, which is an expected intermediate state for this plan, same as Plan A's endpoints existing before their router wiring landed.)

- [ ] Run: `cd frontend && npm run build`
Expected: builds clean (compile-only check — there's no routing behavior to unit-test yet since `DashboardShellComponent` doesn't exist until Task 1.4).

### Task 1.4 — Interface + Logic: Dashboard Shell + Nav (gap-fill, see Chapter note)

**Depends on:** Task 1.2
**Files:**
- Create: `frontend/src/app/layouts/dashboard-shell/dashboard-shell.component.ts`
- Create: `frontend/src/app/layouts/dashboard-shell/dashboard-shell.component.html`

- [ ] Create `frontend/src/app/layouts/dashboard-shell/dashboard-shell.component.ts`:

```typescript
import { Component, inject } from '@angular/core';
import { AsyncPipe, NgIf } from '@angular/common';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { AuthService } from '../../core/services/auth.service';
import { environment } from '../../../environments/environment';

@Component({
  selector: 'app-dashboard-shell',
  standalone: true,
  imports: [RouterOutlet, RouterLink, RouterLinkActive, AsyncPipe, NgIf],
  templateUrl: './dashboard-shell.component.html',
})
export class DashboardShellComponent {
  protected authService = inject(AuthService);
  protected logoutHref = `${environment.apiBasePath}/auth/logout`;
}
```

- [ ] Create `frontend/src/app/layouts/dashboard-shell/dashboard-shell.component.html`:

```html
<header class="shell-header">
  <span class="brand">🤖 Jira U Bot Dashboard</span>
  <nav>
    <span>{{ (authService.currentUser$ | async)?.email }}</span>
    <a routerLink="/dashboard" routerLinkActive="active" [routerLinkActiveOptions]="{exact: true}">My Board</a>
    <a routerLink="/dashboard/team" routerLinkActive="active">Team</a>
    <a routerLink="/dashboard/overdue" routerLinkActive="active">Overdue</a>
    <a *ngIf="(authService.currentUser$ | async)?.isAdmin" routerLink="/dashboard/admin" routerLinkActive="active">Admin</a>
    <a routerLink="/dashboard/report" routerLinkActive="active">Sprint Report</a>
    <a [href]="logoutHref">Logout</a>
  </nav>
</header>
<main>
  <router-outlet></router-outlet>
</main>
```

(Nav links to `/dashboard/admin` and `/dashboard/report` are written now even though `AdminComponent`/`ReportComponent` don't have routes yet — they'll 404 inside the router until Chapters 7a/9 add their route entries. This is fine: nothing links to them yet except this nav, and the "Admin" link is additionally gated behind `isAdmin`, so no end-to-end breakage is reachable before Chapter 7a lands.)

- [ ] Run: `cd frontend && npm run build`
Expected: builds clean.

### Task 1.5 — State + Integration: global 401 interceptor (spec Notes C4)

**Depends on:** Task 1.2
**Files:**
- Create: `frontend/src/app/core/interceptors/auth.interceptor.ts`
- Modify: `frontend/src/app/app.config.ts`

- [ ] Create `frontend/src/app/core/interceptors/auth.interceptor.ts`:

```typescript
import { HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { Router } from '@angular/router';
import { catchError, throwError } from 'rxjs';
import { AuthService } from '../services/auth.service';

export const authInterceptor: HttpInterceptorFn = (req, next) => {
  const router = inject(Router);
  const authService = inject(AuthService);

  return next(req).pipe(
    catchError((error) => {
      if (error?.status === 401) {
        authService.clearSession();
        router.navigate(['/']);
      }
      return throwError(() => error);
    }),
  );
};
```

- [ ] In `frontend/src/app/app.config.ts`, find the `provideHttpClient()` call (added by `ng new --routing`, likely already present, or add it to the `providers` array if not) and wire the interceptor:

```typescript
import { ApplicationConfig } from '@angular/core';
import { provideRouter } from '@angular/router';
import { provideHttpClient, withInterceptors } from '@angular/common/http';
import { routes } from './app.routes';
import { authInterceptor } from './core/interceptors/auth.interceptor';

export const appConfig: ApplicationConfig = {
  providers: [provideRouter(routes), provideHttpClient(withInterceptors([authInterceptor]))],
};
```

(If `ng new` generated a different existing `providers` array, keep every other entry in it — this only adds/updates the `provideHttpClient(...)` line.)

### Task 1.i2 — Integration Test: auth interceptor

**Depends on:** Task 1.5
**Files:**
- Create: `frontend/src/app/core/interceptors/auth.interceptor.spec.ts`

- [ ] Create `frontend/src/app/core/interceptors/auth.interceptor.spec.ts`:

```typescript
import { TestBed } from '@angular/core/testing';
import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router } from '@angular/router';
import { authInterceptor } from './auth.interceptor';
import { AuthService } from '../services/auth.service';

describe('authInterceptor', () => {
  let http: HttpClient;
  let httpMock: HttpTestingController;
  let authService: jasmine.SpyObj<AuthService>;
  let router: jasmine.SpyObj<Router>;

  beforeEach(() => {
    authService = jasmine.createSpyObj('AuthService', ['clearSession']);
    router = jasmine.createSpyObj('Router', ['navigate']);

    TestBed.configureTestingModule({
      providers: [
        provideHttpClient(withInterceptors([authInterceptor])),
        provideHttpClientTesting(),
        { provide: AuthService, useValue: authService },
        { provide: Router, useValue: router },
      ],
    });

    http = TestBed.inject(HttpClient);
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => httpMock.verify());

  it('clears the session and redirects to Login on a 401 response from any request', () => {
    http.get('/jira-notif/api/dashboard/board').subscribe({ error: () => {} });

    httpMock.expectOne('/jira-notif/api/dashboard/board').flush({ error: 'unauthorized' }, { status: 401, statusText: 'Unauthorized' });

    expect(authService.clearSession).toHaveBeenCalled();
    expect(router.navigate).toHaveBeenCalledWith(['/']);
  });

  it('does not touch the session or redirect on a non-401 error', () => {
    http.get('/jira-notif/api/dashboard/board').subscribe({ error: () => {} });

    httpMock.expectOne('/jira-notif/api/dashboard/board').flush({ error: 'failed to load board' }, { status: 500, statusText: 'Server Error' });

    expect(authService.clearSession).not.toHaveBeenCalled();
    expect(router.navigate).not.toHaveBeenCalled();
  });
});
```

- [ ] Run: `cd frontend && npm test -- --watch=false --browsers=ChromeHeadless --include='**/auth.interceptor.spec.ts'`
Expected: 2 tests pass.

### Task 1.x — Dev Test

**Depends on:** Task 1.1, Task 1.2, Task 1.3, Task 1.4, Task 1.5
- [ ] `cd frontend && npm run build && npm test -- --watch=false --browsers=ChromeHeadless` — full suite green.
- [ ] `ng serve --proxy-config proxy.conf.json` against a locally running `jira-u-bot` binary (real session cookie, real OAuth). Self-test mandiri: buka `http://localhost:4200/jira-notif/` tanpa login → Login Screen tampil (bukan 404/blank). Login via Google → redirect balik → cookie `session_token` ter-set → navigasi manual ke `http://localhost:4200/jira-notif/dashboard` → Dashboard Shell + Nav tampil (area tengah masih kosong, itu normal sampai Chapter 4).

### Task 1.exec — Commit

**Depends on:** Task 1.x

```bash
cd frontend
git add src/app/features/login/ src/app/core/services/auth.service.ts src/app/core/guards/auth.guard.ts src/app/core/guards/auth.guard.spec.ts src/app/core/interceptors/ src/app/layouts/dashboard-shell/ src/app/app.routes.ts src/app/app.config.ts
git commit -m "feat: add Login screen, AuthGuard, Dashboard Shell, and global 401 interceptor"
```

---

## Chapter 2 — Update: Reminder Preference

**Category:** Update
**AC covered:** AC5.1, AC5.2

**Interfaces:**
- Produces: `BoardService` (new file, `frontend/src/app/core/services/board.service.ts`) with its first method, `updatePreference(tiers: string[]): Observable<{status: string}>`, calling `POST /api/dashboard/preference`. Chapters 3-6 each add more methods to this same file — see each chapter's "Produces" note for the exact growth.

### Task 2.1 — Interface + Logic: form 6 checkbox, pre-check dari data board

**Depends on:** Task 1 (uses `environment`, no other Chapter 1 dependency)
**Files:**
- Create: `frontend/src/app/core/services/board.service.ts`
- Create: `frontend/src/app/features/dashboard/board/reminder-preference.component.ts`
- Create: `frontend/src/app/features/dashboard/board/reminder-preference.component.html`

- [ ] Create `frontend/src/app/core/services/board.service.ts`:

```typescript
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
```

- [ ] Create `frontend/src/app/features/dashboard/board/reminder-preference.component.ts`:

```typescript
import { Component, OnInit, inject, input } from '@angular/core';
import { FormBuilder, FormGroup, ReactiveFormsModule } from '@angular/forms';
import { NgFor } from '@angular/common';
import { BoardService } from '../../../core/services/board.service';

interface TierOption {
  code: string;
  label: string;
}

const TIER_OPTIONS: TierOption[] = [
  { code: '2h', label: 'T-2 jam' },
  { code: '1h', label: 'T-1 jam' },
  { code: '30m', label: 'T-30 menit' },
  { code: '15m', label: 'T-15 menit' },
  { code: '5m', label: 'T-5 menit' },
  { code: '1m', label: 'T-1 menit' },
];

@Component({
  selector: 'app-reminder-preference',
  standalone: true,
  imports: [ReactiveFormsModule, NgFor],
  templateUrl: './reminder-preference.component.html',
})
export class ReminderPreferenceComponent implements OnInit {
  tiers = input<string[]>([]);

  protected tierOptions = TIER_OPTIONS;
  protected submitting = false;
  protected message: { text: string; isError: boolean } | null = null;
  protected form!: FormGroup;

  private fb = inject(FormBuilder);
  private boardService = inject(BoardService);

  ngOnInit(): void {
    this.form = this.fb.group(
      Object.fromEntries(TIER_OPTIONS.map((opt) => [opt.code, this.tiers().includes(opt.code)])),
    );
  }

  protected onSubmit(): void {
    this.submitting = true;
    const selected = TIER_OPTIONS.filter((opt) => this.form.value[opt.code]).map((opt) => opt.code);

    this.boardService.updatePreference(selected).subscribe({
      next: () => {
        this.submitting = false;
        this.message = { text: 'Preference tersimpan ✅', isError: false };
      },
      error: (err) => {
        this.submitting = false;
        this.message = { text: err?.error?.error ?? 'Gagal menyimpan', isError: true };
      },
    });
  }
}
```

- [ ] Create `frontend/src/app/features/dashboard/board/reminder-preference.component.html`:

```html
<form [formGroup]="form" (ngSubmit)="onSubmit()">
  <div class="tiers">
    <label *ngFor="let opt of tierOptions">
      <input type="checkbox" [formControlName]="opt.code" /> {{ opt.label }}
    </label>
  </div>
  <button type="submit" [disabled]="submitting">Simpan Preference</button>
</form>
<p class="msg" [class.success]="message && !message.isError" [class.error]="message?.isError">{{ message?.text }}</p>
```

### Task 2.2 — State + Integration: submit ke `POST /api/dashboard/preference`

Already implemented as part of Task 2.1 (`onSubmit()` above) — the spec's Task Breakdown lists this as a separate row for SP accounting, but `reminder-preference.component.ts`'s logic and its API call are one cohesive unit (same file, same method), so there is nothing further to do here. Proceed to Task 2.u.

### Task 2.u / 2.i — UI + Integration Test: checkbox pre-check, submit success/error

**Depends on:** Task 2.1
**Files:**
- Create: `frontend/src/app/features/dashboard/board/reminder-preference.component.spec.ts`

- [ ] Create `frontend/src/app/features/dashboard/board/reminder-preference.component.spec.ts`:

```typescript
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { ReminderPreferenceComponent } from './reminder-preference.component';
import { BoardService } from '../../../core/services/board.service';

describe('ReminderPreferenceComponent', () => {
  let fixture: ComponentFixture<ReminderPreferenceComponent>;
  let boardService: jasmine.SpyObj<BoardService>;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', ['updatePreference']);

    await TestBed.configureTestingModule({
      imports: [ReminderPreferenceComponent],
      providers: [{ provide: BoardService, useValue: boardService }],
    }).compileComponents();

    fixture = TestBed.createComponent(ReminderPreferenceComponent);
    fixture.componentRef.setInput('tiers', ['2h', '1m']);
    fixture.detectChanges();
  });

  it('pre-checks only the checkboxes matching the tiers input', () => {
    expect(fixture.componentInstance['form'].value).toEqual({
      '2h': true, '1h': false, '30m': false, '15m': false, '5m': false, '1m': true,
    });
    const checkboxes: HTMLInputElement[] = Array.from(fixture.nativeElement.querySelectorAll('input[type=checkbox]'));
    expect(checkboxes.map((cb) => cb.checked)).toEqual([true, false, false, false, false, true]);
  });

  it('submits only the checked tier codes and shows a success message', () => {
    boardService.updatePreference.and.returnValue(of({ status: 'ok' }));

    fixture.componentInstance['form'].patchValue({ '2h': false });
    fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit'));
    fixture.detectChanges();

    expect(boardService.updatePreference).toHaveBeenCalledWith(['1m']);
    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('Preference tersimpan');
  });

  it('submits an empty array when every tier is unchecked (AC5.1 allows zero selected)', () => {
    boardService.updatePreference.and.returnValue(of({ status: 'ok' }));

    fixture.componentInstance['form'].patchValue({ '2h': false, '1m': false });
    fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit'));

    expect(boardService.updatePreference).toHaveBeenCalledWith([]);
  });

  it('shows an error message and keeps the checkbox state when the save fails', () => {
    boardService.updatePreference.and.returnValue(throwError(() => ({ error: { error: 'failed to save preference' } })));

    fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit'));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('failed to save preference');
    expect(fixture.componentInstance['form'].value['2h']).toBe(true);
  });
});
```

- [ ] Run: `cd frontend && npm test -- --watch=false --browsers=ChromeHeadless --include='**/reminder-preference.component.spec.ts'`
Expected: 4 tests pass.

### Task 2.x — Dev Test

**Depends on:** Task 2.u/2.i
- [ ] `cd frontend && npm run build && npm test -- --watch=false --browsers=ChromeHeadless` — full suite green (this component isn't mounted anywhere yet, so there's no browser self-test until Chapter 4 embeds it into `BoardComponent`; the build+test pass is this task's complete verification).

### Task 2.exec — Commit

```bash
cd frontend
git add src/app/core/services/board.service.ts src/app/features/dashboard/board/reminder-preference.component.ts src/app/features/dashboard/board/reminder-preference.component.html src/app/features/dashboard/board/reminder-preference.component.spec.ts
git commit -m "feat: add Reminder Preference component and BoardService.updatePreference"
```

---

## Chapter 3 — Create: Jira API Key Pribadi

**Category:** Create
**AC covered:** AC6.1, AC6.2

**Interfaces:**
- Consumes: `BoardService` (from Chapter 2).
- Produces: adds `saveCredential(jiraEmail: string, apiToken: string): Observable<{status: string}>` and `deleteCredential(): Observable<{status: string}>` to `board.service.ts`.

### Task 3.1 — Interface + Logic: form + tampilan Aktif, toggle berdasarkan status

**Depends on:** Task 2.1
**Files:**
- Modify: `frontend/src/app/core/services/board.service.ts`
- Create: `frontend/src/app/features/dashboard/board/jira-credential.component.ts`
- Create: `frontend/src/app/features/dashboard/board/jira-credential.component.html`

- [ ] In `frontend/src/app/core/services/board.service.ts`, find:

```typescript
  updatePreference(tiers: string[]): Observable<{ status: string }> {
    return this.http.post<{ status: string }>(`${environment.apiBasePath}/api/dashboard/preference`, { tiers });
  }
}
```

Replace with:

```typescript
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
```

- [ ] Create `frontend/src/app/features/dashboard/board/jira-credential.component.ts`:

```typescript
import { Component, OnInit, inject, input, output } from '@angular/core';
import { FormBuilder, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { NgIf } from '@angular/common';
import { BoardService } from '../../../core/services/board.service';

@Component({
  selector: 'app-jira-credential',
  standalone: true,
  imports: [ReactiveFormsModule, NgIf],
  templateUrl: './jira-credential.component.html',
})
export class JiraCredentialComponent implements OnInit {
  hasCredential = input<boolean>(false);
  credentialMasked = input<string | null>(null);
  credentialChanged = output<void>();

  protected form!: FormGroup;
  protected submitting = false;
  protected error: string | null = null;

  private fb = inject(FormBuilder);
  private boardService = inject(BoardService);

  ngOnInit(): void {
    this.form = this.fb.group({
      jiraEmail: ['', [Validators.required, Validators.email]],
      apiToken: ['', Validators.required],
    });
  }

  protected onSubmit(): void {
    if (this.form.invalid || this.submitting) {
      return;
    }
    this.submitting = true;
    this.error = null;

    this.boardService.saveCredential(this.form.value.jiraEmail, this.form.value.apiToken).subscribe({
      next: () => {
        this.submitting = false;
        this.credentialChanged.emit();
      },
      error: (err) => {
        this.submitting = false;
        this.error = err?.status === 503 ? 'fitur ini belum dikonfigurasi server, hubungi admin' : 'failed to save credential';
      },
    });
  }

  protected onDelete(): void {
    this.boardService.deleteCredential().subscribe({
      next: () => this.credentialChanged.emit(),
      error: () => {
        this.error = 'failed to delete credential';
      },
    });
  }
}
```

- [ ] Create `frontend/src/app/features/dashboard/board/jira-credential.component.html`:

```html
<div *ngIf="!hasCredential()">
  <form [formGroup]="form" (ngSubmit)="onSubmit()" class="cred-form">
    <input type="email" formControlName="jiraEmail" placeholder="Email Jira kamu" />
    <div class="field-error" *ngIf="form.controls.jiraEmail.touched && form.controls.jiraEmail.invalid">Email tidak valid</div>
    <input type="password" formControlName="apiToken" placeholder="API Token" />
    <div class="field-error" *ngIf="form.controls.apiToken.touched && form.controls.apiToken.invalid">Token wajib diisi</div>
    <button type="submit" [disabled]="form.invalid || submitting">Simpan</button>
  </form>
  <p class="instructions">
    Belum punya token? Buat di
    <a href="https://id.atlassian.com/manage-profile/security/api-tokens" target="_blank" rel="noopener"
      >id.atlassian.com/manage-profile/security/api-tokens</a
    >
    → "Create API token" → copy ke sini.
  </p>
</div>
<div *ngIf="hasCredential()">
  <p class="cred-status">Aktif — token berakhiran <strong>{{ credentialMasked() }}</strong></p>
  <button class="secondary" (click)="onDelete()">Hapus Credential</button>
</div>
<p class="msg error" *ngIf="error">{{ error }}</p>
```

### Task 3.2 — State + Integration: `POST`/`DELETE /api/dashboard/credential`

Already implemented as part of Task 3.1 (`onSubmit()`/`onDelete()` above, and the two `BoardService` methods) — same reasoning as Task 2.2. Proceed to Task 3.u/3.i.

### Task 3.u / 3.i — UI + Integration Test: form/Aktif toggle, save/delete success/error

**Depends on:** Task 3.1
**Files:**
- Create: `frontend/src/app/features/dashboard/board/jira-credential.component.spec.ts`

- [ ] Create `frontend/src/app/features/dashboard/board/jira-credential.component.spec.ts`:

```typescript
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { JiraCredentialComponent } from './jira-credential.component';
import { BoardService } from '../../../core/services/board.service';

describe('JiraCredentialComponent', () => {
  let fixture: ComponentFixture<JiraCredentialComponent>;
  let boardService: jasmine.SpyObj<BoardService>;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', ['saveCredential', 'deleteCredential']);

    await TestBed.configureTestingModule({
      imports: [JiraCredentialComponent],
      providers: [{ provide: BoardService, useValue: boardService }],
    }).compileComponents();

    fixture = TestBed.createComponent(JiraCredentialComponent);
  });

  it('shows the form when there is no credential yet', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('form')).not.toBeNull();
    expect(fixture.nativeElement.querySelector('.cred-status')).toBeNull();
  });

  it('shows the masked credential and Hapus button when a credential already exists', () => {
    fixture.componentRef.setInput('hasCredential', true);
    fixture.componentRef.setInput('credentialMasked', '••••ab3f');
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('form')).toBeNull();
    expect(fixture.nativeElement.querySelector('.cred-status').textContent).toContain('••••ab3f');
  });

  it('emits credentialChanged after a successful save', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();
    boardService.saveCredential.and.returnValue(of({ status: 'ok' }));
    const emitted = jasmine.createSpy('emitted');
    fixture.componentInstance.credentialChanged.subscribe(emitted);

    fixture.componentInstance['form'].setValue({ jiraEmail: 'a@esb.co.id', apiToken: 'tok123' });
    fixture.componentInstance['onSubmit']();

    expect(boardService.saveCredential).toHaveBeenCalledWith('a@esb.co.id', 'tok123');
    expect(emitted).toHaveBeenCalled();
  });

  it('shows the encryption-not-configured message on a 503 response', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();
    boardService.saveCredential.and.returnValue(throwError(() => ({ status: 503 })));

    fixture.componentInstance['form'].setValue({ jiraEmail: 'a@esb.co.id', apiToken: 'tok123' });
    fixture.componentInstance['onSubmit']();
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('fitur ini belum dikonfigurasi server, hubungi admin');
  });

  it('shows a generic error message on any other save failure', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();
    boardService.saveCredential.and.returnValue(throwError(() => ({ status: 500 })));

    fixture.componentInstance['form'].setValue({ jiraEmail: 'a@esb.co.id', apiToken: 'tok123' });
    fixture.componentInstance['onSubmit']();
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('failed to save credential');
  });

  it('emits credentialChanged after a successful delete', () => {
    fixture.componentRef.setInput('hasCredential', true);
    fixture.componentRef.setInput('credentialMasked', '••••ab3f');
    fixture.detectChanges();
    boardService.deleteCredential.and.returnValue(of({ status: 'ok' }));
    const emitted = jasmine.createSpy('emitted');
    fixture.componentInstance.credentialChanged.subscribe(emitted);

    fixture.componentInstance['onDelete']();

    expect(emitted).toHaveBeenCalled();
  });
});
```

- [ ] Run: `cd frontend && npm test -- --watch=false --browsers=ChromeHeadless --include='**/jira-credential.component.spec.ts'`
Expected: 6 tests pass.

### Task 3.x — Dev Test

**Depends on:** Task 3.u/3.i
- [ ] `cd frontend && npm run build && npm test -- --watch=false --browsers=ChromeHeadless` — full suite green (not mounted anywhere yet — same note as Task 2.x, this component joins `BoardComponent` in Chapter 4).

### Task 3.exec — Commit

```bash
cd frontend
git add src/app/core/services/board.service.ts src/app/features/dashboard/board/jira-credential.component.ts src/app/features/dashboard/board/jira-credential.component.html src/app/features/dashboard/board/jira-credential.component.spec.ts
git commit -m "feat: add Jira Credential component and BoardService save/delete credential"
```

---

## Chapter 4 — List: My Board

**Category:** List
**AC covered:** AC2.1, AC2.2, AC2.3

**Interfaces:**
- Consumes: `ReminderPreferenceComponent` (Chapter 2), `JiraCredentialComponent` (Chapter 3), `BoardService` (growing again this chapter).
- Produces: adds `getBoard(): Observable<BoardResponse>` and `transitionTicket(issueKey, target): Observable<TransitionResponse>` to `board.service.ts`, plus the `Card`, `BoardColumn`, `BoardResponse`, `TransitionResponse` interfaces every later chapter's `Card`-shaped data (Team, Overdue) reuses by name.
- This chapter also makes the first edit to `app.routes.ts`'s `dashboard.children` array (currently `[]`, from Chapter 1 Task 1.3) — appends `{ path: '', component: BoardComponent }`.

### Task 4.1 — Interface + Logic: tabel board, badge status, dua tombol aksi per baris

**Depends on:** Task 2.1, Task 3.1
**Files:**
- Modify: `frontend/src/app/core/services/board.service.ts`
- Create: `frontend/src/app/features/dashboard/board/board.component.ts`
- Create: `frontend/src/app/features/dashboard/board/board.component.html`

- [ ] In `frontend/src/app/core/services/board.service.ts`, add these interfaces above the `@Injectable` class (after the imports):

```typescript
export interface Card {
  issueKey: string;
  summary: string;
  status: string;
  hasBudgetDetail: boolean;
  storyPoints: number;
  elapsedLabel: string;
  remainingLabel: string;
  deadlineLabel: string;
}

export interface BoardColumn {
  status: string;
  cards: Card[];
}

export interface BoardResponse {
  columns: BoardColumn[];
  tierPreference: { tiers: string[] };
  credential: { hasCredential: boolean; maskedPreview: string };
}

export interface TransitionResponse {
  message: string;
}
```

- [ ] In the same file, find the closing `}` of the `deleteCredential` method (the method added in Chapter 3, immediately before the class's final `}`):

```typescript
  deleteCredential(): Observable<{ status: string }> {
    return this.http.delete<{ status: string }>(`${environment.apiBasePath}/api/dashboard/credential`);
  }
}
```

Replace with:

```typescript
  deleteCredential(): Observable<{ status: string }> {
    return this.http.delete<{ status: string }>(`${environment.apiBasePath}/api/dashboard/credential`);
  }

  getBoard(): Observable<BoardResponse> {
    return this.http.get<BoardResponse>(`${environment.apiBasePath}/api/dashboard/board`);
  }

  transitionTicket(issueKey: string, target: 'todo' | 'code-review'): Observable<TransitionResponse> {
    return this.http.post<TransitionResponse>(
      `${environment.apiBasePath}/api/dashboard/tickets/${encodeURIComponent(issueKey)}/transition`,
      { target },
    );
  }
}
```

- [ ] Create `frontend/src/app/features/dashboard/board/board.component.ts`:

```typescript
import { Component, OnInit, inject } from '@angular/core';
import { NgFor, NgIf } from '@angular/common';
import { BoardService, Card } from '../../../core/services/board.service';
import { ReminderPreferenceComponent } from './reminder-preference.component';
import { JiraCredentialComponent } from './jira-credential.component';

@Component({
  selector: 'app-board',
  standalone: true,
  imports: [NgFor, NgIf, ReminderPreferenceComponent, JiraCredentialComponent],
  templateUrl: './board.component.html',
})
export class BoardComponent implements OnInit {
  protected cards: Card[] = [];
  protected loading = true;
  protected error: string | null = null;
  protected tiers: string[] = [];
  protected hasCredential = false;
  protected credentialMasked: string | null = null;
  protected transitioningIssueKey: string | null = null;
  protected actionMessage: { text: string; isError: boolean } | null = null;

  private boardService = inject(BoardService);

  ngOnInit(): void {
    this.loadBoard();
  }

  protected loadBoard(): void {
    this.loading = true;
    this.error = null;

    this.boardService.getBoard().subscribe({
      next: (res) => {
        this.cards = res.columns.flatMap((col) => col.cards);
        this.tiers = res.tierPreference.tiers;
        this.hasCredential = res.credential.hasCredential;
        this.credentialMasked = res.credential.maskedPreview;
        this.loading = false;
      },
      error: () => {
        this.error = 'Gagal memuat board, coba lagi';
        this.loading = false;
      },
    });
  }

  protected onTransition(issueKey: string, target: 'todo' | 'code-review'): void {
    this.transitioningIssueKey = issueKey;

    this.boardService.transitionTicket(issueKey, target).subscribe({
      next: (res) => {
        this.actionMessage = { text: res.message, isError: false };
        this.transitioningIssueKey = null;
        this.loadBoard();
      },
      error: (err) => {
        this.actionMessage = { text: err?.error?.error ?? 'Gagal memindahkan tiket', isError: true };
        this.transitioningIssueKey = null;
      },
    });
  }

  protected onCredentialChanged(): void {
    this.loadBoard();
  }
}
```

- [ ] Create `frontend/src/app/features/dashboard/board/board.component.html`:

```html
<app-reminder-preference [tiers]="tiers"></app-reminder-preference>
<app-jira-credential
  [hasCredential]="hasCredential"
  [credentialMasked]="credentialMasked"
  (credentialChanged)="onCredentialChanged()"
></app-jira-credential>

<section>
  <h2>My Board</h2>
  <p *ngIf="loading">Memuat...</p>
  <p *ngIf="error" class="msg error">{{ error }}</p>
  <div class="table-wrap" *ngIf="!loading && !error">
    <table class="board">
      <thead>
        <tr><th>Status</th><th>Issue Key</th><th>Summary</th><th>SP</th><th>Elapsed</th><th>Sisa</th><th>Aksi</th></tr>
      </thead>
      <tbody>
        <tr *ngFor="let card of cards" [attr.data-issue-key]="card.issueKey">
          <td><span class="status-badge">{{ card.status }}</span></td>
          <td class="key">{{ card.issueKey }}</td>
          <td>{{ card.summary }}</td>
          <ng-container *ngIf="card.hasBudgetDetail; else noBudget">
            <td class="budget">{{ card.storyPoints }}</td>
            <td class="budget">{{ card.elapsedLabel }}</td>
            <td class="budget">{{ card.remainingLabel }}</td>
          </ng-container>
          <ng-template #noBudget>
            <td class="budget">—</td>
            <td class="budget">—</td>
            <td class="budget">—</td>
          </ng-template>
          <td class="actions">
            <button [disabled]="transitioningIssueKey === card.issueKey" (click)="onTransition(card.issueKey, 'todo')">→ Todo</button>
            <button [disabled]="transitioningIssueKey === card.issueKey" (click)="onTransition(card.issueKey, 'code-review')">→ Code Review</button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
  <p class="msg" [class.success]="actionMessage && !actionMessage.isError" [class.error]="actionMessage?.isError">{{ actionMessage?.text }}</p>
</section>
```

### Task 4.2 — State + Integration: `GET /api/dashboard/board`, `POST .../transition`, reload setelah aksi

Already implemented as part of Task 4.1 (`loadBoard()`/`onTransition()` above) — same reasoning as Tasks 2.2/3.2. Proceed to Task 4.u/4.i.

### Task 4.u / 4.i — UI + Integration Test: tabel render, badge, aksi, GET/POST sukses/gagal

**Depends on:** Task 4.1
**Files:**
- Create: `frontend/src/app/features/dashboard/board/board.component.spec.ts`

- [ ] Create `frontend/src/app/features/dashboard/board/board.component.spec.ts`:

```typescript
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { BoardComponent } from './board.component';
import { BoardService, BoardResponse } from '../../../core/services/board.service';

const SAMPLE_BOARD: BoardResponse = {
  columns: [
    {
      status: 'Code Process',
      cards: [
        { issueKey: 'OMS-1', summary: 'Fix bug', status: 'Code Process', hasBudgetDetail: true, storyPoints: 3, elapsedLabel: '2j', remainingLabel: '4j', deadlineLabel: '' },
      ],
    },
    {
      status: 'Todo',
      cards: [
        { issueKey: 'OMS-2', summary: 'New feature', status: 'Todo', hasBudgetDetail: false, storyPoints: 0, elapsedLabel: '', remainingLabel: '', deadlineLabel: '' },
      ],
    },
  ],
  tierPreference: { tiers: ['2h', '1m'] },
  credential: { hasCredential: false, maskedPreview: '' },
};

describe('BoardComponent', () => {
  let fixture: ComponentFixture<BoardComponent>;
  let boardService: jasmine.SpyObj<BoardService>;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', [
      'getBoard', 'updatePreference', 'saveCredential', 'deleteCredential', 'transitionTicket',
    ]);
    boardService.getBoard.and.returnValue(of(SAMPLE_BOARD));

    await TestBed.configureTestingModule({
      imports: [BoardComponent],
      providers: [{ provide: BoardService, useValue: boardService }],
    }).compileComponents();

    fixture = TestBed.createComponent(BoardComponent);
    fixture.detectChanges();
  });

  it('loads the board on init and renders one row per card, flattened across columns in order', () => {
    expect(boardService.getBoard).toHaveBeenCalled();
    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    expect(rows.length).toBe(2);
    expect(rows[0].getAttribute('data-issue-key')).toBe('OMS-1');
    expect(rows[1].getAttribute('data-issue-key')).toBe('OMS-2');
  });

  it('shows budget columns for hasBudgetDetail cards and dashes otherwise', () => {
    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    const firstRowBudgetCells = rows[0].querySelectorAll('.budget');
    expect(Array.from(firstRowBudgetCells).map((td: any) => td.textContent.trim())).toEqual(['3', '2j', '4j']);

    const secondRowBudgetCells = rows[1].querySelectorAll('.budget');
    expect(Array.from(secondRowBudgetCells).map((td: any) => td.textContent.trim())).toEqual(['—', '—', '—']);
  });

  it('shows an error message when loading the board fails', () => {
    boardService.getBoard.and.returnValue(throwError(() => ({ status: 500 })));
    fixture = TestBed.createComponent(BoardComponent);
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal memuat board, coba lagi');
  });

  it('transitions a ticket and reloads the board on success', () => {
    boardService.transitionTicket.and.returnValue(of({ message: 'OMS-1 dipindahkan ke Todo' }));

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    const todoButton: HTMLButtonElement = rows[0].querySelectorAll('.actions button')[0];
    todoButton.click();

    expect(boardService.transitionTicket).toHaveBeenCalledWith('OMS-1', 'todo');
    expect(boardService.getBoard).toHaveBeenCalledTimes(2);
  });

  it('shows an error message and re-enables the button when a transition fails', () => {
    boardService.transitionTicket.and.returnValue(throwError(() => ({ error: { error: 'Tiket tidak bisa dipindahkan' } })));

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    const todoButton: HTMLButtonElement = rows[0].querySelectorAll('.actions button')[0];
    todoButton.click();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Tiket tidak bisa dipindahkan');
    expect(todoButton.disabled).toBe(false);
  });
});
```

- [ ] Run: `cd frontend && npm test -- --watch=false --browsers=ChromeHeadless --include='**/board.component.spec.ts'`
Expected: 5 tests pass.

### Task 4.routing — Wire `BoardComponent` into `app.routes.ts`

**Depends on:** Task 4.1
**Files:**
- Modify: `frontend/src/app/app.routes.ts`

- [ ] Find:

```typescript
import { Routes } from '@angular/router';
import { LoginComponent } from './features/login/login.component';
import { DashboardShellComponent } from './layouts/dashboard-shell/dashboard-shell.component';
import { AuthGuard } from './core/guards/auth.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [],
  },
  { path: '**', redirectTo: '' },
];
```

Replace with:

```typescript
import { Routes } from '@angular/router';
import { LoginComponent } from './features/login/login.component';
import { DashboardShellComponent } from './layouts/dashboard-shell/dashboard-shell.component';
import { BoardComponent } from './features/dashboard/board/board.component';
import { AuthGuard } from './core/guards/auth.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [{ path: '', component: BoardComponent }],
  },
  { path: '**', redirectTo: '' },
];
```

- [ ] Run: `cd frontend && npm run build`
Expected: builds clean.

### Task 4.x — Dev Test

**Depends on:** Task 4.u/4.i, Task 4.routing
- [ ] `cd frontend && npm run build && npm test -- --watch=false --browsers=ChromeHeadless` — full suite green.
- [ ] `ng serve --proxy-config proxy.conf.json` against a real running backend with real session data. Self-test mandiri di browser, bandingkan dengan tampilan `dashboard.html` lama: login → `/jira-notif/dashboard` → tabel My Board tampil dengan kolom yang sama, checkbox preference pre-checked sesuai data tersimpan, form/Aktif credential sesuai status, klik "→ Todo"/"→ Code Review" pada satu tiket nyata → tiket benar-benar berpindah status di Jira dan tabel reload.

### Task 4.exec — Commit

```bash
cd frontend
git add src/app/core/services/board.service.ts src/app/features/dashboard/board/board.component.ts src/app/features/dashboard/board/board.component.html src/app/features/dashboard/board/board.component.spec.ts src/app/app.routes.ts
git commit -m "feat: add My Board component (table, reminder preference, credential, transitions)"
```

---

## Chapter 5 — List: Team

**Category:** List
**AC covered:** AC3.1

**Interfaces:**
- Produces: adds `getTeam(): Observable<TeamResponse>` plus `TeamGroup`/`TeamResponse` interfaces to `board.service.ts`.
- This chapter appends the second entry to `app.routes.ts`'s `dashboard.children` array — `{ path: 'team', component: TeamComponent }`.

### Task 5.1 — Interface + Logic: grup per assignee, empty state

**Depends on:** Task 4.1
**Files:**
- Modify: `frontend/src/app/core/services/board.service.ts`
- Create: `frontend/src/app/features/dashboard/team/team.component.ts`
- Create: `frontend/src/app/features/dashboard/team/team.component.html`

- [ ] In `frontend/src/app/core/services/board.service.ts`, add this interface after `BoardResponse` (before `TransitionResponse`):

```typescript
export interface TeamGroup {
  assigneeEmail: string;
  cards: Card[];
}

export interface TeamResponse {
  groups: TeamGroup[];
}
```

- [ ] In the same file, find the `transitionTicket` method (the last method, immediately before the class's final `}`):

```typescript
  transitionTicket(issueKey: string, target: 'todo' | 'code-review'): Observable<TransitionResponse> {
    return this.http.post<TransitionResponse>(
      `${environment.apiBasePath}/api/dashboard/tickets/${encodeURIComponent(issueKey)}/transition`,
      { target },
    );
  }
}
```

Replace with:

```typescript
  transitionTicket(issueKey: string, target: 'todo' | 'code-review'): Observable<TransitionResponse> {
    return this.http.post<TransitionResponse>(
      `${environment.apiBasePath}/api/dashboard/tickets/${encodeURIComponent(issueKey)}/transition`,
      { target },
    );
  }

  getTeam(): Observable<TeamResponse> {
    return this.http.get<TeamResponse>(`${environment.apiBasePath}/api/dashboard/team`);
  }
}
```

- [ ] Create `frontend/src/app/features/dashboard/team/team.component.ts`:

```typescript
import { Component, OnInit, inject } from '@angular/core';
import { NgFor, NgIf } from '@angular/common';
import { BoardService, TeamGroup } from '../../../core/services/board.service';

@Component({
  selector: 'app-team',
  standalone: true,
  imports: [NgFor, NgIf],
  templateUrl: './team.component.html',
})
export class TeamComponent implements OnInit {
  protected groups: TeamGroup[] = [];
  protected loading = true;
  protected error: string | null = null;

  private boardService = inject(BoardService);

  ngOnInit(): void {
    this.boardService.getTeam().subscribe({
      next: (res) => {
        this.groups = res.groups;
        this.loading = false;
      },
      error: () => {
        this.error = 'Gagal memuat data team, coba lagi';
        this.loading = false;
      },
    });
  }
}
```

- [ ] Create `frontend/src/app/features/dashboard/team/team.component.html`:

```html
<section>
  <h2>Team</h2>
  <p *ngIf="loading">Memuat...</p>
  <p *ngIf="error" class="msg error">{{ error }}</p>
  <p *ngIf="!loading && !error && groups.length === 0">Tidak ada aktivitas Code Process saat ini</p>
  <div *ngFor="let group of groups" class="team-group">
    <h3>{{ group.assigneeEmail }} ({{ group.cards.length }})</h3>
    <ul>
      <li *ngFor="let card of group.cards">
        <span class="key">{{ card.issueKey }}</span> — {{ card.summary }}
        <span *ngIf="card.hasBudgetDetail" class="budget">{{ card.storyPoints }} SP · elapsed {{ card.elapsedLabel }} · sisa {{ card.remainingLabel }}</span>
      </li>
    </ul>
  </div>
</section>
```

### Task 5.2 — State + Integration: `GET /api/dashboard/team`

Already implemented as part of Task 5.1. Proceed to Task 5.u/5.i.

### Task 5.u / 5.i — UI + Integration Test: render grup, empty state, GET sukses/gagal

**Depends on:** Task 5.1
**Files:**
- Create: `frontend/src/app/features/dashboard/team/team.component.spec.ts`

- [ ] Create `frontend/src/app/features/dashboard/team/team.component.spec.ts`:

```typescript
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { TeamComponent } from './team.component';
import { BoardService, TeamResponse } from '../../../core/services/board.service';

describe('TeamComponent', () => {
  let fixture: ComponentFixture<TeamComponent>;
  let boardService: jasmine.SpyObj<BoardService>;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', ['getTeam']);

    await TestBed.configureTestingModule({
      imports: [TeamComponent],
      providers: [{ provide: BoardService, useValue: boardService }],
    }).compileComponents();

    fixture = TestBed.createComponent(TeamComponent);
  });

  it('renders one group per assignee with a ticket count heading', () => {
    const response: TeamResponse = {
      groups: [
        { assigneeEmail: 'a@esb.co.id', cards: [{ issueKey: 'OMS-1', summary: 'X', status: 'Code Process', hasBudgetDetail: false, storyPoints: 0, elapsedLabel: '', remainingLabel: '', deadlineLabel: '' }] },
      ],
    };
    boardService.getTeam.and.returnValue(of(response));
    fixture.detectChanges();

    const heading = fixture.nativeElement.querySelector('.team-group h3');
    expect(heading.textContent).toContain('a@esb.co.id');
    expect(heading.textContent).toContain('(1)');
  });

  it('shows the empty-state message when there are no groups', () => {
    boardService.getTeam.and.returnValue(of({ groups: [] }));
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('Tidak ada aktivitas Code Process saat ini');
  });

  it('shows an error message when loading fails', () => {
    boardService.getTeam.and.returnValue(throwError(() => ({ status: 500 })));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal memuat data team, coba lagi');
  });
});
```

- [ ] Run: `cd frontend && npm test -- --watch=false --browsers=ChromeHeadless --include='**/team.component.spec.ts'`
Expected: 3 tests pass.

### Task 5.routing — Wire `TeamComponent` into `app.routes.ts`

**Depends on:** Task 5.1
**Files:**
- Modify: `frontend/src/app/app.routes.ts`

- [ ] Find:

```typescript
import { BoardComponent } from './features/dashboard/board/board.component';
import { AuthGuard } from './core/guards/auth.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [{ path: '', component: BoardComponent }],
  },
  { path: '**', redirectTo: '' },
];
```

Replace with:

```typescript
import { BoardComponent } from './features/dashboard/board/board.component';
import { TeamComponent } from './features/dashboard/team/team.component';
import { AuthGuard } from './core/guards/auth.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [
      { path: '', component: BoardComponent },
      { path: 'team', component: TeamComponent },
    ],
  },
  { path: '**', redirectTo: '' },
];
```

- [ ] Run: `cd frontend && npm run build`
Expected: builds clean.

### Task 5.x — Dev Test

**Depends on:** Task 5.u/5.i, Task 5.routing
- [ ] `cd frontend && npm run build && npm test -- --watch=false --browsers=ChromeHeadless` — full suite green.
- [ ] `ng serve --proxy-config proxy.conf.json`: navigasi ke `/jira-notif/dashboard/team`, bandingkan isinya dengan `team.html` lama untuk data user yang sama.

### Task 5.exec — Commit

```bash
cd frontend
git add src/app/core/services/board.service.ts src/app/features/dashboard/team/ src/app/app.routes.ts
git commit -m "feat: add Team component"
```

---

## Chapter 6 — List: Overdue

**Category:** List
**AC covered:** AC4.1

**Interfaces:**
- Produces: adds `getOverdue(): Observable<OverdueResponse>` plus `OverdueItem`/`OverdueResponse` interfaces to `board.service.ts`. This is the last addition to `board.service.ts` in this plan — after this chapter it has `updatePreference`, `saveCredential`, `deleteCredential`, `getBoard`, `transitionTicket`, `getTeam`, `getOverdue`.
- This chapter appends the third entry to `app.routes.ts`'s `dashboard.children` array — `{ path: 'overdue', component: OverdueComponent }`.

### Task 6.1 — Interface + Logic: daftar kartu overdue, empty state

**Depends on:** Task 5.1
**Files:**
- Modify: `frontend/src/app/core/services/board.service.ts`
- Modify: `frontend/src/styles.scss`
- Create: `frontend/src/app/features/dashboard/overdue/overdue.component.ts`
- Create: `frontend/src/app/features/dashboard/overdue/overdue.component.html`

- [ ] In `frontend/src/app/core/services/board.service.ts`, add this interface after `TeamResponse` (before `TransitionResponse`):

```typescript
export interface OverdueItem extends Card {
  assigneeEmail: string;
}

export interface OverdueResponse {
  items: OverdueItem[];
}
```

- [ ] In the same file, find the `getTeam` method (the last method, immediately before the class's final `}`):

```typescript
  getTeam(): Observable<TeamResponse> {
    return this.http.get<TeamResponse>(`${environment.apiBasePath}/api/dashboard/team`);
  }
}
```

Replace with:

```typescript
  getTeam(): Observable<TeamResponse> {
    return this.http.get<TeamResponse>(`${environment.apiBasePath}/api/dashboard/team`);
  }

  getOverdue(): Observable<OverdueResponse> {
    return this.http.get<OverdueResponse>(`${environment.apiBasePath}/api/dashboard/overdue`);
  }
}
```

- [ ] Create `frontend/src/app/features/dashboard/overdue/overdue.component.ts`:

```typescript
import { Component, OnInit, inject } from '@angular/core';
import { NgFor, NgIf } from '@angular/common';
import { BoardService, OverdueItem } from '../../../core/services/board.service';

@Component({
  selector: 'app-overdue',
  standalone: true,
  imports: [NgFor, NgIf],
  templateUrl: './overdue.component.html',
})
export class OverdueComponent implements OnInit {
  protected items: OverdueItem[] = [];
  protected loading = true;
  protected error: string | null = null;

  private boardService = inject(BoardService);

  ngOnInit(): void {
    this.boardService.getOverdue().subscribe({
      next: (res) => {
        this.items = res.items;
        this.loading = false;
      },
      error: () => {
        this.error = 'Gagal memuat data overdue, coba lagi';
        this.loading = false;
      },
    });
  }
}
```

- [ ] Create `frontend/src/app/features/dashboard/overdue/overdue.component.html`:

```html
<section>
  <h2>Overdue</h2>
  <p *ngIf="loading">Memuat...</p>
  <p *ngIf="error" class="msg error">{{ error }}</p>
  <p *ngIf="!loading && !error && items.length === 0">Semua tiket masih dalam budget 🎉</p>
  <div *ngFor="let item of items" class="overdue-card">
    <div class="key">{{ item.issueKey }}</div>
    <div class="summary">{{ item.summary }}</div>
    <div class="assignee">{{ item.assigneeEmail }}</div>
    <div *ngIf="item.hasBudgetDetail" class="budget">{{ item.storyPoints }} SP · elapsed {{ item.elapsedLabel }} · sisa {{ item.remainingLabel }}</div>
  </div>
</section>
```

- [ ] Append to `frontend/src/styles.scss` (the global stylesheet `ng new` generated, at `src/` root — **not** `src/app/styles.scss`, which doesn't exist and isn't referenced by `angular.json`'s `styles` array. This is the only chapter that needs a new shared style rule, since the red-left-border card look is specific to Overdue and not reused elsewhere):

```scss
.overdue-card {
  border-left: 4px solid #b91c1c;
  border-top: 1px solid #e6e6e6;
  border-right: 1px solid #e6e6e6;
  border-bottom: 1px solid #e6e6e6;
  border-radius: 8px;
  padding: 10px 14px;
  margin-bottom: 10px;
}
.overdue-card .key {
  font-weight: 700;
  color: #0e65a9;
}
```

### Task 6.2 — State + Integration: `GET /api/dashboard/overdue`

Already implemented as part of Task 6.1. Proceed to Task 6.u/6.i.

### Task 6.u / 6.i — UI + Integration Test: render kartu, empty state, GET sukses/gagal

**Depends on:** Task 6.1
**Files:**
- Create: `frontend/src/app/features/dashboard/overdue/overdue.component.spec.ts`

- [ ] Create `frontend/src/app/features/dashboard/overdue/overdue.component.spec.ts`:

```typescript
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { OverdueComponent } from './overdue.component';
import { BoardService, OverdueResponse } from '../../../core/services/board.service';

describe('OverdueComponent', () => {
  let fixture: ComponentFixture<OverdueComponent>;
  let boardService: jasmine.SpyObj<BoardService>;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', ['getOverdue']);

    await TestBed.configureTestingModule({
      imports: [OverdueComponent],
      providers: [{ provide: BoardService, useValue: boardService }],
    }).compileComponents();

    fixture = TestBed.createComponent(OverdueComponent);
  });

  it('renders one card per overdue item', () => {
    const response: OverdueResponse = {
      items: [
        { issueKey: 'OMS-9', summary: 'Stuck ticket', status: 'Code Process', hasBudgetDetail: true, storyPoints: 5, elapsedLabel: '10j', remainingLabel: '-2j', deadlineLabel: '', assigneeEmail: 'a@esb.co.id' },
      ],
    };
    boardService.getOverdue.and.returnValue(of(response));
    fixture.detectChanges();

    const card = fixture.nativeElement.querySelector('.overdue-card');
    expect(card.textContent).toContain('OMS-9');
    expect(card.textContent).toContain('a@esb.co.id');
    expect(card.textContent).toContain('5 SP');
  });

  it('shows the empty-state message when there are no overdue items', () => {
    boardService.getOverdue.and.returnValue(of({ items: [] }));
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('Semua tiket masih dalam budget');
  });

  it('shows an error message when loading fails', () => {
    boardService.getOverdue.and.returnValue(throwError(() => ({ status: 500 })));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal memuat data overdue, coba lagi');
  });
});
```

- [ ] Run: `cd frontend && npm test -- --watch=false --browsers=ChromeHeadless --include='**/overdue.component.spec.ts'`
Expected: 3 tests pass.

### Task 6.routing — Wire `OverdueComponent` into `app.routes.ts`

**Depends on:** Task 6.1
**Files:**
- Modify: `frontend/src/app/app.routes.ts`

- [ ] Find:

```typescript
import { TeamComponent } from './features/dashboard/team/team.component';
import { AuthGuard } from './core/guards/auth.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [
      { path: '', component: BoardComponent },
      { path: 'team', component: TeamComponent },
    ],
  },
  { path: '**', redirectTo: '' },
];
```

Replace with:

```typescript
import { TeamComponent } from './features/dashboard/team/team.component';
import { OverdueComponent } from './features/dashboard/overdue/overdue.component';
import { AuthGuard } from './core/guards/auth.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [
      { path: '', component: BoardComponent },
      { path: 'team', component: TeamComponent },
      { path: 'overdue', component: OverdueComponent },
    ],
  },
  { path: '**', redirectTo: '' },
];
```

- [ ] Run: `cd frontend && npm run build`
Expected: builds clean.

### Task 6.x — Dev Test

**Depends on:** Task 6.u/6.i, Task 6.routing
- [ ] `cd frontend && npm run build && npm test -- --watch=false --browsers=ChromeHeadless` — full suite green.
- [ ] `ng serve --proxy-config proxy.conf.json`: navigasi ke `/jira-notif/dashboard/overdue`, bandingkan dengan `overdue.html` lama.

### Task 6.exec — Commit

```bash
cd frontend
git add src/app/core/services/board.service.ts src/styles.scss src/app/features/dashboard/overdue/ src/app/app.routes.ts
git commit -m "feat: add Overdue component"
```

---

## Chapter 7a — List: Admin User List

**Category:** List
**AC covered:** AC7.1

> Route ini hanya boleh diakses jika `AuthService.currentUser$`'s `isAdmin === true` — ditegakkan lewat `AdminGuard` sebagai UX saja; keamanan sebenarnya tetap di `AdminMiddleware` server-side (unchanged, Plan A).

**Ruling (plan-authoring decision, not in the spec's literal text):** the spec's Interface section for 7a lists "Jira Account ID (input editable)" and a conditional "Hapus Credential" button as part of THIS chapter, while 7b's Interface section lists the same elements again as what it wires up. Rendering a half-wired `<input>` with no save button, or a button with no click handler, for one chapter's lifetime violates "no half-finished implementations." This plan resolves it as: **7a renders Jira Account ID as read-only text and leaves the Aksi column empty**; **7b (next chapter) replaces both** with the real editable input + Simpan button + Aktifkan/Nonaktifkan + Hapus Credential + Hapus User buttons, all at once. No user-facing half-finished control exists at any committed point.

**Interfaces:**
- Produces: new file `frontend/src/app/core/services/admin.service.ts` with its first method `listUsers(): Observable<ListUsersResponse>`, and the `UserRow`/`ListUsersResponse` interfaces (field names copied verbatim from the Go `admin_dashboard_dto.UserRow` JSON tags: `userId`, `email`, `userName`, `flagActive`, `createdDate`, `jiraAccountId`, `hasCredential`, `credentialMasked`). New file `frontend/src/app/core/guards/admin.guard.ts`.
- This chapter appends the fourth entry to `app.routes.ts`'s `dashboard.children` array — `{ path: 'admin', component: AdminComponent, canActivate: [AdminGuard] }`.

### Task 7a.1 — Interface + Logic: tabel user, badge status, struktur kolom (tanpa wiring tombol aksi)

**Depends on:** Task 1.2 (uses `environment`, no other chapter dependency)
**Files:**
- Create: `frontend/src/app/core/services/admin.service.ts`
- Create: `frontend/src/app/features/dashboard/admin/admin.component.ts`
- Create: `frontend/src/app/features/dashboard/admin/admin.component.html`

- [ ] Create `frontend/src/app/core/services/admin.service.ts`:

```typescript
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
```

- [ ] Create `frontend/src/app/features/dashboard/admin/admin.component.ts`:

```typescript
import { Component, OnInit, inject } from '@angular/core';
import { NgFor, NgIf } from '@angular/common';
import { AdminService, UserRow } from '../../../core/services/admin.service';

@Component({
  selector: 'app-admin',
  standalone: true,
  imports: [NgFor, NgIf],
  templateUrl: './admin.component.html',
})
export class AdminComponent implements OnInit {
  protected users: UserRow[] = [];
  protected loading = true;
  protected error: string | null = null;

  private adminService = inject(AdminService);

  ngOnInit(): void {
    this.loadUsers();
  }

  protected loadUsers(): void {
    this.loading = true;
    this.error = null;

    this.adminService.listUsers().subscribe({
      next: (res) => {
        this.users = res.users;
        this.loading = false;
      },
      error: (err) => {
        this.error = err?.status === 403 ? 'Kamu tidak punya akses ke halaman ini' : 'Gagal memuat daftar user, coba lagi';
        this.loading = false;
      },
    });
  }
}
```

- [ ] Create `frontend/src/app/features/dashboard/admin/admin.component.html`:

```html
<section>
  <h2>Admin — Daftar User</h2>
  <p *ngIf="loading">Memuat...</p>
  <p *ngIf="error" class="msg error">{{ error }}</p>
  <div class="table-wrap" *ngIf="!loading && !error">
    <table class="admin-users">
      <thead>
        <tr><th>Email</th><th>Status</th><th>Jira Account ID</th><th>Credential</th><th>Aksi</th></tr>
      </thead>
      <tbody>
        <tr *ngFor="let user of users" [attr.data-user-id]="user.userId">
          <td>{{ user.email }}</td>
          <td><span class="status-badge" [class.active]="user.flagActive">{{ user.flagActive ? 'Aktif' : 'Nonaktif' }}</span></td>
          <td>{{ user.jiraAccountId || '—' }}</td>
          <td>{{ user.hasCredential ? 'Aktif' : '—' }}</td>
          <td class="actions"></td>
        </tr>
      </tbody>
    </table>
  </div>
</section>
```

### Task 7a.2 — State + Integration: `GET /api/dashboard/admin/users`

Already implemented as part of Task 7a.1. Proceed to Task 7a.3.

### Task 7a.3 — Routing: `AdminGuard`

**Depends on:** Task 1.2
**Files:**
- Create: `frontend/src/app/core/guards/admin.guard.ts`

- [ ] Create `frontend/src/app/core/guards/admin.guard.ts`:

```typescript
import { Injectable, inject } from '@angular/core';
import { CanActivate, Router } from '@angular/router';
import { Observable } from 'rxjs';
import { map, take } from 'rxjs/operators';
import { AuthService } from '../services/auth.service';

@Injectable({ providedIn: 'root' })
export class AdminGuard implements CanActivate {
  private authService = inject(AuthService);
  private router = inject(Router);

  canActivate(): Observable<boolean> {
    return this.authService.currentUser$.pipe(
      take(1),
      map((user) => {
        if (user?.isAdmin) {
          return true;
        }
        this.router.navigate(['/dashboard']);
        return false;
      }),
    );
  }
}
```

(`AdminGuard` only ever runs after `AuthGuard` has already succeeded — it's nested on the `admin` child route under `dashboard`, which `AuthGuard` already guards — so `currentUser$` is guaranteed to already hold a value by the time this runs.)

### Task 7a.u / 7a.i — UI + Integration Test: tabel render, badge, empty/401/403/500, AdminGuard

**Depends on:** Task 7a.1, Task 7a.3
**Files:**
- Create: `frontend/src/app/features/dashboard/admin/admin.component.spec.ts`
- Create: `frontend/src/app/core/guards/admin.guard.spec.ts`

- [ ] Create `frontend/src/app/features/dashboard/admin/admin.component.spec.ts`:

```typescript
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { AdminComponent } from './admin.component';
import { AdminService, ListUsersResponse } from '../../../core/services/admin.service';

describe('AdminComponent', () => {
  let fixture: ComponentFixture<AdminComponent>;
  let adminService: jasmine.SpyObj<AdminService>;

  beforeEach(async () => {
    adminService = jasmine.createSpyObj('AdminService', ['listUsers']);

    await TestBed.configureTestingModule({
      imports: [AdminComponent],
      providers: [{ provide: AdminService, useValue: adminService }],
    }).compileComponents();

    fixture = TestBed.createComponent(AdminComponent);
  });

  it('renders one row per user with status badge and Jira Account ID', () => {
    const response: ListUsersResponse = {
      users: [
        { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '2026-01-01', jiraAccountId: 'acc-1', hasCredential: true, credentialMasked: '••••ab3f' },
        { userId: 2, email: 'b@esb.co.id', userName: 'B', flagActive: false, createdDate: '2026-01-02', jiraAccountId: '', hasCredential: false, credentialMasked: '' },
      ],
    };
    adminService.listUsers.and.returnValue(of(response));
    fixture.detectChanges();

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    expect(rows.length).toBe(2);
    expect(rows[0].textContent).toContain('a@esb.co.id');
    expect(rows[0].querySelector('.status-badge').textContent).toContain('Aktif');
    expect(rows[1].querySelector('.status-badge').textContent).toContain('Nonaktif');
    expect(rows[1].textContent).toContain('—');
  });

  it('shows an empty table with no rows when there are no users', () => {
    adminService.listUsers.and.returnValue(of({ users: [] }));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelectorAll('tbody tr').length).toBe(0);
  });

  it('shows the access-denied message on a 403 response', () => {
    adminService.listUsers.and.returnValue(throwError(() => ({ status: 403 })));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Kamu tidak punya akses ke halaman ini');
  });

  it('shows a generic error message on a 500 response', () => {
    adminService.listUsers.and.returnValue(throwError(() => ({ status: 500 })));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal memuat daftar user, coba lagi');
  });
});
```

- [ ] Create `frontend/src/app/core/guards/admin.guard.spec.ts`:

```typescript
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
```

- [ ] Run: `cd frontend && npm test -- --watch=false --browsers=ChromeHeadless --include='**/admin.component.spec.ts' --include='**/admin.guard.spec.ts'`
Expected: 6 tests pass.

### Task 7a.routing — Wire `AdminComponent` + `AdminGuard` into `app.routes.ts`

**Depends on:** Task 7a.1, Task 7a.3
**Files:**
- Modify: `frontend/src/app/app.routes.ts`

- [ ] Find:

```typescript
import { OverdueComponent } from './features/dashboard/overdue/overdue.component';
import { AuthGuard } from './core/guards/auth.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [
      { path: '', component: BoardComponent },
      { path: 'team', component: TeamComponent },
      { path: 'overdue', component: OverdueComponent },
    ],
  },
  { path: '**', redirectTo: '' },
];
```

Replace with:

```typescript
import { OverdueComponent } from './features/dashboard/overdue/overdue.component';
import { AdminComponent } from './features/dashboard/admin/admin.component';
import { AuthGuard } from './core/guards/auth.guard';
import { AdminGuard } from './core/guards/admin.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [
      { path: '', component: BoardComponent },
      { path: 'team', component: TeamComponent },
      { path: 'overdue', component: OverdueComponent },
      { path: 'admin', component: AdminComponent, canActivate: [AdminGuard] },
    ],
  },
  { path: '**', redirectTo: '' },
];
```

- [ ] Run: `cd frontend && npm run build`
Expected: builds clean.

### Task 7a.x — Dev Test

**Depends on:** Task 7a.u/7a.i, Task 7a.routing
- [ ] `cd frontend && npm run build && npm test -- --watch=false --browsers=ChromeHeadless` — full suite green.
- [ ] `ng serve --proxy-config proxy.conf.json`: login sebagai admin → menu "Admin" tampil di nav → tabel user tampil. Login sebagai non-admin → menu "Admin" **tidak** tampil di nav, dan mengetik `/jira-notif/dashboard/admin` manual di address bar → redirect balik ke `/jira-notif/dashboard` (AC-equivalent of the spec's Test Scope row #3).

### Task 7a.exec — Commit

```bash
cd frontend
git add src/app/core/services/admin.service.ts src/app/core/guards/admin.guard.ts src/app/core/guards/admin.guard.spec.ts src/app/features/dashboard/admin/ src/app/app.routes.ts
git commit -m "feat: add Admin User List component and AdminGuard"
```

---

## Chapter 7b — Action: Admin Row Actions

**Category:** Action
**AC covered:** AC7.2, AC7.3, AC7.4, AC7.5

This chapter replaces Chapter 7a's read-only Jira Account ID cell and empty Aksi cell with the real editable input, Save button, and the Aktifkan/Nonaktifkan/Hapus Credential/Hapus User buttons — resolving the "ruling" noted at the top of Chapter 7a.

**Interfaces:**
- Consumes: `AdminComponent`, `AdminService` (Chapter 7a).
- Produces: adds `toggleActive`, `editJiraAccountId`, `deleteCredential`, `deleteUser` to `admin.service.ts` — all 4 call the **existing, unchanged** endpoints at `/dashboard/admin/users/...` (not `/api/dashboard/admin/...` — per the spec's Notes & Recommendations **C1**, these write endpoints were never moved under `/api/`, only the new read endpoint from Chapter 7a is).

### Task 7b.1 — Interface + Logic: 4 tombol aksi per baris + dialog konfirmasi

**Depends on:** Task 7a.1
**Files:**
- Modify: `frontend/src/app/core/services/admin.service.ts`
- Modify: `frontend/src/app/features/dashboard/admin/admin.component.ts`
- Modify: `frontend/src/app/features/dashboard/admin/admin.component.html`

- [ ] In `frontend/src/app/core/services/admin.service.ts`, find:

```typescript
  listUsers(): Observable<ListUsersResponse> {
    return this.http.get<ListUsersResponse>(`${environment.apiBasePath}/api/dashboard/admin/users`);
  }
}
```

Replace with:

```typescript
  listUsers(): Observable<ListUsersResponse> {
    return this.http.get<ListUsersResponse>(`${environment.apiBasePath}/api/dashboard/admin/users`);
  }

  toggleActive(userId: number, active: boolean): Observable<unknown> {
    return this.http.post(`${environment.apiBasePath}/dashboard/admin/users/${userId}/active`, { active });
  }

  editJiraAccountId(userId: number, jiraAccountId: string): Observable<unknown> {
    return this.http.post(`${environment.apiBasePath}/dashboard/admin/users/${userId}/jira-account-id`, { jiraAccountId });
  }

  deleteCredential(userId: number): Observable<unknown> {
    return this.http.delete(`${environment.apiBasePath}/dashboard/admin/users/${userId}/credential`);
  }

  deleteUser(userId: number): Observable<unknown> {
    return this.http.delete(`${environment.apiBasePath}/dashboard/admin/users/${userId}`);
  }
}
```

- [ ] In `frontend/src/app/features/dashboard/admin/admin.component.ts`, find:

```typescript
import { Component, OnInit, inject } from '@angular/core';
import { NgFor, NgIf } from '@angular/common';
import { AdminService, UserRow } from '../../../core/services/admin.service';

@Component({
  selector: 'app-admin',
  standalone: true,
  imports: [NgFor, NgIf],
  templateUrl: './admin.component.html',
})
export class AdminComponent implements OnInit {
  protected users: UserRow[] = [];
  protected loading = true;
  protected error: string | null = null;

  private adminService = inject(AdminService);

  ngOnInit(): void {
    this.loadUsers();
  }

  protected loadUsers(): void {
    this.loading = true;
    this.error = null;

    this.adminService.listUsers().subscribe({
      next: (res) => {
        this.users = res.users;
        this.loading = false;
      },
      error: (err) => {
        this.error = err?.status === 403 ? 'Kamu tidak punya akses ke halaman ini' : 'Gagal memuat daftar user, coba lagi';
        this.loading = false;
      },
    });
  }
}
```

Replace with:

```typescript
import { Component, OnInit, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { NgFor, NgIf } from '@angular/common';
import { AdminService, UserRow } from '../../../core/services/admin.service';

@Component({
  selector: 'app-admin',
  standalone: true,
  imports: [NgFor, NgIf, FormsModule],
  templateUrl: './admin.component.html',
})
export class AdminComponent implements OnInit {
  protected users: UserRow[] = [];
  protected loading = true;
  protected error: string | null = null;
  protected rowActionInFlight: Record<number, boolean> = {};
  protected rowMessage: Record<number, string> = {};
  protected jiraAccountIdDrafts: Record<number, string> = {};

  private adminService = inject(AdminService);

  ngOnInit(): void {
    this.loadUsers();
  }

  protected loadUsers(): void {
    this.loading = true;
    this.error = null;

    this.adminService.listUsers().subscribe({
      next: (res) => {
        this.users = res.users;
        this.jiraAccountIdDrafts = Object.fromEntries(res.users.map((u) => [u.userId, u.jiraAccountId]));
        this.loading = false;
      },
      error: (err) => {
        this.error = err?.status === 403 ? 'Kamu tidak punya akses ke halaman ini' : 'Gagal memuat daftar user, coba lagi';
        this.loading = false;
      },
    });
  }

  protected onToggleActive(user: UserRow): void {
    this.rowActionInFlight[user.userId] = true;
    this.adminService.toggleActive(user.userId, !user.flagActive).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        this.loadUsers();
      },
      error: (err) => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = err?.error?.error ?? 'Gagal mengubah status user';
      },
    });
  }

  protected onSaveJiraAccountId(user: UserRow): void {
    const value = this.jiraAccountIdDrafts[user.userId];
    this.rowActionInFlight[user.userId] = true;
    this.adminService.editJiraAccountId(user.userId, value).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        user.jiraAccountId = value;
      },
      error: (err) => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = err?.error?.error ?? 'Gagal menyimpan Jira Account ID';
      },
    });
  }

  protected onDeleteCredential(user: UserRow): void {
    if (!confirm('Hapus credential pribadi user ini?')) {
      return;
    }
    this.rowActionInFlight[user.userId] = true;
    this.adminService.deleteCredential(user.userId).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        this.loadUsers();
      },
      error: () => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = 'Gagal menghapus credential';
      },
    });
  }

  protected onDeleteUser(user: UserRow): void {
    if (!confirm('Hapus user ini secara permanen?')) {
      return;
    }
    this.rowActionInFlight[user.userId] = true;
    this.adminService.deleteUser(user.userId).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        this.loadUsers();
      },
      error: (err) => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = err?.error?.error ?? 'Gagal menghapus user';
      },
    });
  }
}
```

- [ ] In `frontend/src/app/features/dashboard/admin/admin.component.html`, find:

```html
          <td>{{ user.jiraAccountId || '—' }}</td>
          <td>{{ user.hasCredential ? 'Aktif' : '—' }}</td>
          <td class="actions"></td>
```

Replace with:

```html
          <td>
            <input type="text" [(ngModel)]="jiraAccountIdDrafts[user.userId]" [disabled]="rowActionInFlight[user.userId]" />
            <button (click)="onSaveJiraAccountId(user)" [disabled]="rowActionInFlight[user.userId]">Simpan ID</button>
          </td>
          <td>
            {{ user.hasCredential ? 'Aktif' : '—' }}
            <button *ngIf="user.hasCredential" (click)="onDeleteCredential(user)" [disabled]="rowActionInFlight[user.userId]">Hapus Credential</button>
          </td>
          <td class="actions">
            <button (click)="onToggleActive(user)" [disabled]="rowActionInFlight[user.userId]">{{ user.flagActive ? 'Nonaktifkan' : 'Aktifkan' }}</button>
            <button class="secondary" (click)="onDeleteUser(user)" [disabled]="rowActionInFlight[user.userId]">Hapus</button>
            <div class="msg error" *ngIf="rowMessage[user.userId]">{{ rowMessage[user.userId] }}</div>
          </td>
```

### Task 7b.2 — State + Integration: wire ke 4 endpoint aksi existing

Already implemented as part of Task 7b.1. Proceed to Task 7b.u/7b.i.

### Task 7b.u / 7b.i — UI + Integration Test: tombol kondisional, dialog konfirmasi, disabled saat in-flight, 4 endpoint sukses/gagal

**Depends on:** Task 7b.1
**Files:**
- Modify: `frontend/src/app/features/dashboard/admin/admin.component.spec.ts`

- [ ] In `frontend/src/app/features/dashboard/admin/admin.component.spec.ts`, find:

```typescript
    adminService = jasmine.createSpyObj('AdminService', ['listUsers']);
```

Replace with:

```typescript
    adminService = jasmine.createSpyObj('AdminService', ['listUsers', 'toggleActive', 'editJiraAccountId', 'deleteCredential', 'deleteUser']);
```

- [ ] Append these test cases to the same `describe('AdminComponent', ...)` block, after the existing tests (before the final closing `});`):

```typescript
  it('toggles active status and reloads the list on success', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    adminService.toggleActive.and.returnValue(of({}));

    fixture.nativeElement.querySelector('.actions button').click();

    expect(adminService.toggleActive).toHaveBeenCalledWith(1, false);
    expect(adminService.listUsers).toHaveBeenCalledTimes(2);
  });

  it('shows a row-scoped error message when toggling active status fails', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    adminService.toggleActive.and.returnValue(throwError(() => ({ error: { error: 'Tidak bisa menonaktifkan diri sendiri' } })));

    fixture.nativeElement.querySelector('.actions button').click();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.actions .msg.error').textContent).toContain('Tidak bisa menonaktifkan diri sendiri');
  });

  it('saves the Jira Account ID optimistically, without reloading the list', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: 'old-id', hasCredential: false, credentialMasked: '' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    adminService.editJiraAccountId.and.returnValue(of({}));

    fixture.componentInstance['jiraAccountIdDrafts'][1] = 'new-id';
    fixture.componentInstance['onSaveJiraAccountId'](user);

    expect(adminService.editJiraAccountId).toHaveBeenCalledWith(1, 'new-id');
    expect(adminService.listUsers).toHaveBeenCalledTimes(1);
    expect(user.jiraAccountId).toBe('new-id');
  });

  it('only shows the Hapus Credential button when the user has a credential', () => {
    const withCred = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: true, credentialMasked: '••••ab3f' };
    const withoutCred = { userId: 2, email: 'b@esb.co.id', userName: 'B', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '' };
    adminService.listUsers.and.returnValue(of({ users: [withCred, withoutCred] }));
    fixture.detectChanges();

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    expect(rows[0].textContent).toContain('Hapus Credential');
    expect(rows[1].textContent).not.toContain('Hapus Credential');
  });

  it('asks for confirmation before deleting a credential, and does nothing if cancelled', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: true, credentialMasked: '••••ab3f' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    spyOn(window, 'confirm').and.returnValue(false);

    fixture.componentInstance['onDeleteCredential'](user);

    expect(adminService.deleteCredential).not.toHaveBeenCalled();
  });

  it('deletes the credential and reloads the list on confirmation', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: true, credentialMasked: '••••ab3f' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    spyOn(window, 'confirm').and.returnValue(true);
    adminService.deleteCredential.and.returnValue(of({}));

    fixture.componentInstance['onDeleteCredential'](user);

    expect(adminService.deleteCredential).toHaveBeenCalledWith(1);
    expect(adminService.listUsers).toHaveBeenCalledTimes(2);
  });

  it('shows the self-lockout error message when deleting a user fails', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    spyOn(window, 'confirm').and.returnValue(true);
    adminService.deleteUser.and.returnValue(throwError(() => ({ error: { error: 'Tidak bisa menghapus diri sendiri' } })));

    fixture.componentInstance['onDeleteUser'](user);
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.actions .msg.error').textContent).toContain('Tidak bisa menghapus diri sendiri');
  });

  it('disables every action button for a row while an action is in flight', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '' };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    adminService.toggleActive.and.returnValue(of({}));

    fixture.componentInstance['rowActionInFlight'][1] = true;
    fixture.detectChanges();

    const buttons: HTMLButtonElement[] = Array.from(fixture.nativeElement.querySelectorAll('tbody tr button'));
    expect(buttons.every((btn) => btn.disabled)).toBe(true);
  });
```

- [ ] Run: `cd frontend && npm test -- --watch=false --browsers=ChromeHeadless --include='**/admin.component.spec.ts'`
Expected: all tests in the file pass (4 from Chapter 7a + 8 new = 12 total).

### Task 7b.x — Dev Test

**Depends on:** Task 7b.u/7b.i
- [ ] `cd frontend && npm run build && npm test -- --watch=false --browsers=ChromeHeadless` — full suite green.
- [ ] `ng serve --proxy-config proxy.conf.json` sebagai admin: toggle aktif/nonaktif satu user nyata, edit+simpan Jira Account ID, hapus credential (dengan dialog konfirmasi asli browser), hapus user test (dengan dialog konfirmasi) — bandingkan hasil dan behavior dengan `admin.html` lama satu per satu. Coba hapus/nonaktifkan akun admin sendiri → pastikan pesan self-lockout muncul (sama seperti versi lama).

### Task 7b.exec — Commit

```bash
cd frontend
git add src/app/core/services/admin.service.ts src/app/features/dashboard/admin/admin.component.ts src/app/features/dashboard/admin/admin.component.html src/app/features/dashboard/admin/admin.component.spec.ts
git commit -m "feat: add Admin row actions (toggle active, edit Jira Account ID, delete credential/user)"
```

---

## Chapter 8 — Action: Broadcast Message

**Category:** Action
**AC covered:** AC8.1, AC8.2

**Interfaces:**
- Consumes: `AdminComponent` (embeds this chapter's component into its template, matching the old `admin.html`'s layout).
- Produces: adds `broadcast(message: string): Observable<BroadcastResponse>` and the `BroadcastResponse` interface to `admin.service.ts`. Calls the **existing, unchanged** `POST /dashboard/admin/broadcast` (not under `/api/`, same C1 note as Chapter 7b).

### Task 8.1 — Interface + Logic: textarea, tombol kirim, area hasil, dialog konfirmasi

**Depends on:** Task 7a.1 (uses `AdminService`)
**Files:**
- Modify: `frontend/src/app/core/services/admin.service.ts`
- Create: `frontend/src/app/features/dashboard/admin/broadcast.component.ts`
- Create: `frontend/src/app/features/dashboard/admin/broadcast.component.html`

- [ ] In `frontend/src/app/core/services/admin.service.ts`, find:

```typescript
  deleteUser(userId: number): Observable<unknown> {
    return this.http.delete(`${environment.apiBasePath}/dashboard/admin/users/${userId}`);
  }
}
```

Replace with:

```typescript
  deleteUser(userId: number): Observable<unknown> {
    return this.http.delete(`${environment.apiBasePath}/dashboard/admin/users/${userId}`);
  }

  broadcast(message: string): Observable<BroadcastResponse> {
    return this.http.post<BroadcastResponse>(`${environment.apiBasePath}/dashboard/admin/broadcast`, { message });
  }
}
```

- [ ] In the same file, add this interface after `ListUsersResponse`:

```typescript
export interface BroadcastResponse {
  sentCount: number;
  failedEmails: string[];
}
```

- [ ] Create `frontend/src/app/features/dashboard/admin/broadcast.component.ts`:

```typescript
import { Component, inject } from '@angular/core';
import { NgIf } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { AdminService } from '../../../core/services/admin.service';

@Component({
  selector: 'app-broadcast',
  standalone: true,
  imports: [NgIf, FormsModule],
  templateUrl: './broadcast.component.html',
})
export class BroadcastComponent {
  protected message = '';
  protected submitting = false;
  protected result: { text: string; isError: boolean } | null = null;

  private adminService = inject(AdminService);

  protected get canSend(): boolean {
    return this.message.trim().length > 0 && !this.submitting;
  }

  protected onSend(): void {
    if (!this.canSend) {
      return;
    }
    if (!confirm('Kirim pengumuman ini ke semua user aktif?')) {
      return;
    }
    this.submitting = true;

    this.adminService.broadcast(this.message).subscribe({
      next: (res) => {
        this.submitting = false;
        this.message = '';
        const failedPart = res.failedEmails.length ? `, gagal ke: ${res.failedEmails.join(', ')}` : '';
        this.result = { text: `Terkirim ke ${res.sentCount} user${failedPart}`, isError: res.failedEmails.length > 0 };
      },
      error: (err) => {
        this.submitting = false;
        this.result = {
          text:
            err?.status === 503
              ? 'personal DM (OAuth) belum pernah di-setup di server ini, broadcast tidak tersedia'
              : err?.status === 400
                ? 'pesan tidak boleh kosong'
                : 'gagal mengirim broadcast, coba lagi',
          isError: true,
        };
      },
    });
  }
}
```

- [ ] Create `frontend/src/app/features/dashboard/admin/broadcast.component.html`:

```html
<section class="broadcast">
  <h2>Broadcast Message</h2>
  <textarea [(ngModel)]="message" placeholder="Tulis pengumuman fitur baru / update di sini..." [disabled]="submitting"></textarea>
  <button (click)="onSend()" [disabled]="!canSend">Kirim</button>
  <p class="msg" [class.error]="result?.isError" *ngIf="result">{{ result.text }}</p>
</section>
```

### Task 8.2 — State + Integration: `POST /dashboard/admin/broadcast`

Already implemented as part of Task 8.1. Proceed to embedding it into `AdminComponent` and testing.

### Task 8.embed — Embed `BroadcastComponent` into the Admin page

**Depends on:** Task 8.1
**Files:**
- Modify: `frontend/src/app/features/dashboard/admin/admin.component.ts`
- Modify: `frontend/src/app/features/dashboard/admin/admin.component.html`

- [ ] In `frontend/src/app/features/dashboard/admin/admin.component.ts`, find:

```typescript
import { Component, OnInit, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { NgFor, NgIf } from '@angular/common';
import { AdminService, UserRow } from '../../../core/services/admin.service';

@Component({
  selector: 'app-admin',
  standalone: true,
  imports: [NgFor, NgIf, FormsModule],
  templateUrl: './admin.component.html',
})
```

Replace with:

```typescript
import { Component, OnInit, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { NgFor, NgIf } from '@angular/common';
import { AdminService, UserRow } from '../../../core/services/admin.service';
import { BroadcastComponent } from './broadcast.component';

@Component({
  selector: 'app-admin',
  standalone: true,
  imports: [NgFor, NgIf, FormsModule, BroadcastComponent],
  templateUrl: './admin.component.html',
})
```

- [ ] In `frontend/src/app/features/dashboard/admin/admin.component.html`, find the final `</section>` at the end of the file and add right after it:

```html
<app-broadcast></app-broadcast>
```

### Task 8.u / 8.i — UI + Integration Test: tombol disabled saat kosong, hasil tampil sesuai response

**Depends on:** Task 8.1
**Files:**
- Create: `frontend/src/app/features/dashboard/admin/broadcast.component.spec.ts`

- [ ] Create `frontend/src/app/features/dashboard/admin/broadcast.component.spec.ts`:

```typescript
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { BroadcastComponent } from './broadcast.component';
import { AdminService } from '../../../core/services/admin.service';

describe('BroadcastComponent', () => {
  let fixture: ComponentFixture<BroadcastComponent>;
  let adminService: jasmine.SpyObj<AdminService>;

  beforeEach(async () => {
    adminService = jasmine.createSpyObj('AdminService', ['broadcast']);

    await TestBed.configureTestingModule({
      imports: [BroadcastComponent],
      providers: [{ provide: AdminService, useValue: adminService }],
    }).compileComponents();

    fixture = TestBed.createComponent(BroadcastComponent);
    fixture.detectChanges();
  });

  it('disables the Kirim button when the textarea is empty or only whitespace', () => {
    fixture.componentInstance['message'] = '   ';
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button').disabled).toBe(true);
  });

  it('enables the Kirim button when there is non-whitespace text', () => {
    fixture.componentInstance['message'] = 'Update baru!';
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('button').disabled).toBe(false);
  });

  it('does nothing if the confirmation dialog is cancelled', () => {
    fixture.componentInstance['message'] = 'Update baru!';
    spyOn(window, 'confirm').and.returnValue(false);

    fixture.componentInstance['onSend']();

    expect(adminService.broadcast).not.toHaveBeenCalled();
  });

  it('shows the sent count and clears the textarea on full success', () => {
    fixture.componentInstance['message'] = 'Update baru!';
    spyOn(window, 'confirm').and.returnValue(true);
    adminService.broadcast.and.returnValue(of({ sentCount: 5, failedEmails: [] }));

    fixture.componentInstance['onSend']();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('Terkirim ke 5 user');
    expect(fixture.componentInstance['message']).toBe('');
  });

  it('shows the failed email list and marks the result as an error when some sends fail', () => {
    fixture.componentInstance['message'] = 'Update baru!';
    spyOn(window, 'confirm').and.returnValue(true);
    adminService.broadcast.and.returnValue(of({ sentCount: 3, failedEmails: ['a@x.com', 'b@y.com'] }));

    fixture.componentInstance['onSend']();
    fixture.detectChanges();

    const msg = fixture.nativeElement.querySelector('.msg');
    expect(msg.textContent).toContain('Terkirim ke 3 user');
    expect(msg.textContent).toContain('a@x.com, b@y.com');
    expect(msg.classList).toContain('error');
  });

  it('shows the DM-not-configured message on a 503 response and keeps the textarea', () => {
    fixture.componentInstance['message'] = 'Update baru!';
    spyOn(window, 'confirm').and.returnValue(true);
    adminService.broadcast.and.returnValue(throwError(() => ({ status: 503 })));

    fixture.componentInstance['onSend']();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('personal DM (OAuth) belum pernah di-setup di server ini');
    expect(fixture.componentInstance['message']).toBe('Update baru!');
  });

  it('shows a generic error message on any other failure and keeps the textarea', () => {
    fixture.componentInstance['message'] = 'Update baru!';
    spyOn(window, 'confirm').and.returnValue(true);
    adminService.broadcast.and.returnValue(throwError(() => ({ status: 500 })));

    fixture.componentInstance['onSend']();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('gagal mengirim broadcast, coba lagi');
    expect(fixture.componentInstance['message']).toBe('Update baru!');
  });
});
```

- [ ] Run: `cd frontend && npm test -- --watch=false --browsers=ChromeHeadless --include='**/broadcast.component.spec.ts'`
Expected: 7 tests pass.

### Task 8.x — Dev Test

**Depends on:** Task 8.embed, Task 8.u/8.i
- [ ] `cd frontend && npm run build && npm test -- --watch=false --browsers=ChromeHeadless` — full suite green.
- [ ] `ng serve --proxy-config proxy.conf.json` sebagai admin: ketik pengumuman test di halaman Admin, kirim (dengan dialog konfirmasi), pastikan muncul "Terkirim ke N user" dengan N yang masuk akal (jumlah user aktif saat ini).

### Task 8.exec — Commit

```bash
cd frontend
git add src/app/core/services/admin.service.ts src/app/features/dashboard/admin/broadcast.component.ts src/app/features/dashboard/admin/broadcast.component.html src/app/features/dashboard/admin/broadcast.component.spec.ts src/app/features/dashboard/admin/admin.component.ts src/app/features/dashboard/admin/admin.component.html
git commit -m "feat: add Broadcast Message component, embedded into the Admin page"
```

---

## Chapter 9 — Custom Flow: Sprint Report

**Category:** Custom Flow
**AC covered:** AC9.1, AC9.2

**Interfaces:**
- Produces: new file `frontend/src/app/core/services/report.service.ts` with `getReport(sprints?: string[], capPercent?: number): Observable<GetReportResponse>`, and the `ReportRow`/`Report`/`GetReportResponse` interfaces — field names copied verbatim from the Go `sprint_report_dto.Row`/`Report` JSON tags (`sprintName`, `issueKey`, `task`, `estimatedHours`, `actualHours`, `deviationPercent`, `within`; `rows`, `sumEstimatedHours`, `sumActualHours`, `aggregateDeviationPercent`, `achievementPercent`, `capPercent`, `withinCount`, `totalCount`).
- This chapter appends the fifth and final entry to `app.routes.ts`'s `dashboard.children` array — `{ path: 'report', component: ReportComponent }`.

### Task 9.1 — Interface + Logic: checkbox sprint, input cap, tombol Generate, tabel + summary report

**Depends on:** Task 1.2 (uses `environment`, no other chapter dependency)
**Files:**
- Create: `frontend/src/app/core/services/report.service.ts`
- Create: `frontend/src/app/features/dashboard/report/report.component.ts`
- Create: `frontend/src/app/features/dashboard/report/report.component.html`

- [ ] Create `frontend/src/app/core/services/report.service.ts`:

```typescript
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
```

- [ ] Create `frontend/src/app/features/dashboard/report/report.component.ts`:

```typescript
import { Component, OnInit, inject } from '@angular/core';
import { NgFor, NgIf } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ReportService, Report } from '../../../core/services/report.service';

const DEFAULT_CAP_PERCENT = 12;

@Component({
  selector: 'app-report',
  standalone: true,
  imports: [NgFor, NgIf, FormsModule],
  templateUrl: './report.component.html',
})
export class ReportComponent implements OnInit {
  protected availableSprints: string[] = [];
  protected selectedSprints = new Set<string>();
  protected capPercent = DEFAULT_CAP_PERCENT;
  protected report: Report | null = null;
  protected loading = true;
  protected generating = false;
  protected error: string | null = null;

  private reportService = inject(ReportService);

  ngOnInit(): void {
    this.reportService.getReport().subscribe({
      next: (res) => {
        this.availableSprints = res.availableSprints;
        this.capPercent = res.capPercent;
        this.loading = false;
      },
      error: () => {
        this.error = 'Gagal memuat daftar sprint, coba lagi';
        this.loading = false;
      },
    });
  }

  protected onSprintToggle(sprint: string, event: Event): void {
    const checked = (event.target as HTMLInputElement).checked;
    if (checked) {
      this.selectedSprints.add(sprint);
    } else {
      this.selectedSprints.delete(sprint);
    }
  }

  protected onGenerate(): void {
    this.generating = true;
    this.error = null;

    this.reportService.getReport(Array.from(this.selectedSprints), this.capPercent).subscribe({
      next: (res) => {
        this.report = res.report;
        this.generating = false;
      },
      error: () => {
        this.error = 'Gagal membuat report, coba lagi';
        this.generating = false;
      },
    });
  }
}
```

(The error handler intentionally does not touch `this.report` — the spec requires the previously generated table to stay visible if a later Generate click fails.)

- [ ] Create `frontend/src/app/features/dashboard/report/report.component.html`:

```html
<section>
  <h2>Sprint Report</h2>
  <p *ngIf="loading">Memuat...</p>
  <div *ngIf="!loading">
    <div class="sprint-picker">
      <label *ngFor="let sprint of availableSprints">
        <input type="checkbox" (change)="onSprintToggle(sprint, $event)" />
        {{ sprint }}
      </label>
      <label>Cap % <input type="number" [(ngModel)]="capPercent" /></label>
      <button (click)="onGenerate()" [disabled]="generating">Generate</button>
    </div>
    <p *ngIf="error" class="msg error">{{ error }}</p>

    <div *ngIf="report">
      <p class="summary">
        Aggregate Deviation: {{ report.aggregateDeviationPercent }}% · Achievement: {{ report.achievementPercent }}% ·
        {{ report.withinCount }} dari {{ report.totalCount }} tiket dalam cap
      </p>
      <table class="report-table">
        <thead>
          <tr><th>Sprint</th><th>Jira Key</th><th>Task</th><th>Est. jam</th><th>Actual jam</th><th>Deviation %</th><th>Result</th></tr>
        </thead>
        <tbody>
          <tr *ngFor="let row of report.rows" [class.not-within]="!row.within">
            <td>{{ row.sprintName }}</td>
            <td>{{ row.issueKey }}</td>
            <td>{{ row.task }}</td>
            <td>{{ row.estimatedHours }}</td>
            <td>{{ row.actualHours }}</td>
            <td>{{ row.deviationPercent }}%</td>
            <td>{{ row.within ? 'Within' : 'Over' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</section>
```

### Task 9.2 — State + Integration: load awal + generate

Already implemented as part of Task 9.1 (`ngOnInit()`/`onGenerate()` above). Proceed to Task 9.u/9.i.

### Task 9.u / 9.i — UI + Integration Test: picker, tabel+summary, baris within=false ditandai, load/generate sukses/gagal

**Depends on:** Task 9.1
**Files:**
- Create: `frontend/src/app/features/dashboard/report/report.component.spec.ts`

- [ ] Create `frontend/src/app/features/dashboard/report/report.component.spec.ts`:

```typescript
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { ReportComponent } from './report.component';
import { ReportService, GetReportResponse } from '../../../core/services/report.service';

describe('ReportComponent', () => {
  let fixture: ComponentFixture<ReportComponent>;
  let reportService: jasmine.SpyObj<ReportService>;

  beforeEach(async () => {
    reportService = jasmine.createSpyObj('ReportService', ['getReport']);
    reportService.getReport.and.returnValue(of({ availableSprints: ['Sprint 42', 'Sprint 43'], capPercent: 12, report: null }));

    await TestBed.configureTestingModule({
      imports: [ReportComponent],
      providers: [{ provide: ReportService, useValue: reportService }],
    }).compileComponents();

    fixture = TestBed.createComponent(ReportComponent);
    fixture.detectChanges();
  });

  it('loads the available sprints on init and shows the picker without a table', () => {
    expect(reportService.getReport).toHaveBeenCalledWith();
    const labels = fixture.nativeElement.querySelectorAll('.sprint-picker label');
    expect(labels.length).toBeGreaterThanOrEqual(2);
    expect(fixture.nativeElement.querySelector('.report-table')).toBeNull();
  });

  it('shows an error message when loading the initial sprint list fails', () => {
    reportService.getReport.and.returnValue(throwError(() => ({ status: 500 })));
    fixture = TestBed.createComponent(ReportComponent);
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal memuat daftar sprint, coba lagi');
  });

  it('generates a report for the selected sprints and cap, and renders the table plus summary', () => {
    const response: GetReportResponse = {
      availableSprints: ['Sprint 42'],
      capPercent: 12,
      report: {
        rows: [
          { sprintName: 'Sprint 42', issueKey: 'OMS-1', task: 'Fix bug', estimatedHours: 8, actualHours: 7, deviationPercent: -12.5, within: true },
          { sprintName: 'Sprint 42', issueKey: 'OMS-2', task: 'New feature', estimatedHours: 8, actualHours: 20, deviationPercent: 150, within: false },
        ],
        sumEstimatedHours: 16,
        sumActualHours: 27,
        aggregateDeviationPercent: 68.75,
        achievementPercent: 50,
        capPercent: 12,
        withinCount: 1,
        totalCount: 2,
      },
    };
    reportService.getReport.and.returnValue(of(response));

    fixture.componentInstance['onSprintToggle']('Sprint 42', { target: { checked: true } } as unknown as Event);
    fixture.componentInstance['onGenerate']();
    fixture.detectChanges();

    expect(reportService.getReport).toHaveBeenCalledWith(['Sprint 42'], 12);
    const rows = fixture.nativeElement.querySelectorAll('.report-table tbody tr');
    expect(rows.length).toBe(2);
    expect(rows[1].classList).toContain('not-within');
    expect(fixture.nativeElement.querySelector('.summary').textContent).toContain('68.75');
  });

  it('keeps the previous report and shows an error message when a later generate fails', () => {
    const okResponse: GetReportResponse = {
      availableSprints: ['Sprint 42'],
      capPercent: 12,
      report: { rows: [], sumEstimatedHours: 0, sumActualHours: 0, aggregateDeviationPercent: 0, achievementPercent: 0, capPercent: 12, withinCount: 0, totalCount: 0 },
    };
    reportService.getReport.and.returnValue(of(okResponse));
    fixture.componentInstance['onGenerate']();
    fixture.detectChanges();

    reportService.getReport.and.returnValue(throwError(() => ({ status: 500 })));
    fixture.componentInstance['onGenerate']();
    fixture.detectChanges();

    expect(fixture.componentInstance['report']).toEqual(okResponse.report);
    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal membuat report, coba lagi');
  });
});
```

- [ ] Run: `cd frontend && npm test -- --watch=false --browsers=ChromeHeadless --include='**/report.component.spec.ts'`
Expected: 4 tests pass.

### Task 9.routing — Wire `ReportComponent` into `app.routes.ts`

**Depends on:** Task 9.1
**Files:**
- Modify: `frontend/src/app/app.routes.ts`

- [ ] Find:

```typescript
import { AdminComponent } from './features/dashboard/admin/admin.component';
import { AuthGuard } from './core/guards/auth.guard';
import { AdminGuard } from './core/guards/admin.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [
      { path: '', component: BoardComponent },
      { path: 'team', component: TeamComponent },
      { path: 'overdue', component: OverdueComponent },
      { path: 'admin', component: AdminComponent, canActivate: [AdminGuard] },
    ],
  },
  { path: '**', redirectTo: '' },
];
```

Replace with:

```typescript
import { AdminComponent } from './features/dashboard/admin/admin.component';
import { ReportComponent } from './features/dashboard/report/report.component';
import { AuthGuard } from './core/guards/auth.guard';
import { AdminGuard } from './core/guards/admin.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [
      { path: '', component: BoardComponent },
      { path: 'team', component: TeamComponent },
      { path: 'overdue', component: OverdueComponent },
      { path: 'admin', component: AdminComponent, canActivate: [AdminGuard] },
      { path: 'report', component: ReportComponent },
    ],
  },
  { path: '**', redirectTo: '' },
];
```

(This is the fifth and final entry — every nav link `DashboardShellComponent` renders in Chapter 1 now resolves to a real route.)

- [ ] Run: `cd frontend && npm run build`
Expected: builds clean.

### Task 9.x — Dev Test

**Depends on:** Task 9.u/9.i, Task 9.routing
- [ ] `cd frontend && npm run build && npm test -- --watch=false --browsers=ChromeHeadless` — full suite green. This is the last chapter — this run is the whole app's full regression, every component and route together.
- [ ] `ng serve --proxy-config proxy.conf.json`: navigasi ke `/jira-notif/dashboard/report`, pilih 1 sprint nyata, klik Generate, bandingkan hasil (Aggregate Deviation, Achievement %, tabel per-tiket) dengan hasil `report.html` lama untuk **sprint yang sama persis** — angkanya harus identik (business logic tidak berubah, hanya cara tampilnya).

### Task 9.exec — Commit

```bash
cd frontend
git add src/app/core/services/report.service.ts src/app/features/dashboard/report/ src/app/app.routes.ts
git commit -m "feat: add Sprint Report component"
```

---

## End of Plan B

After this plan, `frontend/` contains a complete, independently build-and-test-passing Angular 19 SPA covering all 6 pages (Login, My Board, Team, Overdue, Admin, Sprint Report) plus the Dashboard Shell/Nav, `AuthGuard`, `AdminGuard`, and a global 401 interceptor — consuming every endpoint Plan A shipped plus every pre-existing POST/DELETE action endpoint, with zero backend changes.

**What this plan deliberately does NOT do** (all Plan C's job, not yet written as a plan document):
- `frontend/dist/` is not embedded into the Go binary (no `go:embed`, no catch-all shell route).
- The old HTML routes/templates (`Dashboard`, `TeamView`, `OverdueView`, admin's `UserList`, `Report`, and `login.html`/`LoginPage`) are not removed — they still serve real traffic today, completely unaffected by this plan.
- The Go `Callback` handler's error paths still render `login.html` directly instead of redirecting to `{BasePath}/?error=<message>` (see Chapter 1's note) — Angular's `LoginComponent` is ready for that redirect the moment Plan C wires it up.
- `.github/workflows/deploy.yml` is not updated to build the Angular app as part of deploy.

**Next:** Plan C (not yet created) does all four of the above in one pass — it is destructive to the current production fallback, so per the spec's "Effort Summary & Deploy Order," it must be the very last plan executed, only once this plan and Plan A are both fully merged and reviewed.

## Self-Review

**1. Spec coverage** — every AC in the Background's table (AC1.1-AC9.2) maps to at least one task above: AC1.1/AC1.5 → Chapter 1 routing + shell; AC1.2 → Chapter 1 `AuthService`; AC1.3/AC1.4 → Chapter 1 `LoginComponent`; AC1.6 → Plan A (consumed, not re-implemented here); AC1.7 → Chapter 7a `AdminGuard` (UX) + Plan A `AdminMiddleware` (real enforcement, unchanged); AC2.1-AC2.3 → Chapter 4; AC3.1 → Chapter 5; AC4.1 → Chapter 6; AC5.1/AC5.2 → Chapter 2; AC6.1/AC6.2 → Chapter 3; AC7.1 → Chapter 7a; AC7.2-AC7.5 → Chapter 7b; AC8.1/AC8.2 → Chapter 8; AC9.1/AC9.2 → Chapter 9. AC10.1/AC10.2 (embed + deploy) are explicitly Plan C's scope, not this plan's.

**2. Placeholder scan** — no "TBD"/"TODO"/"handle edge cases" language anywhere above; every step either contains complete code or explicitly says "already implemented as part of Task N.1" with a pointer to exactly which prior step. The one deliberate scope limitation this plan accepts (not a placeholder, a documented decision): per the spec's own Task Breakdown, every `[N].i` Integration Test row lives in the *component* spec file with the relevant service mocked — no chapter gets a dedicated `board.service.spec.ts`/`admin.service.spec.ts`/`report.service.spec.ts` asserting the literal HTTP method/URL/body each service method sends. This means a typo'd URL inside a service method would not be caught by any automated test in this plan — it would surface immediately during that chapter's Dev Test (`ng serve` against the real backend), which is this plan's accepted mitigation, matching the spec's own file-per-task assignments exactly rather than expanding scope beyond them.

**3. Type consistency** — cross-checked every interface name used across chapter boundaries: `Card`/`BoardColumn`/`BoardResponse`/`TransitionResponse` (defined Chapter 4, reused by name in Chapter 5/6's `TeamGroup`/`OverdueItem extends Card`); `UserRow`/`ListUsersResponse` (Chapter 7a, reused unchanged through 7b/8); `CurrentUser` (Chapter 1's `AuthService`, reused by `AdminGuard` in Chapter 7a); method names on `BoardService` (`getBoard`, `getTeam`, `getOverdue`, `transitionTicket`, `updatePreference`, `saveCredential`, `deleteCredential`) and `AdminService` (`listUsers`, `toggleActive`, `editJiraAccountId`, `deleteCredential`, `deleteUser`, `broadcast`) match their "Produces" declarations exactly everywhere they're consumed. `app.routes.ts`'s `dashboard.children` array was traced through all five incremental edits (Chapters 4, 5, 6, 7a, 9) to confirm each "Find X" block matches exactly what the immediately preceding chapter's edit left behind.

**Execution note on parallelism:** unlike Plan A, several chapters here have no dependency on each other and could, in principle, run out of order (Chapter 7a/9 both only depend on Task 1.2, not on Chapters 2-6) — but per this skill's rule, **never dispatch multiple implementation subagents in parallel**, so the controller should still run chapters 1 → 2 → 3 → 4 → 5 → 6 → 7a → 7b → 8 → 9 strictly in sequence regardless, the same discipline Plan A used.
