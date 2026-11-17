import { Component, DestroyRef, ElementRef, HostListener, OnInit, computed, inject, input, output, signal, viewChild } from '@angular/core';
import { Observable, Subject, catchError, map, of, switchMap, timer } from 'rxjs';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { IssueCreateService, ParentCandidate, ParentInfo } from '../../../../core/services/issue-create.service';
import { IconComponent } from '../../../../shared/ui/icon.component';
import { shouldDropUp } from '../../../../shared/ui/popover-position';

let uid = 0;
const KEY_RE = /^[A-Z][A-Z0-9_]*-\d+$/;
/** List height cap (CSS max-height) used to decide whether to flip up. */
const LIST_HEIGHT = 340;

export function looksLikeKey(v: string): boolean {
  return KEY_RE.test(v.trim().toUpperCase());
}

/**
 * Combobox for a subtask parent: open-sprint tickets of the board (epics and
 * subtasks are filtered server-side), searchable by key or summary. Closes on
 * outside click, Esc, Tab/blur and after a pick — with or without a value.
 * A pick is validated by GET /create/parent/:key (server explains rejections).
 */
@Component({
  selector: 'app-parent-picker',
  standalone: true,
  imports: [IconComponent],
  templateUrl: './parent-picker.component.html',
  styleUrl: './parent-picker.component.scss',
  host: { class: 'pp', '[class.open]': 'open()', '[class.drop-up]': 'open() && dropUp()' },
})
export class ParentPickerComponent implements OnInit {
  private api = inject(IssueCreateService);
  private host = inject(ElementRef<HTMLElement>);
  boardId = input.required<number>();
  /** Key to validate and select on first render (e.g. "Buat subtask" from a card). */
  initialKey = input<string | null>(null);
  /** When set (not undefined), the shown selection follows the parent component (e.g. refreshed subtask count). */
  value = input<ParentInfo | null | undefined>(undefined);
  placeholder = input('Cari tiket sprint (key / judul) atau ketik key lalu Enter');
  picked = output<ParentInfo | null>();

  private inputEl = viewChild<ElementRef<HTMLInputElement>>('inp');
  protected query = signal('');
  protected open = signal(false);
  protected dropUp = signal(false);
  protected loading = signal(false);
  protected loadFailed = signal(false);
  protected candidates = signal<ParentCandidate[]>([]);
  protected active = signal(-1);
  protected validating = signal(false);
  protected error = signal<string | null>(null);
  private selected = signal<ParentInfo | null>(null);
  protected shown = computed(() => (this.value() === undefined ? this.selected() : this.value() ?? null));
  protected readonly listId = `pp-list-${++uid}`;

  private loaded = false;
  private search$ = new Subject<{ q: string; delay: number }>();
  private pickSub?: { unsubscribe(): void };

  constructor() {
    this.search$.pipe(
      switchMap(({ q, delay }) => {
        const req: Observable<ParentCandidate[] | null> = this.api.parents(this.boardId(), q.trim()).pipe(map((r) => r.parents ?? []), catchError(() => of(null)));
        return delay ? timer(delay).pipe(switchMap(() => req)) : req;
      }),
      takeUntilDestroyed(inject(DestroyRef)),
    ).subscribe((list) => {
      this.loading.set(false);
      this.loadFailed.set(list === null);
      this.candidates.set(list ?? []);
      this.active.set(-1);
    });
  }

  ngOnInit(): void {
    const k = this.initialKey();
    if (k) this.pick(k);
  }

  protected onFocus(): void { this.openList(); }

  protected openList(): void {
    if (this.open() || this.validating()) return;
    this.dropUp.set(shouldDropUp(this.host.nativeElement, LIST_HEIGHT));
    this.open.set(true);
    if (!this.loaded) {
      this.loaded = true;
      this.loading.set(true);
      this.search$.next({ q: this.query(), delay: 0 });
    }
  }

  protected close(): void {
    this.open.set(false);
    this.active.set(-1);
  }

  protected onInput(v: string): void {
    this.query.set(v);
    this.error.set(null);
    this.loaded = true;
    this.loading.set(true);
    this.search$.next({ q: v, delay: 250 });
    if (!this.open()) this.openList();
  }

  protected onKeydown(e: KeyboardEvent): void {
    const n = this.candidates().length;
    switch (e.key) {
      case 'ArrowDown':
      case 'ArrowUp': {
        e.preventDefault();
        if (!this.open()) { this.openList(); return; }
        if (!n) return;
        const step = e.key === 'ArrowDown' ? 1 : -1;
        const cur = this.active();
        this.active.set(cur < 0 ? (step > 0 ? 0 : n - 1) : (cur + step + n) % n);
        this.scrollActive();
        return;
      }
      case 'Enter': {
        e.preventDefault();
        const c = this.open() ? this.candidates()[this.active()] : undefined;
        if (c) this.pick(c.key);
        else if (looksLikeKey(this.query())) this.pick(this.query());
        return;
      }
      case 'Escape':
        if (this.open()) { e.preventDefault(); e.stopPropagation(); }
        this.close();
        return;
      case 'Tab':
        this.close();
        return;
    }
  }

  /** Focus leaving the picker (Tab, click on another field) closes the list. */
  protected onFocusOut(e: FocusEvent): void {
    const next = e.relatedTarget;
    if (!(next instanceof Node && this.host.nativeElement.contains(next))) this.close();
  }

  @HostListener('document:pointerdown', ['$event'])
  protected onOutside(e: Event): void {
    if (this.open() && !(e.target instanceof Node && this.host.nativeElement.contains(e.target))) this.close();
  }

  protected pick(key: string): void {
    const k = key.trim().toUpperCase();
    if (!k) return;
    this.close();
    this.query.set(k);
    this.error.set(null);
    this.validating.set(true);
    this.pickSub?.unsubscribe();
    this.pickSub = this.api.parent(this.boardId(), k).subscribe({
      next: (p) => {
        this.validating.set(false);
        this.selected.set(p);
        this.query.set('');
        this.picked.emit(p);
      },
      error: (err) => {
        this.validating.set(false);
        this.selected.set(null);
        this.error.set(err?.status === 404 ? `${k} tidak ditemukan` : err?.error?.error ?? 'Tiket ini tidak bisa jadi parent');
      },
    });
  }

  /** "Ganti": drop the selection and reopen the search, focused. */
  protected change(): void {
    this.selected.set(null);
    this.error.set(null);
    this.picked.emit(null);
    setTimeout(() => this.inputEl()?.nativeElement.focus());
  }

  protected statusClass(cat: string | undefined): string {
    return cat === 'done' ? 'done' : cat === 'indeterminate' ? 'progress' : 'todo';
  }

  private scrollActive(): void {
    const el = this.host.nativeElement.querySelector(`#${this.listId}-${this.active()}`) as HTMLElement | null;
    el?.scrollIntoView?.({ block: 'nearest' });
  }
}
