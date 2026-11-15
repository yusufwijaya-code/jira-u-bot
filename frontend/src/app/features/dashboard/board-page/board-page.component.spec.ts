import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { BoardPageComponent, COLLAPSED_STORAGE_KEY, shortDuration, statusTone } from './board-page.component';

describe('BoardPageComponent', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    localStorage.removeItem(COLLAPSED_STORAGE_KEY);
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
    const empty = () => el.querySelector('[data-status="Code Process"]')!;
    expect(empty().querySelector('.tb-empty')).toBeNull();

    (empty().querySelector('[data-section-toggle]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(empty().querySelector('.tb-empty')?.textContent).toContain('Tidak ada tiket');

    const todoToggle = el.querySelector('[data-status="To Do"] [data-section-toggle]') as HTMLButtonElement;
    todoToggle.click();
    fixture.detectChanges();
    expect(el.querySelector('[data-status="To Do"] table')).toBeNull();
    expect(JSON.parse(localStorage.getItem(COLLAPSED_STORAGE_KEY)!)).toEqual({ 'Code Process': false, 'To Do': true });
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
