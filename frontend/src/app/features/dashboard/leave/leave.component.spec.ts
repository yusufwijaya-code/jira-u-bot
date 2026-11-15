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

  it('submits a range and reloads', () => {
    const fixture = render([]);
    const el: HTMLElement = fixture.nativeElement;
    const from = el.querySelector('[data-from]') as HTMLInputElement;
    const to = el.querySelector('[data-to]') as HTMLInputElement;
    from.value = '2026-11-23';
    from.dispatchEvent(new Event('input'));
    to.value = '2026-11-24';
    to.dispatchEvent(new Event('input'));
    fixture.detectChanges();

    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    const post = http.expectOne((r) => r.method === 'POST');
    expect(post.request.body).toEqual({ from: '2026-11-23', to: '2026-11-24', note: '' });
    post.flush({ added: ['2026-11-23', '2026-11-24'], skipped: [], recalculating: false });
    http.expectOne((r) => r.method === 'GET').flush({ leaves: [] });
  });

  it('blocks an end date before the start date', () => {
    const fixture = render([]);
    const el: HTMLElement = fixture.nativeElement;
    const from = el.querySelector('[data-from]') as HTMLInputElement;
    const to = el.querySelector('[data-to]') as HTMLInputElement;
    from.value = '2026-11-24';
    from.dispatchEvent(new Event('input'));
    to.value = '2026-11-23';
    to.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect((el.querySelector('[data-submit]') as HTMLButtonElement).disabled).toBeTrue();
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
