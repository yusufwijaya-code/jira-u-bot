import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { LeaveComponent, leaveLabel } from './leave.component';

describe('LeaveComponent', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [LeaveComponent],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  function render(leaves = [{ date: '2026-11-18', note: 'Keluarga' }]) {
    const fixture = TestBed.createComponent(LeaveComponent);
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/leaves') && r.method === 'GET').flush({ leaves });
    fixture.detectChanges();
    return fixture;
  }

  it('formats a leave day in Indonesian', () => {
    expect(leaveLabel('2026-11-18')).toBe('Rabu, 18 Nov 2026');
  });

  it('lists the leave days', () => {
    const el: HTMLElement = render().nativeElement;
    expect(el.querySelectorAll('[data-leave]').length).toBe(1);
    expect(el.textContent).toContain('Keluarga');
    expect(el.textContent).toContain('Rabu, 18 Nov 2026');
  });

  /** Day `n` days from today (WIB) as 2006-01-02. */
  const inDays = (n: number) => new Date(Date.now() + 7 * 3600_000 + n * 86400_000).toISOString().slice(0, 10);

  /** Clicks `date` in the open calendar, paging months as needed. */
  function clickDay(fixture: ReturnType<typeof render>, date: string) {
    const el: HTMLElement = fixture.nativeElement;
    for (let i = 0; i < 24 && !el.querySelector(`[data-day="${date}"]`); i++) {
      const first = (el.querySelector('[data-day]') as HTMLElement).dataset['day']!;
      (el.querySelector(date > first ? '[data-next]' : '[data-prev]') as HTMLButtonElement).click();
      fixture.detectChanges();
    }
    (el.querySelector(`[data-day="${date}"]`) as HTMLButtonElement).click();
    fixture.detectChanges();
  }

  function openPicker(fixture: ReturnType<typeof render>) {
    ((fixture.nativeElement as HTMLElement).querySelector('[data-range] .ui-date-trigger') as HTMLButtonElement).click();
    fixture.detectChanges();
  }

  it('submits a range picked with two clicks in one field and reloads', () => {
    const fixture = render([]);
    const el: HTMLElement = fixture.nativeElement;
    const start = inDays(2);
    const end = inDays(4);
    openPicker(fixture);
    clickDay(fixture, start);
    expect(el.querySelector('.ui-date-popup')).not.toBeNull();
    clickDay(fixture, end);
    expect(el.querySelector('.ui-date-popup')).toBeNull();
    expect(el.querySelector('[data-range] .ui-date-count')?.textContent).toContain('3 hari');

    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    const post = http.expectOne((r) => r.method === 'POST');
    expect(post.request.body).toEqual({ from: start, to: end, note: '' });
    post.flush({ added: [start, end], skipped: [], recalculating: false });
    http.expectOne((r) => r.method === 'GET').flush({ leaves: [] });
  });

  it('saves a single day with one click and Selesai', () => {
    const fixture = render([]);
    const el: HTMLElement = fixture.nativeElement;
    const day = inDays(3);
    openPicker(fixture);
    clickDay(fixture, day);
    (el.querySelector('[data-done]') as HTMLButtonElement).click();
    fixture.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    const post = http.expectOne((r) => r.method === 'POST');
    expect(post.request.body).toEqual({ from: day, to: '', note: '' });
    post.flush({ added: [day], skipped: [], recalculating: false });
    http.expectOne((r) => r.method === 'GET').flush({ leaves: [] });
  });

  it('restarts the range when the second click lands before the start', () => {
    const fixture = render([]);
    const el: HTMLElement = fixture.nativeElement;
    openPicker(fixture);
    clickDay(fixture, inDays(5));
    clickDay(fixture, inDays(3));
    expect(el.querySelector('.ui-date-popup')).not.toBeNull();
    clickDay(fixture, inDays(4));
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    const post = http.expectOne((r) => r.method === 'POST');
    expect(post.request.body).toEqual({ from: inDays(3), to: inDays(4), note: '' });
    post.flush({ added: [], skipped: [], recalculating: false });
    http.expectOne((r) => r.method === 'GET').flush({ leaves: [] });
  });

  it('does not offer days older than 60 days', () => {
    const fixture = render([]);
    const el: HTMLElement = fixture.nativeElement;
    openPicker(fixture);
    for (let i = 0; i < 4 && !el.querySelector(`[data-day="${inDays(-61)}"]`); i++) {
      (el.querySelector('[data-prev]') as HTMLButtonElement).click();
      fixture.detectChanges();
    }
    expect((el.querySelector(`[data-day="${inDays(-61)}"]`) as HTMLButtonElement).disabled).toBeTrue();
    expect((el.querySelector(`[data-day="${inDays(-59)}"]`) as HTMLButtonElement | null)?.disabled ?? false).toBeFalse();
  });

  it('deletes a day after confirmation', () => {
    const fixture = render();
    const el: HTMLElement = fixture.nativeElement;
    (el.querySelector('[data-leave="2026-11-18"] [data-delete]') as HTMLButtonElement).click();
    fixture.detectChanges();
    (el.querySelector('[data-leave="2026-11-18"] [data-confirm]') as HTMLButtonElement).click();
    http
      .expectOne((r) => r.method === 'DELETE' && r.url.endsWith('/leaves/2026-11-18'))
      .flush({ deleted: '2026-11-18', recalculating: true });
    http.expectOne((r) => r.method === 'GET').flush({ leaves: [] });
  });
});
