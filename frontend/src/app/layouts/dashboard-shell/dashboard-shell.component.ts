import { Component, HostListener, inject } from '@angular/core';
import { AsyncPipe, NgIf } from '@angular/common';
import { ActivatedRoute, NavigationEnd, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { filter, map, startWith } from 'rxjs';
import { AuthService } from '../../core/services/auth.service';
import { IconComponent } from '../../shared/ui/icon.component';
import { environment } from '../../../environments/environment';

interface PageHeading {
  heading: string;
  subheading: string;
}

@Component({
  selector: 'app-dashboard-shell',
  standalone: true,
  imports: [RouterOutlet, RouterLink, RouterLinkActive, AsyncPipe, NgIf, IconComponent],
  templateUrl: './dashboard-shell.component.html',
})
export class DashboardShellComponent {
  protected authService = inject(AuthService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);

  protected logoutHref = `${environment.apiBasePath}/auth/logout`;
  protected menuOpen = false;
  protected sidebarOpen = false;

  protected initial$ = this.authService.currentUser$.pipe(
    map((user) => (user?.email ?? '?').charAt(0).toUpperCase()),
  );

  /** Page title shown in the top bar, taken from the deepest route's `data`. */
  protected page$ = this.router.events.pipe(
    filter((event) => event instanceof NavigationEnd),
    startWith(null),
    map((): PageHeading => {
      let snapshot = this.route.snapshot;
      while (snapshot.firstChild) {
        snapshot = snapshot.firstChild;
      }
      return { heading: snapshot.data['heading'] ?? '', subheading: snapshot.data['subheading'] ?? '' };
    }),
  );

  protected toggleMenu(event: Event): void {
    event.stopPropagation();
    this.menuOpen = !this.menuOpen;
  }

  protected closeMenu(): void {
    this.menuOpen = false;
  }

  protected toggleSidebar(): void {
    this.sidebarOpen = !this.sidebarOpen;
  }

  protected closeSidebar(): void {
    this.sidebarOpen = false;
  }

  @HostListener('document:click')
  protected onDocumentClick(): void {
    this.menuOpen = false;
  }

  @HostListener('document:keydown.escape')
  protected onEscape(): void {
    this.menuOpen = false;
    this.sidebarOpen = false;
  }
}
