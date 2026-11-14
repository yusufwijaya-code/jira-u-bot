import { Routes } from '@angular/router';
import { LoginComponent } from './features/login/login.component';
import { DashboardShellComponent } from './layouts/dashboard-shell/dashboard-shell.component';
import { SettingsComponent } from './features/dashboard/settings/settings.component';
import { HomeComponent } from './features/dashboard/home/home.component';
import { AuthGuard } from './core/guards/auth.guard';
import { AdminGuard } from './core/guards/admin.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent, title: 'Masuk · Nudge' },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [
      {
        path: '',
        pathMatch: 'full',
        component: HomeComponent,
        title: 'Dashboard · Nudge',
        data: { heading: 'Dashboard', subheading: 'Ringkasan tiket dan budget waktu kamu' },
      },
      {
        path: 'settings',
        component: SettingsComponent,
        title: 'Pengaturan · Nudge',
        data: { heading: 'Pengaturan', subheading: 'Atur cara Nudge mengingatkan kamu' },
      },
      // The admin panel moved to its own shell (Tech Spec "Admin Panel Revamp").
      { path: 'admin', pathMatch: 'full', redirectTo: '/admin' },
    ],
  },
  {
    // Lazy-loaded: only admins ever download the admin panel.
    path: 'admin',
    loadComponent: () => import('./layouts/admin-shell/admin-shell.component').then((m) => m.AdminShellComponent),
    canActivate: [AuthGuard, AdminGuard],
    children: [
      {
        path: '',
        pathMatch: 'full',
        loadComponent: () => import('./features/admin/overview/admin-overview.component').then((m) => m.AdminOverviewComponent),
        title: 'Admin · Nudge',
        data: { heading: 'Dashboard', subheading: 'Ringkasan user Nudge', crumbs: ['Admin'] },
      },
      {
        path: 'users',
        loadComponent: () => import('./features/admin/users/admin-users.component').then((m) => m.AdminUsersComponent),
        title: 'User · Admin · Nudge',
        data: { heading: 'User', subheading: 'Kelola user dan pengaturannya', crumbs: ['Admin', 'User'] },
      },
      {
        path: 'users/:id',
        loadComponent: () => import('./features/admin/user-detail/admin-user-detail.component').then((m) => m.AdminUserDetailComponent),
        title: 'Detail User · Admin · Nudge',
        data: { heading: 'Detail User', subheading: 'Atur notifikasi, mute, dan Jira credential user', crumbs: ['Admin', 'User', 'Detail'] },
      },
      {
        path: 'broadcast',
        loadComponent: () => import('./features/admin/broadcast/broadcast.component').then((m) => m.BroadcastComponent),
        title: 'Broadcast · Admin · Nudge',
        data: { heading: 'Broadcast Message', subheading: 'Kirim pengumuman ke semua user aktif', crumbs: ['Admin', 'Broadcast Message'] },
      },
    ],
  },
  { path: '**', redirectTo: '' },
];
