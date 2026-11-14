import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { Component } from '@angular/core';
import { routes } from './app.routes';
import { AuthGuard } from './core/guards/auth.guard';
import { AdminGuard } from './core/guards/admin.guard';
import { AdminShellComponent } from './layouts/admin-shell/admin-shell.component';

@Component({ selector: 'app-blank', standalone: true, template: '' })
class BlankComponent {}

describe('app routes', () => {
  it('redirects the old /dashboard/admin to /admin', async () => {
    TestBed.configureTestingModule({
      providers: [
        provideRouter(
          // Same route tree, components swapped out so nothing renders or calls HTTP.
          routes.map((r) => ({ ...r, component: r.component ? BlankComponent : undefined, canActivate: [], children: r.children?.map((c) => ({ ...c, component: c.component ? BlankComponent : undefined })) })),
        ),
        { provide: AuthGuard, useValue: { canActivate: () => true } },
        { provide: AdminGuard, useValue: { canActivate: () => true } },
      ],
    });
    const router = TestBed.inject(Router);
    await router.navigateByUrl('/dashboard/admin');
    expect(router.url).toBe('/admin');
  });

  it('guards /admin with AuthGuard and AdminGuard on the admin shell', () => {
    const admin = routes.find((r) => r.path === 'admin')!;
    expect(admin.component).toBe(AdminShellComponent);
    expect(admin.canActivate).toEqual([AuthGuard, AdminGuard]);
    expect(admin.children!.map((c) => c.path)).toEqual(jasmine.arrayContaining(['', 'users', 'broadcast']));
  });
});
