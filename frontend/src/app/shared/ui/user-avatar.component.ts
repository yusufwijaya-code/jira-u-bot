import { Component, computed, input, signal } from '@angular/core';
import { environment } from '../../../environments/environment';

/** The logged-in user's Jira profile picture (backend proxy). */
export function myAvatarUrl(): string {
  return `${environment.apiBasePath}/api/dashboard/me/avatar`;
}

/** Any user's Jira profile picture — admin only. */
export function adminUserAvatarUrl(userId: number): string {
  return `${environment.apiBasePath}/api/dashboard/admin/users/${userId}/avatar`;
}

/**
 * Round user avatar: the user's Jira profile picture, falling back to the
 * initial of their email/name when there is none (404) or it fails to load
 * (Tech Spec "Admin Panel Revamp").
 */
@Component({
  selector: 'app-user-avatar',
  standalone: true,
  host: {
    class: 'user-avatar',
    '[class.lg]': "size() === 'lg'",
    '[class.xl]': "size() === 'xl'",
    '[class.has-photo]': 'showPhoto()',
    '[attr.aria-hidden]': 'true',
  },
  template: `
    @if (showPhoto()) {
      <img [src]="src()" alt="" loading="lazy" (error)="failedFor.set(src())" />
    } @else {
      {{ initial() }}
    }
  `,
})
export class UserAvatarComponent {
  /** Picture URL (see myAvatarUrl / adminUserAvatarUrl); null = initial only. */
  src = input<string | null>(null);
  /** Email or name — its first letter is the fallback. */
  name = input<string | null | undefined>('');
  size = input<'md' | 'lg' | 'xl'>('md');

  /** The URL whose image failed — a new src gets a fresh try. */
  protected failedFor = signal<string | null>(null);
  protected showPhoto = computed(() => !!this.src() && this.failedFor() !== this.src());
  protected initial = computed(() => (this.name() || '?').charAt(0).toUpperCase());
}
