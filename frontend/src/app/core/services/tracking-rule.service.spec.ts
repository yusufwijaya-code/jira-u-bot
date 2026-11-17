import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { PROJECT_KEY_PATTERN, TrackingRuleInput, TrackingRuleService, normalizeProjectKey } from './tracking-rule.service';

describe('TrackingRuleService', () => {
  it('calls the admin tracking-rule endpoints and the board statuses endpoint', () => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
    const svc = TestBed.inject(TrackingRuleService);
    const http = TestBed.inject(HttpTestingController);
    const input: TrackingRuleInput = {
      trackedStatuses: ['In Progress'], doneStatus: 'Done', reviewStatus: null,
      storyPointFields: ['customfield_1'], storyPointWriteField: 'customfield_1', storyPointWriteFieldSubtask: 'customfield_1',
    };

    svc.list().subscribe();
    http.expectOne((r) => r.method === 'GET' && r.url.endsWith('/api/dashboard/admin/tracking-rules')).flush({ rules: [] });
    svc.lookup('POS').subscribe();
    http.expectOne((r) => r.method === 'GET' && r.url.endsWith('/api/dashboard/admin/tracking-rules/jira/POS')).flush({});
    svc.lookup('POS', true).subscribe();
    http.expectOne((r) => r.method === 'GET' && r.url.endsWith('/api/dashboard/admin/tracking-rules/jira/POS?fresh=1')).flush({});
    svc.create({ ...input, projectKey: 'POS' }).subscribe();
    const created = http.expectOne((r) => r.method === 'POST' && r.url.endsWith('/api/dashboard/admin/tracking-rules'));
    expect(created.request.body.projectKey).toBe('POS');
    created.flush({});
    svc.update('POS', input).subscribe();
    http.expectOne((r) => r.method === 'PUT' && r.url.endsWith('/api/dashboard/admin/tracking-rules/POS')).flush({});
    svc.setActive('POS', false).subscribe();
    const active = http.expectOne((r) => r.method === 'PUT' && r.url.endsWith('/api/dashboard/admin/tracking-rules/POS/active'));
    expect(active.request.body).toEqual({ active: false });
    active.flush({});
    svc.recalculate().subscribe();
    http.expectOne((r) => r.method === 'POST' && r.url.endsWith('/api/dashboard/admin/tracking-rules/recalculate')).flush({ started: true });
    svc.boardStatuses(26).subscribe();
    http.expectOne((r) => r.method === 'GET' && r.url.endsWith('/api/dashboard/boards/26/statuses')).flush({ statuses: [] });
    http.verify();
  });

  it('normalizes and validates project keys like the server (AC3.8)', () => {
    expect(normalizeProjectKey(' pos ')).toBe('POS');
    expect(PROJECT_KEY_PATTERN.test('POS')).toBeTrue();
    expect(PROJECT_KEY_PATTERN.test('P')).toBeFalse();
    expect(PROJECT_KEY_PATTERN.test('1POS')).toBeFalse();
    expect(PROJECT_KEY_PATTERN.test('POS-1')).toBeFalse();
  });
});
