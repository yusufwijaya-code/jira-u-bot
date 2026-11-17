import { Component, DestroyRef, ElementRef, HostListener, computed, inject, input, output, signal } from '@angular/core';
import { Subject, debounceTime, distinctUntilChanged, switchMap, of, catchError, map } from 'rxjs';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { IssueCreateService, JiraUser } from '../../../../core/services/issue-create.service';
import { shouldDropUp } from '../../../../shared/ui/popover-position';

/**
 * "Saya" by default; typing ≥2 chars searches assignable Jira users of the
 * board's project. Same popover behaviour as the parent picker: closes on
 * outside click, Esc, Tab/blur and after a pick; flips up near the bottom.
 */
@Component({
  selector: 'app-user-picker',
  standalone: true,
  host: { class: 'up', '[class.open]': 'open()', '[class.drop-up]': 'open() && dropUp()' },
  template: `
    <div class="up-box">
      <input type="search" role="combobox" autocomplete="off" [attr.aria-expanded]="open()" [placeholder]="label()" [value]="text()"
        (input)="onInput($any($event.target).value)" (focus)="openList()" (click)="openList()" (blur)="close()" (keydown)="onKeydown($event)"
        data-user-input [disabled]="disabled()" />
      @if (open()) {
        <ul class="up-list" role="listbox" (mousedown)="$event.preventDefault()">
          @for (o of options(); track o?.accountId ?? 'me'; let i = $index) {
            <li><button type="button" tabindex="-1" role="option" [class.active]="i === active()" [attr.aria-selected]="i === active()"
              [attr.data-user]="o?.accountId ?? null" (mouseenter)="active.set(i)" (click)="pick(o)">
              @if (o) {
                @if (o.avatarUrl) { <img [src]="o.avatarUrl" alt="" width="20" height="20" /> } @else { <span class="up-av">{{ o.displayName.charAt(0) }}</span> }
                {{ o.displayName }}
              } @else { <span class="up-av me">★</span> Saya }
            </button></li>
          }
        </ul>
      }
    </div>`,
  styles: [`
    :host { position: relative; display: block; min-width: 0; }
    .up-box { position: relative; }
    input { width: 100%; font: inherit; font-size: .86rem; padding: 7px 10px; border: 1px solid var(--border); border-radius: 8px; background: var(--surface); color: var(--text);
      transition: border-color .15s var(--ease), box-shadow .15s var(--ease); }
    input::placeholder { color: var(--text-muted); }
    input:hover:not(:disabled) { border-color: var(--border-strong); }
    input:focus { outline: 0; border-color: var(--primary-light); box-shadow: 0 0 0 3px var(--primary-ring); }
    input:disabled { background: var(--surface-2); color: var(--text-faint); cursor: not-allowed; }
    .up-list { position: absolute; z-index: 40; top: calc(100% + 6px); left: 0; right: 0; min-width: 180px; max-height: 260px; overflow-y: auto; margin: 0; padding: 5px; list-style: none;
      background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-sm); box-shadow: var(--shadow-lg);
      transform-origin: top; animation: pop-in 160ms var(--ease); }
    :host(.drop-up) .up-list { top: auto; bottom: calc(100% + 6px); transform-origin: bottom; animation-name: pop-up-in; }
    button { display: flex; align-items: center; gap: 8px; width: 100%; text-align: left; border: 0; background: none; padding: 7px 8px; font: inherit; font-size: .86rem;
      color: var(--text); cursor: pointer; border-radius: 7px; transition: background-color var(--fast) var(--ease); }
    button.active { background: var(--primary-soft); }
    img, .up-av { flex: none; width: 20px; height: 20px; border-radius: 50%; }
    .up-av { display: inline-grid; place-items: center; background: var(--surface-2); color: var(--text-muted); font-size: .7rem; font-weight: 700; box-shadow: inset 0 0 0 1px var(--border); }
    .up-av.me { background: var(--primary-soft); color: var(--primary); box-shadow: none; }
    @media (prefers-reduced-motion: reduce) { .up-list { animation: none; } input, button { transition: none; } }
  `],
})
export class UserPickerComponent {
  private api = inject(IssueCreateService);
  private host = inject(ElementRef<HTMLElement>);
  boardId = input.required<number>();
  disabled = input(false);
  picked = output<string | null>();
  protected text = signal('');
  protected label = signal('Saya');
  protected open = signal(false);
  protected dropUp = signal(false);
  protected active = signal(-1);
  protected results = signal<JiraUser[]>([]);
  /** null = "Saya". */
  protected options = computed<(JiraUser | null)[]>(() => [null, ...this.results()]);
  private q = new Subject<string>();

  constructor() {
    this.q.pipe(
      debounceTime(250), distinctUntilChanged(),
      switchMap((q) => (q.trim().length < 2 ? of([]) : this.api.users(this.boardId(), q).pipe(map((r) => r.users ?? []), catchError(() => of([]))))),
      takeUntilDestroyed(inject(DestroyRef)),
    ).subscribe((u) => { this.results.set(u); this.active.set(-1); });
  }

  protected openList(): void {
    if (this.open() || this.disabled()) return;
    this.dropUp.set(shouldDropUp(this.host.nativeElement, 260));
    this.open.set(true);
  }

  protected close(): void {
    this.open.set(false);
    this.active.set(-1);
  }

  protected onInput(v: string): void {
    this.text.set(v);
    this.openList();
    this.q.next(v);
    if (v === '') { this.label.set('Saya'); this.picked.emit(null); }
  }

  protected onKeydown(e: KeyboardEvent): void {
    const n = this.options().length;
    switch (e.key) {
      case 'ArrowDown':
      case 'ArrowUp':
        e.preventDefault();
        if (!this.open()) { this.openList(); return; }
        this.active.set(this.active() < 0 && e.key === 'ArrowUp' ? n - 1 : (this.active() + (e.key === 'ArrowDown' ? 1 : -1) + n) % n);
        return;
      case 'Enter':
        if (this.open() && this.active() >= 0) { e.preventDefault(); this.pick(this.options()[this.active()]); }
        return;
      case 'Escape':
        if (this.open()) { e.preventDefault(); e.stopPropagation(); }
        this.close();
        return;
      case 'Tab':
        this.close();
        return;
    }
  }

  @HostListener('document:pointerdown', ['$event'])
  protected onOutside(e: Event): void {
    if (this.open() && !(e.target instanceof Node && this.host.nativeElement.contains(e.target))) this.close();
  }

  protected pick(u: JiraUser | null): void {
    this.text.set(u ? u.displayName : '');
    this.label.set(u ? u.displayName : 'Saya');
    this.close();
    this.picked.emit(u ? u.accountId : null);
  }
}
