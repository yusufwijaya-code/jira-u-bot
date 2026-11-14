import { Injectable, computed, signal } from '@angular/core';
import { MuteStatus } from './board.service';

/**
 * The caller's mute status, shared between the Pengaturan page (which
 * changes it) and the shell top bar (which shows a badge while it lasts).
 */
@Injectable({ providedIn: 'root' })
export class MuteStateService {
  private readonly status = signal<MuteStatus>({ active: false, until: null });
  /** Ticks every 30s so an expired mute disappears without a reload. */
  private readonly now = signal(Date.now());

  readonly until = computed(() => {
    const s = this.status();
    if (!s.active || !s.until) return null;
    const until = new Date(s.until);
    return until.getTime() > this.now() ? until : null;
  });
  readonly active = computed(() => this.until() !== null);
  /** "Selamanya": off until the user switches notifications back on. */
  readonly forever = computed(() => this.active() && !!this.status().forever);

  constructor() {
    setInterval(() => this.now.set(Date.now()), 30_000);
  }

  set(status: MuteStatus): void {
    this.now.set(Date.now());
    this.status.set(status);
  }
}
