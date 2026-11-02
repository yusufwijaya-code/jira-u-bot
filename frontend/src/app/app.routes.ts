import { Routes } from '@angular/router';
import { LoginComponent } from './features/login/login.component';
import { DashboardShellComponent } from './layouts/dashboard-shell/dashboard-shell.component';
import { BoardComponent } from './features/dashboard/board/board.component';
import { TeamComponent } from './features/dashboard/team/team.component';
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
