import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { MoveDone, MoveFieldsService, MoveRequirement, fieldsRequired } from '../../core/ui/move-fields.service';
import { MoveFieldsDialogComponent } from './move-fields-dialog.component';
import { HttpErrorResponse } from '@angular/common/http';

const req: MoveRequirement = {
  issueKey: 'OMS-1', issueUrl: 'https://jira/browse/OMS-1',
  message: 'Fill in the Merge Request Fields Before Code Review',
  transition: { id: '9', name: 'Merge Request to Snapshot Branch', to: 'Code Review' },
  fields: [{ id: 'customfield_10095', name: 'Merge Request', kind: 'textarea', multi: false, required: true }],
};

describe('MoveFieldsDialogComponent', () => {
  let http: HttpTestingController;

  function render() {
    TestBed.configureTestingModule({ imports: [MoveFieldsDialogComponent], providers: [provideHttpClient(), provideHttpClientTesting()] });
    http = TestBed.inject(HttpTestingController);
    const f = TestBed.createComponent(MoveFieldsDialogComponent);
    f.detectChanges();
    return { f, el: f.nativeElement as HTMLElement, service: TestBed.inject(MoveFieldsService) };
  }

  function open(service: MoveFieldsService, f: ReturnType<typeof render>['f'], r: MoveRequirement = req) {
    let result: MoveDone | null | undefined;
    service.open(r).then((v) => (result = v));
    f.detectChanges();
    return () => result;
  }

  function type(el: HTMLElement, sel: string, v: string) {
    const i = el.querySelector(sel) as HTMLTextAreaElement;
    i.value = v;
    i.dispatchEvent(new Event('input'));
  }

  afterEach(() => http.verify());

  it('shows the target status, Jira message and the required field', () => {
    const { f, el, service } = render();
    expect(el.querySelector('[data-move-fields]')).toBeNull();
    open(service, f);
    expect(el.querySelector('#mf-title')?.textContent).toContain('Lengkapi dulu sebelum pindah ke Code Review');
    expect(el.querySelector('[data-move-message]')?.textContent).toContain('Fill in the Merge Request');
    expect(el.querySelector('textarea[data-field="customfield_10095"]')).not.toBeNull();
    expect((el.querySelector('[data-move-submit]') as HTMLButtonElement).disabled).withContext('empty required field').toBeTrue();
  });

  it('Batal resolves null without calling the server', async () => {
    const { f, el, service } = render();
    const result = open(service, f);
    (el.querySelector('[data-move-cancel]') as HTMLButtonElement).click();
    await Promise.resolve();
    f.detectChanges();
    expect(result()).toBeNull();
    expect(el.querySelector('[data-move-fields]')).toBeNull();
  });

  it('submits the values with the transition and resolves with the result', async () => {
    const { f, el, service } = render();
    const result = open(service, f);
    type(el, 'textarea[data-field="customfield_10095"]', 'https://gitlab/mr/1');
    f.detectChanges();
    (el.querySelector('[data-move-submit]') as HTMLButtonElement).click();
    f.detectChanges();
    expect(el.querySelector('[data-move-submit]')?.textContent).toContain('Menyimpan');
    const post = http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transition'));
    expect(post.request.body).toEqual({ transitionId: '9', fields: { customfield_10095: 'https://gitlab/mr/1' } });
    post.flush({ message: 'OMS-1 dipindah ke Code Review.', status: 'Code Review' });
    await Promise.resolve();
    expect(result()).toEqual({ message: 'OMS-1 dipindah ke Code Review.', status: 'Code Review' });
  });

  it('shows server field errors inline and keeps the dialog open', () => {
    const { f, el, service } = render();
    open(service, f);
    type(el, 'textarea[data-field="customfield_10095"]', 'x');
    f.detectChanges();
    (el.querySelector('[data-move-submit]') as HTMLButtonElement).click();
    http.expectOne((r) => r.url.endsWith('/transition')).flush(
      { error: 'invalid_fields', message: 'Ada isian yang belum valid.', fields: { customfield_10095: 'terlalu pendek' } },
      { status: 422, statusText: 'Unprocessable Entity' },
    );
    f.detectChanges();
    expect(el.querySelector('.cc-err')?.textContent).toContain('terlalu pendek');
    expect(el.querySelector('[data-move-fields]')).not.toBeNull();
  });

  it('re-renders when Jira asks for another field', () => {
    const { f, el, service } = render();
    open(service, f);
    type(el, 'textarea[data-field="customfield_10095"]', 'x');
    f.detectChanges();
    (el.querySelector('[data-move-submit]') as HTMLButtonElement).click();
    http.expectOne((r) => r.url.endsWith('/transition')).flush(
      { ...req, error: 'fields_required', message: 'Release note wajib', fields: [{ id: 'cf_7', name: 'Release note', kind: 'text', multi: false, required: true }] },
      { status: 422, statusText: 'Unprocessable Entity' },
    );
    f.detectChanges();
    expect(el.querySelector('input[data-field="cf_7"]')).not.toBeNull();
    expect(el.querySelector('[data-move-message]')?.textContent).toContain('Release note wajib');
  });

  it('an unsupported field links to Jira and blocks saving', () => {
    const { f, el, service } = render();
    open(service, f, { ...req, fields: [{ id: 'cf_user', name: 'Reviewer', kind: 'unsupported', multi: false, required: true }] });
    expect(el.querySelector('[data-unsupported] [data-fill-in-jira]')?.getAttribute('href')).toBe('https://jira/browse/OMS-1');
    expect((el.querySelector('[data-move-submit]') as HTMLButtonElement).disabled).toBeTrue();
  });
});

describe('fieldsRequired', () => {
  it('reads only a 422 fields_required body', () => {
    const body = { error: 'fields_required', message: 'm', issueKey: 'OMS-1', transition: { id: '9', name: 'n', to: 'Code Review' }, fields: [] };
    expect(fieldsRequired(new HttpErrorResponse({ status: 422, error: body }))?.transition.id).toBe('9');
    expect(fieldsRequired(new HttpErrorResponse({ status: 422, error: { error: 'Transisi tidak tersedia' } }))).toBeNull();
    expect(fieldsRequired(new HttpErrorResponse({ status: 502, error: body }))).toBeNull();
  });
});
