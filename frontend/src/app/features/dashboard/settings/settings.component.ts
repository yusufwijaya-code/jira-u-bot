import { Component, DestroyRef, ElementRef, Injector, afterNextRender, computed, effect, inject, signal, untracked, viewChild } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { NgTemplateOutlet } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { Subscription } from 'rxjs';
import { OtherBoard } from '../../../core/services/board-context.service';
import { NotificationSwitchState } from '../../../core/services/notification-switch.service';
import {
  MuteStatus,
  NotificationSettings,
  SettingsValidationError,
} from '../../../core/services/board.service';
import { MuteStateService } from '../../../core/services/mute-state.service';
import { NotificationSwitchService } from '../../../core/services/notification-switch.service';
import { SETTINGS_MODE, SettingsApi, SettingsScope } from '../../../core/services/settings-api';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { TrackingRuleService } from '../../../core/services/tracking-rule.service';
import { BoardAvatarComponent } from '../../../shared/ui/board-avatar.component';
import { SelectComponent } from '../../../shared/ui/select.component';
import { TimePickerComponent } from '../../../shared/ui/time-picker.component';
import { IconComponent } from '../../../shared/ui/icon.component';
import { SegIndicatorDirective } from '../../../shared/ui/seg-indicator.directive';
import { JiraCredentialComponent } from './jira-credential.component';
import { MutePanelComponent, activeUntil, formatWib } from './mute-panel.component';
import { TierPickerComponent } from './tier-picker.component';
import { ChatSenderCardComponent } from './chat-sender-card.component';
import { sortTiers, tierDuration } from './tier-codes';

/** Which card's Simpan button is running — each card saves only its own fields. */
export type SettingsCard = 'reminder' | 'overage' | 'events' | 'quiet';

/** Quiet hours suggested when the toggle is first switched on. */
export const DEFAULT_QUIET_START = '09:00';
export const DEFAULT_QUIET_END = '10:00';

const CARD_FIELDS: Record<SettingsCard, (keyof NotificationSettings)[]> = {
  reminder: ['reminderTiers'],
  overage: ['overageTiers', 'overageRepeatMinutes'],
  events: ['notifyStatusChange', 'statusChangeStatuses'],
  quiet: ['quietHours'],
};

/**
 * Personal settings for the board picked in the navbar (Tech Spec "Per-Board
 * Settings & Active Sprint Dashboard"): "Semua board" edits the default for
 * every board; a board either follows that default (read-only until "Atur
 * khusus") or has its own settings ("Kembalikan ke default"). Mute has two
 * levels: global and per board. The Jira API key is per user.
 */
@Component({
  selector: 'app-settings',
  standalone: true,
  imports: [
    ChatSenderCardComponent,
    FormsModule,
    NgTemplateOutlet,
    IconComponent,
    BoardAvatarComponent,
    SelectComponent,
    TimePickerComponent,
    JiraCredentialComponent,
    MutePanelComponent,
    TierPickerComponent,
    SegIndicatorDirective,
  ],
  templateUrl: './settings.component.html',
})
export class SettingsComponent {
  protected readonly reminderPresets = ['2h', '1h', '30m', '15m', '5m', '1m'];
  protected readonly overagePresets = ['5m', '15m', '30m', '1h', '2h'];
  protected readonly repeatOptions: { value: number | null; label: string }[] = [
    { value: 5, label: 'Ulang tiap 5 menit' },
    { value: 10, label: 'Ulang tiap 10 menit' },
    { value: 15, label: 'Ulang tiap 15 menit' },
    { value: 30, label: 'Ulang tiap 30 menit' },
    { value: 60, label: 'Ulang tiap 60 menit' },
    { value: null, label: 'Berhenti' },
  ];

  /** First load only (nothing to show yet): the skeleton cards. */
  protected loading = true;
  /**
   * Reloading while settings are already on screen (board switch, Batal,
   * reset): the old content stays, dimmed, until the new response lands, so
   * the page never unmounts to the skeleton and collapses.
   */
  protected switching = signal(false);
  /** Page height held while switching, so a shorter layout can't jump the scroll. */
  protected holdHeight = signal<number | null>(null);
  private host = inject<ElementRef<HTMLElement>>(ElementRef);
  private releaseHoldTimer?: ReturnType<typeof setTimeout>;
  protected error: string | null = null;
  protected hasCredential = false;
  protected credentialMasked: string | null = null;

  protected draft = signal<NotificationSettings | null>(null);
  /** The settings as last loaded / saved — the base each card's save starts from. */
  private saved: NotificationSettings | null = null;
  /** The in-flight GET /settings, cancelled when the scope changes again. */
  private loadSub?: Subscription;
  /** Scope of the in-flight GET /settings (undefined = none). */
  private loadingBoard: number | null | undefined;
  /** Admin mode: the target user's master switch (read-only here). */
  protected targetSwitch = signal<NotificationSwitchState | null>(null);
  /** Boards outside the active sprints that still have custom settings or a mute. */
  protected otherBoardBusy = signal<number | null>(null);
  /** Scope of the loaded settings (board id, null = default). */
  protected scopeBoardId = signal<number | null>(null);
  /**
   * Umum = what applies to every board (master switch, sender, Jira key);
   * Per board = notification settings of the default / one board. Opens on
   * "Per board" when a board is already picked in the navbar.
   */
  protected tab = signal<SettingsTab>(inject(SettingsScope).selectedId() === null ? 'umum' : 'board');
  /** Panels opened so far: each is mounted on first visit and then only hidden, never destroyed. */
  protected visited = signal<Record<SettingsTab, boolean>>({ umum: this.tab() === 'umum', board: this.tab() === 'board' });
  /** The user switched tabs at least once: only then does the incoming panel cross-fade (not on arrival). */
  protected tabSwitched = signal(false);
  /** Side the incoming tab panel slides in from (Per board is on the right). */
  protected tabSlide = signal<string | null>(null);
  /**
   * After a board switch lands: the board panel's content cross-fades in from
   * the side of the newly picked board ('a'/'b' alternate so it replays).
   */
  protected boardIn = signal<'a' | 'b' | null>(null);
  protected boardSlide = signal<string | null>(null);
  private boardRuns = 0;
  /** Direction of the board switch in flight (+1 = a board to the right), null = same board. */
  private pendingBoardDir: number | null = null;
  private boardInTimer?: ReturnType<typeof setTimeout>;
  private tabBar = viewChild<ElementRef<HTMLElement>>('tabBar');
  private injector = inject(Injector);
  protected useDefault = signal(false);
  /** "Atur khusus board ini" was clicked: editable until saved / cancelled. */
  protected editingCustom = signal(false);
  protected boardMute = signal<MuteStatus | null>(null);
  protected globalMute = signal<MuteStatus | null>(null);
  protected resetting = signal(false);
  protected saving = signal<SettingsCard | null>(null);
  protected fieldErrors = signal<Partial<Record<keyof NotificationSettings, string>>>({});
  /** Values for the quiet hours inputs, kept while the toggle is off. */
  protected quietStart = DEFAULT_QUIET_START;
  protected quietEnd = DEFAULT_QUIET_END;

  /** "Budget habis → +15 menit → +1 jam → tiap 30 menit". */
  protected overagePreview = computed(() => {
    const d = this.draft();
    if (!d) return '';
    const steps = ['Budget habis', ...sortTiers(d.overageTiers, 'asc').map((code) => `+${tierDuration(code)}`)];
    steps.push(d.overageRepeatMinutes === null ? 'berhenti' : `tiap ${d.overageRepeatMinutes} menit`);
    return steps.join(' → ');
  });

  private boardService = inject(SettingsApi);
  private muteState = inject(MuteStateService);
  /** Navbar board picker, or the page-local scope on Admin › User detail. */
  protected boardContext = inject(SettingsScope);
  /** An admin editing another user (Admin › User detail). */
  protected readonly isAdminMode = inject(SETTINGS_MODE) === 'admin';
  private confirm = inject(ConfirmService);
  private toast = inject(ToastService);
  /** The caller's master on/off switch (not shown in admin mode). */
  protected notifSwitch = inject(NotificationSwitchService);
  protected savingSwitch = signal(false);

  /** Master switch (Tech Spec "Notification On-Off Consent"): saved right away. */
  protected toggleNotifications(input: HTMLInputElement): void {
    const enabled = input.checked;
    this.savingSwitch.set(true);
    this.notifSwitch.save(enabled).subscribe({
      next: () => {
        this.savingSwitch.set(false);
        this.toast.success(
          enabled ? 'Notifikasi aktif' : 'Notifikasi dimatikan',
          enabled ? 'Nudge akan mengingatkan kamu lewat Google Chat.' : 'Tidak ada notif otomatis yang dikirim.',
        );
      },
      error: () => {
        this.savingSwitch.set(false);
        input.checked = !enabled;
        this.toast.error('Gagal menyimpan', 'Pilihan notifikasi belum tersimpan, coba lagi.');
      },
    });
  }

  /** The board being configured (null = default for every board). */
  protected scopeBoard = computed(() => {
    const id = this.scopeBoardId();
    return id === null ? null : this.boardContext.boards().find((b) => b.boardId === id) ?? null;
  });
  protected scopeName = computed(() => this.scopeBoard()?.name ?? (this.scopeBoardId() ? `Board #${this.scopeBoardId()}` : ''));
  /** Notification cards are read-only while a board follows the default. */
  protected readOnly = computed(() => this.scopeBoardId() !== null && this.useDefault() && !this.editingCustom());
  protected globalMuteUntil = computed(() => activeUntil(this.globalMute()));

  /**
   * Which project rule the scope follows: 'core' tracks review time and is
   * "done" at Code Completed, 'other' is done at Code Review, 'mixed' is the
   * default scope (or a board not in the picker) covering both.
   */
  protected statusRule = computed<'core' | 'other' | 'mixed'>(() => {
    const board = this.scopeBoard();
    if (this.scopeBoardId() === null || !board) return 'mixed';
    return board.reviewTracked ? 'core' : 'other';
  });
  /** Destination statuses offered for the status-change filter, from the scope's project workflow. */
  private trackingRules = inject(TrackingRuleService);
  /** The scope board's workflow statuses from Jira; null = default scope, loading or Jira failed. */
  protected jiraStatuses = signal<string[] | null>(null);
  private statusesSub?: Subscription;
  private destroyRef = inject(DestroyRef);
  /** Destination statuses for the status-change filter: the board's Jira workflow, else the static lists. */
  protected statusOptions = computed(() => this.jiraStatuses() ?? statusOptionsFor(this.statusRule()));

  protected trackedLabel = computed(() => {
    switch (this.statusRule()) {
      case 'core':
        return 'Code Process atau CODE REVIEW';
      case 'other':
        return 'Code Process';
      default:
        return 'status yang dihitung waktunya';
    }
  });

  /** Empty filter = every status. */
  protected isStatusPicked(d: NotificationSettings, status: string): boolean {
    const picked = d.statusChangeStatuses ?? [];
    return picked.length === 0 || picked.some((s) => s.toLowerCase() === status.toLowerCase());
  }

  protected toggleStatus(d: NotificationSettings, status: string): void {
    const all = this.statusOptions();
    const current = d.statusChangeStatuses ?? [];
    const picked = current.length ? [...current] : [...all];
    const i = picked.findIndex((s) => s.toLowerCase() === status.toLowerCase());
    if (i >= 0) picked.splice(i, 1);
    else picked.push(status);
    // Keep at least one — switching everything off is the main toggle's job.
    if (picked.length === 0) return;
    // Every option picked again → back to "every status" (also covers unlisted ones).
    const everything = all.every((o) => picked.some((p) => p.toLowerCase() === o.toLowerCase()));
    this.patch({ statusChangeStatuses: everything ? [] : picked });
  }

  protected pickAllStatuses(): void {
    this.patch({ statusChangeStatuses: [] });
  }

  constructor() {
    inject(DestroyRef).onDestroy(() => {
      clearTimeout(this.releaseHoldTimer);
      clearTimeout(this.boardInTimer);
    });
    // Reload whenever the navbar board changes — but not again for a
    // selection re-emitted while that same board is still loading (arrival).
    effect(() => {
      const boardId = this.boardContext.selectedId();
      untracked(() => {
        if (this.loadingBoard !== undefined && this.loadingBoard === boardId) return;
        this.loadSettings(boardId);
      });
    });
    // Pengaturan › Notif perubahan status: the board's real workflow (AC5.5).
    effect(() => {
      const boardId = this.scopeBoardId();
      untracked(() => {
        // Arriving on a board: the initial "no scope yet" must not cancel
        // the statuses prefetched with that board's settings.
        if (boardId === null && this.loadingBoard) return;
        this.loadBoardStatuses(boardId);
      });
    });
  }

  /** Workflow statuses per board, kept for the page's lifetime (board switches show them at once). */
  private statusCache = new Map<number, string[] | null>();
  /** Board whose statuses are being fetched. */
  private statusesBoard: number | null = null;

  /**
   * Loads the board's workflow statuses — started together with the settings
   * (not after them) so the status filter rarely changes size after the
   * cards appear, and served from the cache on a revisit.
   */
  private loadBoardStatuses(boardId: number | null): void {
    if (boardId !== null && this.statusCache.has(boardId)) {
      this.statusesSub?.unsubscribe();
      this.statusesBoard = null;
      if (this.scopeBoardId() === boardId) this.jiraStatuses.set(this.statusCache.get(boardId) ?? null);
      return;
    }
    if (boardId === null) {
      this.statusesSub?.unsubscribe();
      this.statusesBoard = null;
      this.jiraStatuses.set(null);
      return;
    }
    // Prefetched with the settings: the scope now shows this board, so drop
    // the previous board's list until its own arrives.
    if (boardId === this.statusesBoard && this.statusesSub && !this.statusesSub.closed) {
      if (this.scopeBoardId() === boardId) this.jiraStatuses.set(null);
      return;
    }
    this.statusesSub?.unsubscribe();
    this.statusesBoard = boardId;
    // While prefetching for a board that isn't on screen yet, the current
    // list stays (the content is dimmed until the switch lands).
    if (this.scopeBoardId() === boardId) this.jiraStatuses.set(null);
    this.statusesSub = this.trackingRules.boardStatuses(boardId).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: (res) => {
        const statuses = res.statuses?.length ? res.statuses : null;
        this.statusCache.set(boardId, statuses);
        if (this.statusesBoard === boardId) this.jiraStatuses.set(statuses);
      },
      error: () => {
        if (this.statusesBoard === boardId) this.jiraStatuses.set(null);
      },
    });
  }

  /**
   * Switches Umum ↔ Per board without a jump: the panels only toggle
   * visibility (state and data kept), the incoming one cross-fades (CSS), and
   * the scroll is kept sensible — a tab bar stuck under the topbar stays where
   * it is with the new panel starting right below it; a tab bar out of view
   * is brought just under the topbar.
   */
  protected selectTab(next: SettingsTab): void {
    if (this.tab() === next) return;
    const bar = this.tabBar()?.nativeElement;
    const stack = this.host.nativeElement.firstElementChild as HTMLElement | null;
    const barTop = bar?.getBoundingClientRect().top ?? 0;
    // The bar is stuck when the page is scrolled past its natural spot (the top of the stack).
    const stuck = !!bar && !!stack && stack.getBoundingClientRect().top < barTop - 1;
    const inView = !!bar && barTop >= 0 && barTop < window.innerHeight;

    this.visited.update((v) => ({ ...v, [next]: true }));
    this.tabSwitched.set(true);
    this.tabSlide.set(next === 'board' ? '16px' : '-16px');
    this.tab.set(next);
    if (!bar || !stack) return;

    afterNextRender(
      () => {
        const behavior: ScrollBehavior = prefersReducedMotion() ? 'auto' : 'smooth';
        if (stuck) {
          // Start the new panel right under the (unmoved) sticky bar.
          const delta = stack.getBoundingClientRect().top - barTop;
          if (Math.abs(delta) > 1) window.scrollBy({ top: delta, behavior });
        } else if (inView) {
          // Not stuck: the bar sits in the flow; keep it at the same viewport position.
          const delta = bar.getBoundingClientRect().top - barTop;
          if (Math.abs(delta) > 1) window.scrollBy({ top: delta, behavior: 'auto' });
        } else {
          bar.scrollIntoView({ block: 'start', behavior });
        }
      },
      { injector: this.injector },
    );
  }

  /** The board chip whose settings are loading (gets the inline spinner). */
  protected isPending(boardId: number | null): boolean {
    return this.switching() && this.boardContext.selectedId() === boardId;
  }

  protected loadSettings(boardId: number | null = this.boardContext.selectedId()): void {
    if (this.draft() && !this.error) {
      clearTimeout(this.releaseHoldTimer);
      const height = this.host.nativeElement.firstElementChild?.getBoundingClientRect().height ?? 0;
      this.holdHeight.set(height > 0 ? Math.round(height) : null);
      this.switching.set(true);
      this.pendingBoardDir = this.boardDirection(this.scopeBoardId(), boardId);
    } else {
      this.loading = true;
    }
    this.error = null;
    this.editingCustom.set(false);
    this.fieldErrors.set({});

    // Fetch the board's statuses alongside the settings, not after them.
    if (boardId !== null) this.loadBoardStatuses(boardId);

    // Switching boards quickly must never let an older response land last
    // and show (then save) another board's settings.
    this.loadSub?.unsubscribe();
    this.loadingBoard = boardId;
    this.loadSub = this.boardService.getSettings(boardId).subscribe({
      next: (res) => {
        this.loadingBoard = undefined;
        this.scopeBoardId.set(res.scope === 'board' ? res.boardId : null);
        this.useDefault.set(res.scope === 'board' && res.useDefault);
        // An older API has no status filter: treat it as "every status".
        const notification = { ...res.notification, statusChangeStatuses: res.notification.statusChangeStatuses ?? [] };
        this.saved = { ...notification };
        this.draft.set({ ...notification });
        this.quietStart = res.notification.quietHours?.start ?? DEFAULT_QUIET_START;
        this.quietEnd = res.notification.quietHours?.end ?? DEFAULT_QUIET_END;
        this.globalMute.set(res.mute.global);
        this.boardMute.set(res.mute.board);
        // The top bar badge shows the caller's own mute only.
        if (!this.isAdminMode) {
          this.muteState.set(res.mute.global);
          this.notifSwitch.set(res.notifications);
        } else {
          this.targetSwitch.set(res.notifications ?? null);
        }
        this.hasCredential = res.credential.hasCredential;
        this.credentialMasked = res.credential.maskedPreview;
        this.loading = false;
        this.playBoardIn();
        this.endSwitch();
      },
      error: () => {
        this.loadingBoard = undefined;
        this.pendingBoardDir = null;
        this.error = 'Gagal memuat pengaturan, coba lagi';
        this.loading = false;
        this.endSwitch();
      },
    });
  }

  /** +1 / -1 by the boards' order in the picker (Default first), null when the board doesn't change. */
  private boardDirection(from: number | null, to: number | null): number | null {
    if (from === to) return null;
    const order: (number | null)[] = [null, ...this.boardContext.boards().map((b) => b.boardId)];
    return order.indexOf(to) >= order.indexOf(from) ? 1 : -1;
  }

  /** The new board's settings are in: its content slides in from the picked board's side. */
  private playBoardIn(): void {
    const dir = this.pendingBoardDir;
    this.pendingBoardDir = null;
    if (dir === null || prefersReducedMotion()) return;
    this.boardRuns++;
    this.boardSlide.set(`${dir * 14}px`);
    this.boardIn.set(this.boardRuns % 2 ? 'a' : 'b');
    clearTimeout(this.boardInTimer);
    this.boardInTimer = setTimeout(() => this.boardIn.set(null), 400);
  }

  /** New content is in: fade it back in, then let the page take its natural height. */
  private endSwitch(): void {
    this.switching.set(false);
    clearTimeout(this.releaseHoldTimer);
    this.releaseHoldTimer = setTimeout(() => this.holdHeight.set(null), 260);
  }

  protected patch(changes: Partial<NotificationSettings>): void {
    const current = this.draft();
    if (!current) return;
    this.draft.set({ ...current, ...changes });
    // Editing a field clears its stale server error.
    const errors = { ...this.fieldErrors() };
    for (const key of Object.keys(changes) as (keyof NotificationSettings)[]) {
      delete errors[key];
    }
    this.fieldErrors.set(errors);
  }

  protected setQuietEnabled(enabled: boolean): void {
    this.patch({ quietHours: enabled ? { start: this.quietStart, end: this.quietEnd } : null });
  }

  protected setQuietTime(which: 'start' | 'end', value: string): void {
    if (which === 'start') this.quietStart = value;
    else this.quietEnd = value;
    if (this.draft()?.quietHours) {
      this.patch({ quietHours: { start: this.quietStart, end: this.quietEnd } });
    }
  }

  protected errorFor(card: SettingsCard): string | null {
    const errors = this.fieldErrors();
    for (const field of CARD_FIELDS[card]) {
      if (errors[field]) return errors[field]!;
    }
    return null;
  }

  protected formatWib(date: Date): string {
    return formatWib(date);
  }

  /** Start customizing a board that follows the default. */
  protected startCustom(): void {
    this.editingCustom.set(true);
  }

  /** Drop unsaved custom edits and go back to showing the default. */
  protected cancelCustom(): void {
    this.loadSettings(this.scopeBoardId());
  }

  protected async resetToDefault(): Promise<void> {
    const boardId = this.scopeBoardId();
    if (boardId === null) return;
    const confirmed = await this.confirm.ask({
      title: 'Kembalikan ke default?',
      message: `Pengaturan khusus ${this.scopeName()} dihapus dan board ini kembali mengikuti pengaturan default semua board. Mute board ini tetap.`,
      confirmText: 'Kembalikan',
      icon: 'refresh',
    });
    if (!confirmed) return;

    this.resetting.set(true);
    this.boardService.resetNotificationSettings(boardId).subscribe({
      next: () => {
        this.resetting.set(false);
        this.toast.success('Kembali ke default', `${this.scopeName()} sekarang mengikuti pengaturan default.`);
        this.boardContext.load();
        this.loadSettings(boardId);
      },
      error: () => {
        this.resetting.set(false);
        this.toast.error('Gagal mengembalikan ke default', 'Coba lagi sebentar.');
      },
    });
  }

  protected save(card: SettingsCard): void {
    const draft = this.draft();
    if (!draft || !this.saved || this.saving() || this.readOnly()) return;
    this.saving.set(card);
    const boardId = this.scopeBoardId();
    // Only this card's fields go out; unsaved edits in other cards stay
    // drafts instead of being saved along silently.
    const payload: NotificationSettings = { ...this.saved, ...pick(draft, CARD_FIELDS[card]) };

    this.boardService.saveNotificationSettings(payload, boardId).subscribe({
      next: (saved) => {
        this.saving.set(null);
        const errors = { ...this.fieldErrors() };
        for (const field of CARD_FIELDS[card]) delete errors[field];
        this.fieldErrors.set(errors);
        this.saved = { ...saved };
        const current = this.draft() ?? saved;
        this.draft.set({ ...current, ...pick(saved, CARD_FIELDS[card]) });
        if (boardId !== null) {
          const becameCustom = this.useDefault();
          this.useDefault.set(false);
          this.editingCustom.set(false);
          if (becameCustom) this.boardContext.load(); // custom dot in the picker
        }
        this.toast.success(
          'Pengaturan tersimpan',
          boardId === null
            ? 'Berlaku untuk semua board yang mengikuti default.'
            : `Berlaku khusus untuk ${this.scopeName()}.`,
        );
      },
      error: (err: HttpErrorResponse) => {
        this.saving.set(null);
        const body = err?.error as SettingsValidationError | undefined;
        if (err?.status === 400 && body?.fields) {
          this.fieldErrors.set(body.fields);
          return;
        }
        this.toast.error('Gagal menyimpan pengaturan', body?.error ?? 'Coba lagi sebentar.');
      },
    });
  }

  protected onCredentialChanged(): void {
    this.loadSettings(this.scopeBoardId());
  }

  /** "Kembalikan ke default" for a board outside the active sprints. */
  protected async resetOtherBoard(board: OtherBoard): Promise<void> {
    const confirmed = await this.confirm.ask({
      title: 'Kembalikan ke default?',
      message: `Pengaturan khusus ${board.name} dihapus. Kalau board ini aktif lagi, ia mengikuti pengaturan default semua board.`,
      confirmText: 'Kembalikan',
      icon: 'refresh',
    });
    if (!confirmed) return;
    this.otherBoardBusy.set(board.boardId);
    this.boardService.resetNotificationSettings(board.boardId).subscribe({
      next: () => {
        this.otherBoardBusy.set(null);
        this.toast.success('Kembali ke default', `${board.name} sekarang mengikuti pengaturan default.`);
        this.boardContext.load();
      },
      error: () => {
        this.otherBoardBusy.set(null);
        this.toast.error('Gagal mengembalikan ke default', 'Coba lagi sebentar.');
      },
    });
  }

  /** Ends the mute of a board outside the active sprints. */
  protected unmuteOtherBoard(board: OtherBoard): void {
    this.otherBoardBusy.set(board.boardId);
    this.boardService.unmute(board.boardId).subscribe({
      next: () => {
        this.otherBoardBusy.set(null);
        this.toast.success('Mute dibatalkan', `${board.name} tidak lagi dibungkam.`);
        this.boardContext.load();
      },
      error: () => {
        this.otherBoardBusy.set(null);
        this.toast.error('Gagal membatalkan mute', 'Coba lagi sebentar.');
      },
    });
  }

  protected otherBoardMuteLabel(board: OtherBoard): string | null {
    const until = activeUntil(board.muted);
    if (!until) return null;
    return board.muted.forever ? 'Notifikasi dimatikan' : `Dimute sampai ${formatWib(until)} WIB`;
  }
}

export type SettingsTab = 'umum' | 'board';

function prefersReducedMotion(): boolean {
  return typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

function pick(source: NotificationSettings, fields: (keyof NotificationSettings)[]): Partial<NotificationSettings> {
  const out: Partial<NotificationSettings> = {};
  for (const field of fields) {
    (out as Record<string, unknown>)[field] = source[field];
  }
  return out;
}

/** Fallback when Jira can't answer: statuses of the CORE and OMS workflows; matching is case-insensitive. */
const CORE_STATUSES = ['To Do', 'Code Process', 'CODE REVIEW', 'Code Completed', 'Feedback', 'SIT', 'TEST RUN', 'TEST COMPLETE', 'UAT', 'Release Candidate', 'Done'];
const OMS_STATUSES = ['To Do', 'Code Process', 'Code Review', 'Code Complete', 'Feedback', 'Testing & Review', 'Test Complete', 'UAT', 'Release Candidate', 'Done'];

export function statusOptionsFor(rule: 'core' | 'other' | 'mixed'): string[] {
  if (rule === 'core') return CORE_STATUSES;
  if (rule === 'other') return OMS_STATUSES;
  const out: string[] = [];
  for (const s of [...CORE_STATUSES, ...OMS_STATUSES]) {
    if (!out.some((o) => o.toLowerCase() === s.toLowerCase())) out.push(s);
  }
  return out;
}
