import { Component, ElementRef, HostListener, OnInit, ViewChild, inject } from '@angular/core';
import { AsyncPipe, NgIf } from '@angular/common';
import { ActivatedRoute, NavigationEnd, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { filter, map, startWith } from 'rxjs';
import { AuthService } from '../../core/services/auth.service';
import { BoardService } from '../../core/services/board.service';
import { MuteStateService } from '../../core/services/mute-state.service';
import { BoardContextService } from '../../core/services/board-context.service';
import { NotificationSwitchService } from '../../core/services/notification-switch.service';
import { BoardAvatarComponent } from '../../shared/ui/board-avatar.component';
import { NotificationConsentComponent } from '../../shared/ui/notification-consent.component';
import { IconComponent } from '../../shared/ui/icon.component';
import { UserAvatarComponent, myAvatarUrl } from '../../shared/ui/user-avatar.component';
import { ChatAppBannerComponent } from '../../shared/ui/chat-app-banner.component';
import { environment } from '../../../environments/environment';

interface PageHeading {
  heading: string;
  subheading: string;
}

@Component({
  selector: 'app-dashboard-shell',
  standalone: true,
  imports: [RouterOutlet, RouterLink, RouterLinkActive, AsyncPipe, NgIf, IconComponent, UserAvatarComponent, BoardAvatarComponent, NotificationConsentComponent, ChatAppBannerComponent],
  templateUrl: './dashboard-shell.component.html',
})
export class DashboardShellComponent implements OnInit {
  protected authService = inject(AuthService);
  protected muteState = inject(MuteStateService);
  protected boardContext = inject(BoardContextService);
  private boardService = inject(BoardService);
  private notifSwitch = inject(NotificationSwitchService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);

  @ViewChild('userMenu') private userMenu?: ElementRef<HTMLElement>;
  @ViewChild('boardMenu') private boardMenu?: ElementRef<HTMLElement>;

  protected logoutHref =`${environment.apiBasePath}/auth/logout`;
  protected menuOpen = false;
  protected boardMenuOpen = false;
  protected sidebarOpen = false;

  /** The logged-in user's Jira profile picture. */
  protected myAvatar = myAvatarUrl();

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

  ngOnInit(): void {
    // Seeds the top bar mute badge on every dashboard page (e.g. Admin);
    // the Pengaturan page refreshes it again on its own load.
    this.boardService.getSettings().subscribe({
      next: (res) => {
        this.muteState.set(res.mute.global);
        // Not chosen yet → the consent pop up opens.
        this.notifSwitch.set(res.notifications);
      },
      error: () => undefined,
    });
    this.boardContext.load();
  }

  protected toggleBoardMenu(): void {
    this.boardMenuOpen = !this.boardMenuOpen;
    if (this.boardMenuOpen) this.menuOpen = false;
  }

  protected selectBoard(boardId: number | null): void {
    this.boardContext.select(boardId);
    this.boardMenuOpen = false;
  }

  /** "9–22 Nov" from the sprint's ISO dates (WIB). */
  protected sprintRange(start: string, end: string): string {
    if (!start || !end) return '';
    const fmt = (iso: string, withMonth: boolean) =>
      new Date(iso).toLocaleDateString('id-ID', {
        timeZone: 'Asia/Jakarta',
        day: 'numeric',
        ...(withMonth ? { month: 'short' } : {}),
      });
    return `${fmt(start, false)}–${fmt(end, true)}`;
  }

  /** "16.30" today, or "Sen 08.30" for another day — always WIB. */
  protected muteLabel(until: Date): string {
    const wib = { timeZone: 'Asia/Jakarta' } as const;
    const sameDay = until.toLocaleDateString('id-ID', wib) === new Date().toLocaleDateString('id-ID', wib);
    const time = until.toLocaleTimeString('id-ID', { ...wib, hour: '2-digit', minute: '2-digit' });
    return sameDay ? time : `${until.toLocaleDateString('id-ID', { ...wib, weekday: 'short' })} ${time}`;
  }

  protected toggleMenu(): void {
    this.menuOpen = !this.menuOpen;
    if (this.menuOpen) this.boardMenuOpen = false;
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
    const target = event.target instanceof Node ? event.target : null;
    const inside = (ref?: ElementRef<HTMLElement>) => !!(target && ref?.nativeElement.contains(target));
    if (this.menuOpen && !inside(this.userMenu)) {
      this.menuOpen = false;
    }
    if (this.boardMenuOpen && !inside(this.boardMenu)) {
      this.boardMenuOpen = false;
    }
  }

  @HostListener('document:keydown.escape')
  protected onEscape(): void {
    this.menuOpen = false;
    this.boardMenuOpen = false;
    this.sidebarOpen = false;
  }
}
