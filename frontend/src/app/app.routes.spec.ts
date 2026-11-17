import { TestBed } from '@angular/core/testing';
import { Route, Router, provideRouter } from '@angular/router';
import { Component } from '@angular/core';
import { routes } from './app.routes';
import { AuthGuard } from './core/guards/auth.guard';
import { AdminGuard } from './core/guards/admin.guard';
import { AdminShellComponent } from './layouts/admin-shell/admin-shell.component';
import { AdminUserDetailComponent } from './features/admin/user-detail/admin-user-detail.component';

@Component({ selector: 'app-blank', standalone: true, template: '' })
class BlankComponent {}

/** Same route tree with every page swapped for a blank one and no guards. */
function blank(list: Route[]): Route[] {
  return list.map((r) => {
    const { loadComponent, component, canActivate, children, ...rest } = r;
    return {
      ...rest,
      ...(component || loadComponent ? { component: BlankComponent } : {}),
      ...(children ? { children: blank(children) } : {}),
    };
  });
}

describe('app routes', () => {
  const admin = () => routes.find((r) => r.path === 'admin')!;

  it('redirects the old /dashboard/admin to /admin', async () => {
    TestBed.configureTestingModule({ providers: [provideRouter(blank(routes))] });
    const router = TestBed.inject(Router);
    await router.navigateByUrl('/dashboard/admin');
    expect(router.url).toBe('/admin');
  });

  it('guards the lazy admin shell with AuthGuard and AdminGuard', async () => {
    expect(admin().canActivate).toEqual([AuthGuard, AdminGuard]);
    expect(await admin().loadComponent!()).toBe(AdminShellComponent);
  });

  it('has the admin pages', async () => {
    expect(admin().children!.map((c) => c.path)).toEqual(['', 'users', 'users/:id', 'broadcast', 'chat-logs', 'tracking-rules', 'tracking-rules/new', 'tracking-rules/:key']);
    const detail = admin().children!.find((c) => c.path === 'users/:id')!;
    expect(await detail.loadComponent!()).toBe(AdminUserDetailComponent);
  });
});
