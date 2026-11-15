import { Injectable, computed, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { NavigationEnd, Router } from '@angular/router';
import { filter } from 'rxjs';
import { environment } from '../../../environments/environment';
import { MuteStatus } from './board.service';
import { ToastService } from '../ui/toast.service';

/** One entry of the navbar board picker (GET /api/dashboard/boards). */
export interface BoardSummary {
  boardId: number;
  name: string;
  projectKey: string;
  activeSprint: { id: number; name: string; startDate: string; endDate: string };
  ticketCount: number;
  hasCustomSettings: boolean;
  muted: MuteStatus;
  /** The board's project counts code review time (CORE) → "Sedang di-review" card. */
  reviewTracked?: boolean;
}

/**
 * A board outside the user's active sprints that still has custom settings
 * or a running mute (GET /boards → otherBoards). Not in the navbar picker;
 * Pengaturan lists it so it can be undone.
 */
export interface OtherBoard {
  boardId: number;
  name: string;
  projectKey: string;
  hasCustomSettings: boolean;
  muted: MuteStatus;
}

/** GET /api/dashboard/boards (also the admin per-user variant). */
export interface BoardsResponse {
  boards: BoardSummary[] | null;
  otherBoards?: OtherBoard[] | null;
}

export const BOARD_STORAGE_KEY = 'nudge.board';

/**
 * Which Jira board the dashboard and Pengaturan are scoped to (Tech Spec
 * "Per-Board Settings & Active Sprint Dashboard"). `selectedId` null means
 * "Semua board" (dashboard) / the default settings (Pengaturan). The choice
 * lives in the URL (?board=) and localStorage.
 */
@Injectable({ providedIn: 'root' })
export class BoardContextService {
  private http = inject(HttpClient);
  private router = inject(Router);
  private toast = inject(ToastService);

  readonly boards = signal<BoardSummary[]>([]);
  readonly otherBoards = signal<OtherBoard[]>([]);
  readonly loaded = signal(false);
  readonly selectedId = signal<number | null>(null);
  readonly selected = computed(() => this.boards().find((b) => b.boardId === this.selectedId()) ?? null);

  constructor() {
    this.selectedId.set(this.fromUrl() ?? this.fromStorage());
    // Back/forward or a pasted link with ?board= wins over the stored choice.
    this.router.events.pipe(filter((e) => e instanceof NavigationEnd)).subscribe(() => {
      const fromUrl = this.fromUrl();
      if (fromUrl !== undefined && fromUrl !== this.selectedId()) {
        this.selectedId.set(fromUrl);
        this.store(fromUrl);
      }
    });
  }

  /** (Re)loads the boards; a selected board that's gone falls back to "Semua board". */
  load(): void {
    this.http.get<BoardsResponse>(`${environment.apiBasePath}/api/dashboard/boards`).subscribe({
      next: (res) => {
        this.boards.set(res.boards ?? []);
        this.otherBoards.set(res.otherBoards ?? []);
        this.loaded.set(true);
        const id = this.selectedId();
        if (id !== null && !this.boards().some((b) => b.boardId === id)) {
          this.select(null);
          this.toast.info('Board tidak lagi aktif', 'Sprint aktif board itu tidak berisi tiket kamu, jadi beralih ke Semua board.');
        }
      },
      error: () => this.loaded.set(true),
    });
  }

  select(boardId: number | null): void {
    this.selectedId.set(boardId);
    this.store(boardId);
    this.router.navigate([], {
      queryParams: { board: boardId ?? null },
      queryParamsHandling: 'merge',
      replaceUrl: true,
    });
  }

  avatarUrl(boardId: number): string {
    return `${environment.apiBasePath}/api/dashboard/boards/${boardId}/avatar`;
  }

  /** undefined = no ?board= in the URL; null = explicit "all". */
  private fromUrl(): number | null | undefined {
    const raw = this.router.parseUrl(this.router.url).queryParamMap.get('board');
    if (raw === null) return undefined;
    const id = Number(raw);
    return Number.isInteger(id) && id > 0 ? id : null;
  }

  private fromStorage(): number | null {
    try {
      const id = Number(localStorage.getItem(BOARD_STORAGE_KEY));
      return Number.isInteger(id) && id > 0 ? id : null;
    } catch {
      return null;
    }
  }

  private store(boardId: number | null): void {
    try {
      if (boardId) localStorage.setItem(BOARD_STORAGE_KEY, String(boardId));
      else localStorage.removeItem(BOARD_STORAGE_KEY);
    } catch {
      /* storage unavailable — the URL still carries the choice */
    }
  }
}
