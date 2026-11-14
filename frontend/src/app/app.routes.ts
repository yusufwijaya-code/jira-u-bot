import { Routes } from '@angular/router';
import { LoginComponent } from './features/login/login.component';
import { DashboardShellComponent } from './layouts/dashboard-shell/dashboard-shell.component';
import { AdminShellComponent } from './layouts/admin-shell/admin-shell.component';
import { SettingsComponent } from './features/dashboard/settings/settings.component';
import { HomeComponent } from './features/dashboard/home/home.component';
import { AdminComponent } from './features/dashboard/admin/admin.component';
import { BroadcastComponent } from './features/dashboard/admin/broadcast.component';
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
    path: 'admin',
    component: AdminShellComponent,
    canActivate: [AuthGuard, AdminGuard],
    children: [
      {
        path: '',
        pathMatch: 'full',
        component: AdminComponent,
        title: 'Admin · Nudge',
        data: { heading: 'Dashboard', subheading: 'Ringkasan user Nudge', crumbs: ['Admin'] },
      },
      {
        path: 'users',
        component: AdminComponent,
        title: 'User · Admin · Nudge',
        data: { heading: 'User', subheading: 'Kelola user dan pengaturannya', crumbs: ['Admin', 'User'] },
      },
      {
        path: 'broadcast',
        component: BroadcastComponent,
        title: 'Broadcast · Admin · Nudge',
        data: { heading: 'Broadcast Message', subheading: 'Kirim pengumuman ke semua user aktif', crumbs: ['Admin', 'Broadcast Message'] },
      },
    ],
  },
  { path: '**', redirectTo: '' },
];
