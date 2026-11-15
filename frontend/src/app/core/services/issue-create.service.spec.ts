import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { IssueCreateService } from './issue-create.service';

describe('IssueCreateService', () => {
  it('hits the create endpoints with boardId', () => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
    const svc = TestBed.inject(IssueCreateService);
    const http = TestBed.inject(HttpTestingController);
    svc.meta(26).subscribe();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/create/meta') && r.params.get('boardId') === '26').flush({ projectKey: 'CORE', types: [], subtaskTypeId: '' });
    svc.parent(26, 'CORE-1').subscribe();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/create/parent/CORE-1') && r.params.get('boardId') === '26').flush({});
    svc.normalize(26, 'x').subscribe();
    const req = http.expectOne((r) => r.url.endsWith('/api/dashboard/create/normalize'));
    expect(req.request.body).toEqual({ boardId: 26, text: 'x' });
    req.flush({ rows: [], warnings: [], cached: false });
    http.verify();
  });
});
