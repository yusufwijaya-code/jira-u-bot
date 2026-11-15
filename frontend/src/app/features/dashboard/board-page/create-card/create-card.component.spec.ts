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
