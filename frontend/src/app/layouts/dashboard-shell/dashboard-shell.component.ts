import { Component, ElementRef, HostListener, ViewChild, inject } from '@angular/core';
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

  @ViewChild('userMenu') private userMenu?: ElementRef<HTMLElement>;

  protected logoutHref =`${environment.apiBasePath}/auth/logout`;
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

  protected toggleMenu(): void {
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

  /**
   * Closes the user menu on any click (or keyboard focus) outside of it.
   * Uses a containment check instead of stopPropagation on the trigger, so
   * it doesn't depend on event ordering and any click elsewhere on the page
   * — content, sidebar, inputs — reliably closes it.
   */
  // Listened on the shell's own host element (which wraps the whole page)
  // as well as on document: element-level listeners still fire even when
  // something in the browser (e.g. an extension) swallows events before they
  // reach document. Handling the same event twice is harmless.
  @HostListener('pointerdown', ['$event'])
  @HostListener('click', ['$event'])
  @HostListener('focusin', ['$event'])
  @HostListener('document:pointerdown', ['$event'])
  @HostListener('document:click', ['$event'])
  @HostListener('document:focusin', ['$event'])
  protected onDocumentInteraction(event: Event): void {
    if (!this.menuOpen) {
      return;
    }
    const menu = this.userMenu?.nativeElement;
    if (menu && event.target instanceof Node && menu.contains(event.target)) {
      return;
    }
    this.menuOpen = false;
  }

  @HostListener('document:keydown.escape')
  protected onEscape(): void {
    this.menuOpen = false;
    this.sidebarOpen = false;
  }
}
