import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { BoardContextService, BoardSummary } from '../../../../core/services/board-context.service';
import { CreateCardComponent } from './create-card.component';

describe('CreateCardComponent', () => {
  let http: HttpTestingController;
  function render(boardId: number | null) {
    TestBed.configureTestingModule({ imports: [CreateCardComponent], providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])] });
    http = TestBed.inject(HttpTestingController);
    const f = TestBed.createComponent(CreateCardComponent);
    f.componentRef.setInput('boardId', boardId);
    return f;
  }

  it('asks for the Jira token when the user has none', () => {
    const f = render(26);
    f.detectChanges();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/create/status')).flush({ tokenReady: false, aiReady: true });
    http.match((r) => r.url.endsWith('/api/dashboard/create/meta')).forEach((r) => r.flush({ projectKey: 'CORE', types: [], subtaskTypeId: '' }));
    f.detectChanges();
    const el: HTMLElement = f.nativeElement;
    const banner = el.querySelector('[data-token-banner]');
    expect(banner).not.toBeNull();
    expect(banner!.querySelector('a')!.getAttribute('href')).toBe('/dashboard/settings');
    expect(el.querySelector('[data-mode]')).toBeNull();
  });

  it('shows the token banner, not an endless spinner, when meta answers 412', () => {
    const f = render(26);
    f.detectChanges();
    http.expectOne((r) => r.url.endsWith('/create/status')).flush({ tokenReady: true, aiReady: true });
    http.expectOne((r) => r.url.endsWith('/create/meta')).flush({ error: 'jira_token_required' }, { status: 412, statusText: 'x' });
    f.detectChanges();
    expect(f.nativeElement.querySelector('[data-token-banner]')).not.toBeNull();
    expect(f.nativeElement.querySelector('.tb-spin')).toBeNull();
  });

  it('shows an empty state when meta has no types', () => {
    const f = render(26);
    f.detectChanges();
    http.expectOne((r) => r.url.endsWith('/create/status')).flush({ tokenReady: true, aiReady: true });
    http.expectOne((r) => r.url.endsWith('/create/meta')).flush({ projectKey: 'CORE', types: [], subtaskTypeId: '' });
    f.detectChanges();
    expect(f.nativeElement.textContent).toContain('Tidak ada tipe card yang bisa kamu buat di project ini.');
  });

  it('shows a neutral error, not the token banner, when status fails', () => {
    const f = render(26);
    f.detectChanges();
    http.expectOne((r) => r.url.endsWith('/create/status')).flush({}, { status: 500, statusText: 'x' });
    http.match((r) => r.url.endsWith('/create/meta'));
    f.detectChanges();
    const el: HTMLElement = f.nativeElement;
    expect(el.querySelector('[data-status-error]')?.textContent).toContain('Gagal memuat status');
    expect(el.querySelector('[data-token-banner]')).toBeNull();
    expect(el.querySelector('[data-mode]')).toBeNull();
  });

  it('cancels the in-flight meta request on a quick board switch', () => {
    const f = render(26);
    f.detectChanges();
    http.expectOne((r) => r.url.endsWith('/create/status')).flush({ tokenReady: true, aiReady: true });
    const a = http.expectOne((r) => r.url.endsWith('/create/meta') && r.params.get('boardId') === '26');
    f.componentRef.setInput('boardId', 27);
    f.detectChanges();
    expect(a.cancelled).toBeTrue();
    http.expectOne((r) => r.url.endsWith('/create/meta') && r.params.get('boardId') === '27').flush({ projectKey: 'OMS', types: [], subtaskTypeId: '' });
  });

  it('asks to pick a board when the scope is all boards', () => {
    const f = render(null);
    TestBed.inject(BoardContextService).boards.set([{ boardId: 26, name: 'ESB Core Board', projectKey: 'CORE' } as BoardSummary]);
    f.detectChanges();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/create/status')).flush({ tokenReady: true, aiReady: true });
    f.detectChanges();
    const el: HTMLElement = f.nativeElement;
    const pick = el.querySelector('[data-board-pick]') as HTMLElement;
    expect(pick).not.toBeNull();
    expect(pick.querySelectorAll('option').length).toBe(2); // placeholder + 1 board
    const sel = pick.querySelector('select') as HTMLSelectElement;
    sel.value = '26'; sel.dispatchEvent(new Event('change')); f.detectChanges();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/create/meta') && r.params.get('boardId') === '26')
      .flush({ projectKey: 'CORE', types: [], subtaskTypeId: '' });
    http.verify();
  });
});
