import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { BoardPageComponent } from './board-page.component';

describe('BoardPageComponent', () => {
  let http: HttpTestingController;

  beforeEach(() => {
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
          cards: [{ issueKey: 'OMS-1', summary: 'Lock payments', status: 'To Do', issueURL: 'https://x/browse/OMS-1', boardId: 28, storyPoints: 2 }],
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
});
