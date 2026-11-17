import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { BoardService } from './board.service';

describe('BoardService.getSettings', () => {
  let service: BoardService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
    service = TestBed.inject(BoardService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('shares one request between concurrent callers of the same scope', () => {
    const got: unknown[] = [];
    service.getSettings(26).subscribe((r) => got.push(r));
    service.getSettings(26).subscribe((r) => got.push(r));

    const req = http.expectOne((r) => r.url.endsWith('/settings') && r.params.get('board') === '26');
    req.flush({ scope: 'board' });
    expect(got.length).toBe(2);
  });

  it('keeps scopes apart and fetches again once a request finished', () => {
    service.getSettings(26).subscribe();
    service.getSettings(null).subscribe();
    const reqs = http.match((r) => r.url.endsWith('/settings'));
    expect(reqs.length).toBe(2);
    reqs.forEach((r) => r.flush({}));

    service.getSettings(26).subscribe();
    http.expectOne((r) => r.url.endsWith('/settings') && r.params.get('board') === '26').flush({});
  });

  it('a cancelled request does not block the next one', () => {
    const sub = service.getSettings(26).subscribe();
    const first = http.expectOne((r) => r.url.endsWith('/settings'));
    sub.unsubscribe();
    expect(first.cancelled).toBeTrue();

    service.getSettings(26).subscribe();
    http.expectOne((r) => r.url.endsWith('/settings')).flush({});
  });
});
