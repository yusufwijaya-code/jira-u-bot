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
