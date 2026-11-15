import { Component, DestroyRef, inject, input, output, signal } from '@angular/core';
import { Subject, debounceTime, distinctUntilChanged, switchMap, of, catchError, map } from 'rxjs';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { IssueCreateService, JiraUser } from '../../../../core/services/issue-create.service';

/** "Saya" by default; typing ≥2 chars searches assignable Jira users of the board's project. */
@Component({
  selector: 'app-user-picker',
  standalone: true,
  template: `
    <div class="up">
      <input type="search" [placeholder]="label()" [value]="text()" (input)="onInput($any($event.target).value)" (focus)="open.set(true)" data-user-input />
      @if (open() && results().length) {
        <ul class="up-list" role="listbox">
          <li><button type="button" (click)="pick(null)">Saya</button></li>
          @for (u of results(); track u.accountId) {
            <li><button type="button" [attr.data-user]="u.accountId" (click)="pick(u)">{{ u.displayName }}</button></li>
          }
        </ul>
      }
    </div>`,
  styles: [`.up{position:relative}.up input{width:100%}.up-list{position:absolute;z-index:5;left:0;right:0;margin:2px 0 0;padding:4px;list-style:none;background:var(--surface);border:1px solid var(--border);border-radius:var(--radius-sm);box-shadow:0 8px 24px rgba(0,0,0,.08)}.up-list button{width:100%;text-align:left;border:0;background:none;padding:6px 8px;font:inherit;cursor:pointer;border-radius:6px}.up-list button:hover{background:var(--surface-muted,rgba(0,0,0,.05))}`],
})
export class UserPickerComponent {
  private api = inject(IssueCreateService);
  boardId = input.required<number>();
  picked = output<string | null>();
  protected text = signal('');
  protected label = signal('Saya');
  protected open = signal(false);
  protected results = signal<JiraUser[]>([]);
  private q = new Subject<string>();

  constructor() {
    this.q.pipe(
      debounceTime(250), distinctUntilChanged(),
      switchMap((q) => (q.trim().length < 2 ? of([]) : this.api.users(this.boardId(), q).pipe(map((r) => r.users ?? []), catchError(() => of([]))))),
      takeUntilDestroyed(inject(DestroyRef)),
    ).subscribe((u) => this.results.set(u));
  }

  protected onInput(v: string): void { this.text.set(v); this.open.set(true); this.q.next(v); }

  protected pick(u: JiraUser | null): void {
    this.text.set(u ? u.displayName : '');
    this.label.set(u ? u.displayName : 'Saya');
    this.open.set(false);
    this.picked.emit(u ? u.accountId : null);
  }
}
