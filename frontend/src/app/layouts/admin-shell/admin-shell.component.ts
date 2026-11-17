import { Component, ElementRef, HostListener, ViewChild, inject } from '@angular/core';
import { AsyncPipe } from '@angular/common';
import { ActivatedRoute, NavigationEnd, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { filter, map, startWith } from 'rxjs';
import { AuthService } from '../../core/services/auth.service';
import { IconComponent } from '../../shared/ui/icon.component';
import { TopbarHeightDirective } from '../../shared/ui/topbar-height.directive';
import { UserAvatarComponent, myAvatarUrl } from '../../shared/ui/user-avatar.component';
import { environment } from '../../../environments/environment';

interface AdminPage {
  heading: string;
  subheading: string;
  crumbs: string[];
}

/**
 * Layout of the admin area (/admin, Tech Spec "Admin Panel Revamp"): its
 * own sidebar (Dashboard / User / Broadcast Message / Pesan / Aturan Board) and a top bar with a
 * way back to the main dashboard. No board picker or mute badge — those
 * belong to the user's own dashboard.
 */
@Component({
  selector: 'app-admin-shell',
  standalone: true,
  imports: [TopbarHeightDirective, RouterOutlet, RouterLink, RouterLinkActive, AsyncPipe, IconComponent, UserAvatarComponent],
  templateUrl: './admin-shell.component.html',
})
export class AdminShellComponent {
  protected authService = inject(AuthService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);

  @ViewChild('userMenu') private userMenu?: ElementRef<HTMLElement>;

  protected logoutHref = `${environment.apiBasePath}/auth/logout`;
  protected menuOpen = false;
  protected sidebarOpen = false;

  /** The logged-in user's Jira profile picture. */
  protected myAvatar = myAvatarUrl();

  /** Heading, subheading and breadcrumb from the deepest route's `data`. */
  protected page$ = this.router.events.pipe(
    filter((event) => event instanceof NavigationEnd),
    startWith(null),
    map((): AdminPage => {
      let snapshot = this.route.snapshot;
      while (snapshot.firstChild) {
        snapshot = snapshot.firstChild;
      }
      return {
        heading: snapshot.data['heading'] ?? '',
        subheading: snapshot.data['subheading'] ?? '',
        crumbs: snapshot.data['crumbs'] ?? [],
      };
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

  /** Closes the user menu on any interaction outside of it (same as the dashboard shell). */
  @HostListener('pointerdown', ['$event'])
  @HostListener('click', ['$event'])
  @HostListener('focusin', ['$event'])
  @HostListener('document:pointerdown', ['$event'])
  @HostListener('document:click', ['$event'])
  @HostListener('document:focusin', ['$event'])
  protected onDocumentInteraction(event: Event): void {
    const target = event.target instanceof Node ? event.target : null;
    if (this.menuOpen && !(target && this.userMenu?.nativeElement.contains(target))) {
      this.menuOpen = false;
    }
  }

  @HostListener('document:keydown.escape')
  protected onEscape(): void {
    this.menuOpen = false;
    this.sidebarOpen = false;
  }
}
