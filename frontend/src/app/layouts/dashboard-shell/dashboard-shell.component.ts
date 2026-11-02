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
