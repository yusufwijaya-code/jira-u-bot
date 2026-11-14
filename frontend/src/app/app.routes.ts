import { Routes } from '@angular/router';
import { LoginComponent } from './features/login/login.component';
import { DashboardShellComponent } from './layouts/dashboard-shell/dashboard-shell.component';
import { SettingsComponent } from './features/dashboard/settings/settings.component';
import { AdminComponent } from './features/dashboard/admin/admin.component';
import { AuthGuard } from './core/guards/auth.guard';
import { AdminGuard } from './core/guards/admin.guard';

export const routes: Routes = [
  { path: '', component: LoginComponent, title: 'Masuk · Nudge' },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [
      { path: '', pathMatch: 'full', redirectTo: 'settings' },
      {
        path: 'settings',
        component: SettingsComponent,
        title: 'Pengaturan · Nudge',
        data: { heading: 'Pengaturan', subheading: 'Atur cara Nudge mengingatkan kamu' },
      },
      {
        path: 'admin',
        component: AdminComponent,
        canActivate: [AdminGuard],
        title: 'Admin · Nudge',
        data: { heading: 'Admin', subheading: 'Kelola user dan kirim pengumuman' },
      },
    ],
  },
  { path: '**', redirectTo: '' },
];
