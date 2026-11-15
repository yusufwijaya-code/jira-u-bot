import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { ToastService } from '../../../core/ui/toast.service';
import { BoardPageComponent, BRANCHES_STORAGE_KEY, COLLAPSED_STORAGE_KEY, shortDuration, statusTone } from './board-page.component';

describe('BoardPageComponent', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    localStorage.removeItem(COLLAPSED_STORAGE_KEY);
    localStorage.removeItem(BRANCHES_STORAGE_KEY);
    TestBed.configureTestingModule({
      imports: [BoardPageComponent],
      providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  function render() {
    const fixture = TestBed.createComponent(BoardPageComponent);
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({
      columns: [
        {
          status: 'To Do',
          cards: [
            { issueKey: 'OMS-1', summary: 'Lock payments', status: 'To Do', issueURL: 'https://x/browse/OMS-1', boardId: 28, storyPoints: 2 },
            {
              issueKey: 'OMS-2', summary: 'Refund flow', status: 'To Do', issueURL: 'https://x/browse/OMS-2', boardId: 28, storyPoints: 1,
              hasBudgetDetail: true, elapsedSeconds: 4500, budgetSeconds: 3600, parentKey: 'OMS-9', parentSummary: 'Payments epic',
            },
          ],
        },
        { status: 'Code Process', cards: [] },
      ],
    });
    fixture.detectChanges();
    return fixture;
  }

  function openMenu(fixture: ReturnType<typeof render>) {
    const el: HTMLElement = fixture.nativeElement;
    (el.querySelector('[data-card="OMS-1"] [data-status-chip]') as HTMLButtonElement).click();
    fixture.detectChanges();
    return http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transitions'));
  }

  it('renders one column per status with its cards', () => {
    const el: HTMLElement = render().nativeElement;
    expect(el.querySelectorAll('[data-column]').length).toBe(2);
    expect(el.querySelector('[data-card="OMS-1"]')?.textContent).toContain('Lock payments');
    expect(el.querySelector('[data-card="OMS-1"] .ticket-key')?.getAttribute('href')).toBe('https://x/browse/OMS-1');
  });

  it('lists only the moves Jira offers and moves the ticket', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    openMenu(fixture).flush({
      issueKey: 'OMS-1',
      currentStatus: 'To Do',
      transitions: [{ id: '11', name: 'Start Development', toStatus: 'Code Process' }],
    });
    fixture.detectChanges();

    const options = el.querySelectorAll('[data-transition]');
    expect(options.length).toBe(1);
    expect(options[0].textContent).toContain('Start Development');
    expect(options[0].textContent).toContain('Code Process');

    (options[0] as HTMLButtonElement).click();
    const post = http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transition'));
    expect(post.request.body).toEqual({ transitionId: '11' });
    post.flush({ message: 'ok', status: 'Code Process' });
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({ columns: [] });
  });

  it('says so when Jira offers no transition', () => {
    const fixture = render();
    openMenu(fixture).flush({ issueKey: 'OMS-1', currentStatus: 'To Do', transitions: [] });
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Tidak ada transisi');
  });

  it('closes the status menu on a click anywhere outside it, even inside the card', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    openMenu(fixture).flush({ issueKey: 'OMS-1', currentStatus: 'To Do', transitions: [] });
    fixture.detectChanges();

    // Inside the menu: stays open.
    el.querySelector('.status-dropdown')!.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    fixture.detectChanges();
    expect(el.querySelector('.status-dropdown')).not.toBeNull();

    // Elsewhere in the same card row: closes.
    el.querySelector('[data-card="OMS-1"] td')!.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    fixture.detectChanges();
    expect(el.querySelector('.status-dropdown')).toBeNull();
  });

  it('closes the status menu on Escape', () => {
    const fixture = render();
    openMenu(fixture).flush({ issueKey: 'OMS-1', currentStatus: 'To Do', transitions: [] });
    fixture.detectChanges();
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('.status-dropdown')).toBeNull();
  });

  it('shows the error when the transitions cannot be loaded', () => {
    const fixture = render();
    openMenu(fixture).flush({ error: 'Tiket ini bukan tiket kamu di sprint aktif.' }, { status: 403, statusText: 'Forbidden' });
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('bukan tiket kamu');
  });

  it('renders each ticket as a table row with compact budget time', () => {
    const el: HTMLElement = render().nativeElement;
    const rows = el.querySelectorAll('[data-status="To Do"] tbody tr');
    expect(rows.length).toBe(2);
    const over = el.querySelector('[data-card="OMS-2"] [data-time]')!;
    expect(over.textContent).toContain('1j 15m / 1j');
    expect(over.textContent).toContain('lewat 15m');
    expect(over.classList).toContain('over');
    expect(el.querySelector('[data-card="OMS-2"]')!.textContent).toContain('↳ OMS-9 · Payments epic');
    expect(el.querySelector('[data-card="OMS-1"]')!.textContent).toContain('—');
  });

  it('collapses empty statuses by default and remembers a toggle', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    const isOpen = (status: string) => el.querySelector(`[data-status="${status}"] [data-collapse]`)!.classList.contains('open');
    expect(isOpen('Code Process')).toBeFalse();
    expect(el.querySelector('[data-status="Code Process"] [data-collapse]')!.hasAttribute('inert')).toBeTrue();

    (el.querySelector('[data-status="Code Process"] [data-section-toggle]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(isOpen('Code Process')).toBeTrue();
    expect(el.querySelector('[data-status="Code Process"] .tb-empty')?.textContent).toContain('Tidak ada tiket');

    const todoToggle = el.querySelector('[data-status="To Do"] [data-section-toggle]') as HTMLButtonElement;
    todoToggle.click();
    fixture.detectChanges();
    expect(isOpen('To Do')).toBeFalse();
    expect(JSON.parse(localStorage.getItem(COLLAPSED_STORAGE_KEY)!)).toEqual({ 'Code Process': false, 'To Do': true });
  });

  it('opens and closes every status at once', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    const btn = el.querySelector('[data-toggle-all]') as HTMLButtonElement;
    const openCount = () => el.querySelectorAll('[data-collapse].open').length;
    expect(openCount()).toBe(1);
    expect(btn.textContent).toContain('Tutup semua');

    btn.click();
    fixture.detectChanges();
    expect(openCount()).toBe(0);
    expect(btn.textContent).toContain('Buka semua');
    expect(JSON.parse(localStorage.getItem(COLLAPSED_STORAGE_KEY)!)).toEqual({ 'To Do': true, 'Code Process': true });

    btn.click();
    fixture.detectChanges();
    expect(openCount()).toBe(2);
    expect(btn.textContent).toContain('Tutup semua');
  });

  it('filters rows by key or summary and hides sections without a match', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    const input = el.querySelector('[data-search]') as HTMLInputElement;
    input.value = 'refund';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();

    expect(el.querySelectorAll('[data-column]').length).toBe(1);
    expect(el.querySelector('[data-card="OMS-1"]')).toBeNull();
    expect(el.querySelector('[data-card="OMS-2"]')).not.toBeNull();

    input.value = 'zzz';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(el.textContent).toContain('Tidak ada tiket yang cocok');
  });

  it('shows a summary pill per status with its count', () => {
    const el: HTMLElement = render().nativeElement;
    const pills = Array.from(el.querySelectorAll('[data-jump]')).map((p) => p.textContent!.replace(/\s+/g, ' ').trim());
    expect(pills).toEqual(['To Do 2', 'Code Process 0']);
  });
  function renderWithBranches() {
    const fixture = TestBed.createComponent(BoardPageComponent);
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({
      columns: [
        {
          status: 'Code Completed',
          cards: [
            {
              issueKey: 'CORE-10923', summary: 'Required Date', status: 'Code Completed', issueURL: 'https://x/browse/CORE-10923',
              boardId: 26, storyPoints: 3, hasBudgetDetail: false, timeRecorded: true, elapsedSeconds: 9000, budgetSeconds: 10800,
            },
          ],
        },
      ],
      related: [
        {
          issueKey: 'CORE-10924', summary: 'Backend API', status: 'Code Process', issueURL: 'https://x/browse/CORE-10924',
          boardId: 26, storyPoints: 1, hasBudgetDetail: true, elapsedSeconds: 1800, budgetSeconds: 3600,
          parentKey: 'CORE-10923', parentSummary: 'Required Date', relation: 'subtask',
        },
        { issueKey: 'CORE-777', summary: 'Orphan', status: 'To Do', issueURL: '', boardId: 26, storyPoints: 0, hasBudgetDetail: false, parentKey: 'CORE-1' },
      ],
    });
    fixture.detectChanges();
    return fixture;
  }

  it('hangs out-of-sprint subtasks under their parent row with a movable status', () => {
    const el: HTMLElement = renderWithBranches().nativeElement;
    const branch = el.querySelector('[data-branch="CORE-10924"]') as HTMLElement;
    expect(branch.getAttribute('data-branch-of')).toBe('CORE-10923');
    expect(branch.previousElementSibling?.getAttribute('data-card')).toBe('CORE-10923');
    expect(branch.textContent).toContain('subtask');
    expect(branch.textContent).toContain('30m / 1j');
    expect(branch.querySelector('[data-status-chip]')).not.toBeNull();
    expect(el.querySelector('[data-branch="CORE-777"]')).toBeNull();
    expect(el.querySelector('.tb-hint')?.textContent).toContain('+ 1 subtask');
    // branches are not counted per status
    expect(el.querySelector('[data-jump]')?.textContent?.replace(/s+/g, ' ').trim()).toBe('Code Completed 1');
  });

  it('shows SP and the recorded time of a finished ticket', () => {
    const el: HTMLElement = renderWithBranches().nativeElement;
    const row = el.querySelector('[data-card="CORE-10923"]') as HTMLElement;
    expect(row.querySelector('.tb-num')?.textContent?.trim()).toBe('3');
    const time = row.querySelector('[data-time]') as HTMLElement;
    expect(time.textContent).toContain('2j 30m / 3j');
    expect(time.textContent).toContain('di bawah budget 30m');
    expect(time.classList).toContain('stopped');
  });

  it('keeps a parent row when only its branch matches the search', () => {
    const fixture = renderWithBranches();
    const el: HTMLElement = fixture.nativeElement;
    const input = el.querySelector('[data-search]') as HTMLInputElement;
    input.value = 'backend';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(el.querySelector('[data-card="CORE-10923"]')).not.toBeNull();
    expect(el.querySelector('[data-branch="CORE-10924"]')).not.toBeNull();
  });

  it('shows a skeleton until the first answer arrives', () => {
    const fixture = TestBed.createComponent(BoardPageComponent);
    fixture.detectChanges();
    const el: HTMLElement = fixture.nativeElement;
    expect(el.querySelector('[data-skeleton]')).not.toBeNull();
    expect(el.querySelectorAll('.tb-sk-row').length).toBe(6);
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({ columns: [{ status: 'To Do', cards: [] }] });
    fixture.detectChanges();
    expect(el.querySelector('[data-skeleton]')).toBeNull();
  });

  it('keeps the table on screen with a progress bar while reloading', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    (fixture.componentInstance as any).load();
    fixture.detectChanges();
    expect(el.querySelector('[data-refreshing]')).not.toBeNull();
    expect(el.querySelector('[data-card="OMS-1"]')).not.toBeNull();
    expect(el.querySelector('[data-skeleton]')).toBeNull();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({ columns: [{ status: 'To Do', cards: [] }] });
    fixture.detectChanges();
    expect(el.querySelector('[data-refreshing]')).toBeNull();
  });

  it('keeps the tickets and toasts when a reload fails', () => {
    const fixture = render();
    const toast = TestBed.inject(ToastService);
    spyOn(toast, 'error');
    (fixture.componentInstance as any).load();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({}, { status: 502, statusText: 'Bad Gateway' });
    fixture.detectChanges();
    expect(toast.error).toHaveBeenCalled();
    expect((fixture.nativeElement as HTMLElement).querySelector('[data-card="OMS-1"]')).not.toBeNull();
  });

  it('shows skeleton options while transitions load', () => {
    const fixture = render();
    const req = openMenu(fixture);
    expect((fixture.nativeElement as HTMLElement).querySelector('[data-menu-loading]')).not.toBeNull();
    req.flush({ issueKey: 'OMS-1', currentStatus: 'To Do', transitions: [] });
  });

  function startMove(fixture: ReturnType<typeof render>) {
    const el: HTMLElement = fixture.nativeElement;
    (el.querySelector('[data-card="OMS-1"] [data-status-chip]') as HTMLButtonElement).click();
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transitions')).flush({ transitions: [{ id: '21', name: 'Start', toStatus: 'Code Process' }] });
    fixture.detectChanges();
    (el.querySelector('[data-transition="21"]') as HTMLButtonElement).click();
    fixture.detectChanges();
  }

  it('moves the row before Jira answers, then offers Undo', () => {
    const fixture = render();
    const toast = TestBed.inject(ToastService);
    spyOn(toast, 'show').and.callThrough();
    const el: HTMLElement = fixture.nativeElement;
    startMove(fixture);
    expect(el.querySelector('[data-status="Code Process"] [data-card="OMS-1"]')).not.toBeNull();
    expect(el.querySelector('[data-card="OMS-1"] [data-status-chip]')?.textContent).toContain('Memindah');
    http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transition')).flush({});
    fixture.detectChanges();
    expect(el.querySelector('[data-card="OMS-1"] [data-status-chip]')?.textContent).not.toContain('Memindah');
    expect(toast.show).toHaveBeenCalledWith('success', 'Status dipindah', 'OMS-1 → Code Process', 6000, jasmine.objectContaining({ label: 'Batalkan' }));
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({ columns: [] });
  });

  it('puts the row back when Jira rejects the move', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    startMove(fixture);
    http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transition')).flush({ error: 'bukan tiket kamu' }, { status: 403, statusText: 'Forbidden' });
    fixture.detectChanges();
    expect(el.querySelector('[data-status="To Do"] [data-card="OMS-1"]')).not.toBeNull();
  });

  it('undo moves back when the workflow allows it', () => {
    const fixture = render();
    const cmp = fixture.componentInstance as unknown as { undo(k: string, s: string): void };
    cmp.undo('OMS-1', 'To Do');
    http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transitions')).flush({ transitions: [{ id: '11', name: 'Back', toStatus: 'TO DO' }] });
    const req = http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transition'));
    expect(req.request.body).toEqual(jasmine.objectContaining({ transitionId: '11' }));
    req.flush({});
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({ columns: [] });
  });

  it('negative undo: tells the user when the workflow has no way back', () => {
    const fixture = render();
    const toast = TestBed.inject(ToastService);
    spyOn(toast, 'error');
    (fixture.componentInstance as unknown as { undo(k: string, s: string): void }).undo('OMS-1', 'To Do');
    http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transitions')).flush({ transitions: [{ id: '31', name: 'Ship', toStatus: 'Done' }] });
    expect(toast.error).toHaveBeenCalledWith('Tidak bisa dibatalkan', jasmine.stringContaining('To Do'));
  });

  it('undo tells the user when the ticket is no longer on the board', () => {
    const fixture = render();
    const toast = TestBed.inject(ToastService);
    spyOn(toast, 'error');
    (fixture.componentInstance as unknown as { undo(k: string, s: string): void }).undo('OMS-1', 'To Do');
    (fixture.componentInstance as unknown as { columns: { set(v: unknown): void } }).columns.set([]);
    http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transitions')).flush({ transitions: [{ id: '11', name: 'Back', toStatus: 'To Do' }] });
    expect(toast.error).toHaveBeenCalledWith('Tidak bisa dibatalkan', 'Tiket tidak lagi tampil di papan.');
  });

  it('reloads the board once, only after the last in-flight move answers', () => {
    const fixture = render();
    const cmp = fixture.componentInstance as unknown as { move(c: unknown, o: unknown): void };
    const card = (k: string) => ({ issueKey: k, summary: '', status: 'To Do', issueURL: '', boardId: 28 });
    cmp.move(card('OMS-1'), { id: '21', name: 'Start', toStatus: 'Code Process' });
    cmp.move(card('OMS-2'), { id: '21', name: 'Start', toStatus: 'Code Process' });
    http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transition')).flush({});
    http.expectNone((r) => r.url.endsWith('/api/dashboard/board'));
    http.expectOne((r) => r.url.endsWith('/tickets/OMS-2/transition')).flush({});
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({ columns: [] });
  });

  it('drops a column made up for an unknown status when the move is rolled back', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    const cmp = fixture.componentInstance as unknown as { move(c: unknown, o: unknown): void };
    cmp.move({ issueKey: 'OMS-1', summary: '', status: 'To Do', issueURL: '', boardId: 28 }, { id: '99', name: 'Odd', toStatus: 'Weird' });
    fixture.detectChanges();
    expect(el.querySelector('[data-status="Weird"]')).not.toBeNull();
    http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transition')).flush({}, { status: 500, statusText: 'x' });
    fixture.detectChanges();
    expect(el.querySelector('[data-status="Weird"]')).toBeNull();
    expect(el.querySelector('[data-status="To Do"] [data-card="OMS-1"]')).not.toBeNull();
  });

  it('shows summary cards for the scope', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    expect(el.querySelector('[data-stat="tickets"]')?.textContent).toContain('2');
    expect(el.querySelector('[data-stat="points"]')?.textContent).toContain('3');
    expect(el.querySelector('[data-stat="over"]')?.textContent).toContain('1');
    expect(el.querySelector('[data-stat="done"]')?.textContent).toContain('0%');
  });

  it('filters to over-budget tickets from the chip and the summary card', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    (el.querySelector('[data-filter="over"]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(el.querySelector('[data-card="OMS-1"]')).toBeNull();
    expect(el.querySelector('[data-card="OMS-2"]')).not.toBeNull();
    (el.querySelector('[data-stat="over"]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(el.querySelector('[data-card="OMS-1"]')).not.toBeNull();
  });

  it('shows a reset button when the filter leaves nothing', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    (el.querySelector('[data-search]') as HTMLInputElement).value = 'zzz';
    el.querySelector('[data-search]')!.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    (el.querySelector('[data-reset-filter]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(el.querySelector('[data-card="OMS-1"]')).not.toBeNull();
  });

  it('focuses the search on "/" and clears it on Escape', () => {
    const fixture = render();
    const input = fixture.nativeElement.querySelector('[data-search]') as HTMLInputElement;
    document.dispatchEvent(new KeyboardEvent('keydown', { key: '/' }));
    expect(document.activeElement).toBe(input);
    input.value = 'oms';
    input.dispatchEvent(new Event('input'));
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    fixture.detectChanges();
    expect(input.value).toBe('');
  });

  function renderWithBranch() {
    const fixture = TestBed.createComponent(BoardPageComponent);
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({
      columns: [{ status: 'Code Process', cards: [
        { issueKey: 'CORE-1', summary: 'Parent', status: 'Code Process', issueURL: 'https://x/browse/CORE-1', boardId: 26, storyPoints: 5 },
      ] }],
      related: [
        { issueKey: 'CORE-2', summary: 'Child A', status: 'Done', issueURL: 'https://x/browse/CORE-2', boardId: 26, storyPoints: 1, parentKey: 'CORE-1', relation: 'subtask' },
        { issueKey: 'CORE-3', summary: 'Child B', status: 'Code Process', issueURL: 'https://x/browse/CORE-3', boardId: 26, storyPoints: 1, parentKey: 'CORE-1', relation: 'subtask' },
      ],
    });
    fixture.detectChanges();
    return fixture;
  }

  it('lets a branch row open the status menu', () => {
    const fixture = renderWithBranch();
    const chip = fixture.nativeElement.querySelector('[data-branch="CORE-3"] [data-status-chip]') as HTMLButtonElement;
    chip.click();
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/tickets/CORE-3/transitions')).flush({ transitions: [{ id: '31', name: 'Review', toStatus: 'Code Review' }] });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('[data-transition="31"]')).not.toBeNull();
  });

  it('shows subtask progress on the parent and collapses its branches', () => {
    const fixture = renderWithBranch();
    const el: HTMLElement = fixture.nativeElement;
    expect(el.querySelector('[data-branch-progress="CORE-1"]')?.textContent).toContain('1/2');
    (el.querySelector('[data-branch-toggle="CORE-1"]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(el.querySelector('[data-branch="CORE-2"]')).toBeNull();
  });

  it('copies a key and offers + Subtask only on non-branch rows', () => {
    const fixture = renderWithBranch();
    const el: HTMLElement = fixture.nativeElement;
    const clip = jasmine.createSpy('writeText').and.returnValue(Promise.resolve());
    spyOnProperty(navigator, 'clipboard').and.returnValue({ writeText: clip } as unknown as Clipboard);
    (el.querySelector('[data-card="CORE-1"] [data-copy]') as HTMLButtonElement).click();
    expect(clip).toHaveBeenCalledWith('CORE-1');
    expect(el.querySelector('[data-branch="CORE-2"] [data-add-subtask]')).toBeNull();
  });

  it('offers + Subtask on the parent row only and opens the create tab for it', () => {
    const fixture = renderWithBranch();
    const cmp = fixture.componentInstance as any;
    cmp.createCardEnabled = true;
    fixture.detectChanges();
    const el: HTMLElement = fixture.nativeElement;
    expect(el.querySelector('[data-branch="CORE-2"] [data-add-subtask]')).toBeNull();
    const btn = el.querySelector('[data-card="CORE-1"] [data-add-subtask]') as HTMLButtonElement;
    expect(btn).not.toBeNull();
    const spy = spyOn(cmp, 'setTab');
    btn.click();
    expect(spy).toHaveBeenCalledWith('create', { mode: 'bulk', parent: 'CORE-1' });
  });

  it('the Buat Card tab resets mode and parent, unlike + Subtask', () => {
    const fixture = renderWithBranch();
    const cmp = fixture.componentInstance as any;
    cmp.createCardEnabled = true;
    fixture.detectChanges();
    (fixture.nativeElement.querySelector('[data-card="CORE-1"] [data-add-subtask]') as HTMLButtonElement).click();
    expect(cmp.createMode()).toBe('bulk');
    expect(cmp.createParent()).toBe('CORE-1');
    (fixture.nativeElement.querySelector('[data-tab="create"]') as HTMLButtonElement).click();
    expect(cmp.createMode()).toBe('single');
    expect(cmp.createParent()).toBeNull();
  });

  it('toasts an error when the clipboard is unavailable', () => {
    const fixture = renderWithBranch();
    spyOnProperty(navigator, 'clipboard').and.returnValue(undefined as unknown as Clipboard);
    const toast = spyOn(TestBed.inject(ToastService), 'error');
    (fixture.nativeElement.querySelector('[data-card="CORE-1"] [data-copy]') as HTMLButtonElement).click();
    expect(toast).toHaveBeenCalledWith('Gagal menyalin', 'CORE-1');
  });

  it('shows Sisa and Mulai for an under-budget card with a start time, and skips an unparsable one', () => {
    const cmp = renderWithBranch().componentInstance as any;
    const base = { issueKey: 'X-1', hasBudgetDetail: true, elapsedSeconds: 1800, budgetSeconds: 3600 };
    const tip = cmp.timeTooltip({ ...base, startedAt: '2026-11-15T03:00:00Z' });
    expect(tip).toContain('Sisa 30m');
    expect(tip).toContain('Mulai');
    expect(cmp.timeTooltip({ ...base, startedAt: 'garbage' })).not.toContain('Mulai');
  });

  it('explains the time bar in a tooltip', () => {
    const fixture = render();
    const time = fixture.nativeElement.querySelector('[data-card="OMS-2"] [data-time]') as HTMLElement;
    expect(time.getAttribute('title')).toContain('Dipakai 1j 15m');
    expect(time.getAttribute('title')).toContain('Budget 1j');
    expect(time.getAttribute('title')).toContain('Lewat 15m');
  });
  it('switches to the column view and remembers it', () => {
    localStorage.removeItem('nudge.board.view');
    const fixture = render();
    (fixture.nativeElement.querySelector('[data-view="columns"]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('app-board-columns')).not.toBeNull();
    expect(localStorage.getItem('nudge.board.view')).toBe('columns');
    localStorage.removeItem('nudge.board.view');
  });
});

describe('board-page helpers', () => {
  it('formats durations compactly', () => {
    expect(shortDuration(5400)).toBe('1j 30m');
    expect(shortDuration(2700)).toBe('45m');
    expect(shortDuration(7200)).toBe('2j');
    expect(shortDuration(-900)).toBe('15m');
  });

  it('maps status names to a tone', () => {
    expect(statusTone('To Do')).toBe('todo');
    expect(statusTone('Code Process')).toBe('process');
    expect(statusTone('CODE REVIEW')).toBe('review');
    expect(statusTone('Code Completed')).toBe('done');
    expect(statusTone('QA')).toBe('other');
  });
});
