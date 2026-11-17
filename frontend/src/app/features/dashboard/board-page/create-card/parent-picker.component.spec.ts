import { TestBed, fakeAsync, tick } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ParentInfo } from '../../../../core/services/issue-create.service';
import { ParentPickerComponent, looksLikeKey } from './parent-picker.component';

const candidates = [
  { key: 'CORE-1', summary: 'Login revamp', type: 'Story', typeIconUrl: 'https://i/story.svg', status: 'In Progress', statusCategory: 'indeterminate', subtaskCount: 3 },
  { key: 'CORE-2', summary: 'Fix rounding', type: 'Bug', typeIconUrl: '', status: 'To Do', statusCategory: 'new', subtaskCount: 0 },
];

describe('ParentPickerComponent', () => {
  let http: HttpTestingController;

  function render() {
    TestBed.configureTestingModule({ imports: [ParentPickerComponent], providers: [provideHttpClient(), provideHttpClientTesting()] });
    http = TestBed.inject(HttpTestingController);
    const f = TestBed.createComponent(ParentPickerComponent);
    f.componentRef.setInput('boardId', 26);
    const emitted: (ParentInfo | null)[] = [];
    f.componentInstance.picked.subscribe((p) => emitted.push(p));
    f.detectChanges();
    const el: HTMLElement = f.nativeElement;
    const input = () => el.querySelector('[data-parent-input]') as HTMLInputElement;
    return { f, el, input, emitted };
  }

  function openWithList(r: ReturnType<typeof render>) {
    r.input().dispatchEvent(new Event('focus'));
    r.f.detectChanges();
    expect(r.el.querySelector('[data-parent-loading]')).withContext('skeleton while loading').not.toBeNull();
    const req = http.expectOne((q) => q.url.endsWith('/create/parents'));
    expect(req.request.params.get('boardId')).toBe('26');
    req.flush({ parents: candidates });
    r.f.detectChanges();
  }

  it('opens on focus and loads the sprint candidates once', () => {
    const r = render();
    expect(r.el.querySelector('[data-parent-list]')).toBeNull();
    openWithList(r);
    const opts = r.el.querySelectorAll('[data-parent-option]');
    expect(opts.length).toBe(2);
    expect(opts[0].textContent).toContain('CORE-1');
    expect(opts[0].textContent).toContain('3 subtask');
    expect(opts[0].querySelector('.pp-status')?.classList).toContain('progress');
    expect(opts[0].querySelector('img')?.getAttribute('src')).toBe('https://i/story.svg');
    // Re-opening reuses the list.
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    r.input().dispatchEvent(new Event('focus'));
    r.f.detectChanges();
    http.expectNone((q) => q.url.endsWith('/create/parents'));
  });

  it('closes on an outside click without picking anything', () => {
    const r = render();
    openWithList(r);
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    r.f.detectChanges();
    expect(r.el.querySelector('[data-parent-list]')).toBeNull();
    expect(r.emitted).toEqual([]);
    http.expectNone((q) => q.url.includes('/create/parent/'));
  });

  it('closes on Escape without picking anything', () => {
    const r = render();
    openWithList(r);
    r.input().dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    r.f.detectChanges();
    expect(r.el.querySelector('[data-parent-list]')).toBeNull();
    expect(r.emitted).toEqual([]);
  });

  it('picks via click, validates it and shows the selected card', () => {
    const r = render();
    openWithList(r);
    (r.el.querySelector('[data-parent-option="CORE-1"]') as HTMLButtonElement).click();
    r.f.detectChanges();
    expect(r.el.querySelector('[data-parent-list]')).withContext('closes right after the pick').toBeNull();
    http.expectOne((q) => q.url.endsWith('/create/parent/CORE-1')).flush({
      key: 'CORE-1', summary: 'Login revamp', type: 'Story', status: 'In Progress', statusCategory: 'indeterminate', subtasks: [{ key: 'CORE-9', summary: 'x' }],
    });
    r.f.detectChanges();
    expect(r.emitted.length).toBe(1);
    expect(r.emitted[0]?.key).toBe('CORE-1');
    const card = r.el.querySelector('[data-parent-selected]');
    expect(card?.textContent).toContain('Login revamp');
    expect(card?.textContent).toContain('1 subtask');
  });

  it('shows the server error inline when the ticket cannot be a parent', () => {
    const r = render();
    openWithList(r);
    r.input().value = 'CORE-50'; r.input().dispatchEvent(new Event('input'));
    r.input().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
    http.expectOne((q) => q.url.endsWith('/create/parent/CORE-50')).flush({ error: 'Epic tidak bisa punya subtask' }, { status: 422, statusText: 'x' });
    r.f.detectChanges();
    expect(r.el.querySelector('[data-parent-error]')?.textContent).toContain('Epic tidak bisa punya subtask');
    expect(r.el.querySelector('[data-parent-selected]')).toBeNull();
    expect(r.emitted).toEqual([]);
  });

  it('searches with a debounce, picks the highlighted option with the keyboard, and "Ganti" clears it', fakeAsync(() => {
    const r = render();
    openWithList(r);
    r.input().value = 'login'; r.input().dispatchEvent(new Event('input'));
    tick(200);
    http.expectNone((q) => q.url.endsWith('/create/parents'));
    tick(50);
    http.expectOne((q) => q.url.endsWith('/create/parents') && q.params.get('q') === 'login').flush({ parents: [] });
    r.f.detectChanges();
    expect(r.el.querySelector('[data-parent-empty]')?.textContent).toContain('Tidak ada tiket sprint yang cocok');
    r.input().value = 'fix'; r.input().dispatchEvent(new Event('input'));
    tick(250);
    http.expectOne((q) => q.params.get('q') === 'fix').flush({ parents: [candidates[1]] });
    r.f.detectChanges();
    r.input().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }));
    r.f.detectChanges();
    expect(r.el.querySelector('[data-parent-option="CORE-2"]')?.classList).toContain('active');
    r.input().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
    http.expectOne((q) => q.url.endsWith('/create/parent/CORE-2')).flush({ key: 'CORE-2', summary: 'Fix rounding', subtasks: [] });
    r.f.detectChanges();
    (r.el.querySelector('[data-parent-change]') as HTMLButtonElement).click();
    r.f.detectChanges();
    tick();
    expect(r.emitted.map((p) => p?.key ?? null)).toEqual(['CORE-2', null]);
    expect(r.el.querySelector('[data-parent-input]')).not.toBeNull();
  }));

  it('recognises issue keys', () => {
    expect(looksLikeKey(' core-12 ')).toBeTrue();
    expect(looksLikeKey('login')).toBeFalse();
  });
});
