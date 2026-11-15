import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { CreateMeta } from '../../../../core/services/issue-create.service';
import { SingleCardFormComponent } from './single-card-form.component';

const meta: CreateMeta = {
  projectKey: 'OMS', subtaskTypeId: '10143',
  types: [
    { id: '10139', name: 'Story', subtask: false, supported: true, parentAllowed: true, storyPointField: 'customfield_10016', fields: [
      { id: 'customfield_12001', name: 'Merge Request', kind: 'text', multi: false, required: true },
      { id: 'customfield_12002', name: 'Cloud Services', kind: 'select', multi: false, required: true, options: [{ id: '1', label: 'OMS SERVICE' }] },
    ] },
    { id: '9', name: 'Weird', subtask: false, supported: false, parentAllowed: false, fields: [] },
  ],
};

describe('SingleCardFormComponent', () => {
  let http: HttpTestingController;
  function render() {
    TestBed.configureTestingModule({ imports: [SingleCardFormComponent], providers: [provideHttpClient(), provideHttpClientTesting()] });
    http = TestBed.inject(HttpTestingController);
    const f = TestBed.createComponent(SingleCardFormComponent);
    f.componentRef.setInput('boardId', 28);
    f.componentRef.setInput('meta', meta);
    f.detectChanges();
    return f;
  }

  it("renders the type's required fields and submits them", () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    const type = el.querySelector('[data-type]') as HTMLSelectElement;
    type.value = '10139'; type.dispatchEvent(new Event('change')); f.detectChanges();
    expect(el.querySelector('[data-field="customfield_12001"]')).not.toBeNull();
    expect(el.querySelectorAll('[data-field="customfield_12002"] option').length).toBe(2); // placeholder + 1
    const set = (sel: string, v: string) => { const i = el.querySelector(sel) as HTMLInputElement; i.value = v; i.dispatchEvent(new Event(i.tagName === 'SELECT' ? 'change' : 'input')); };
    set('[data-summary]', 'New story'); set('[data-field="customfield_12001"]', 'https://mr'); set('[data-field="customfield_12002"]', '1');
    f.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    const req = http.expectOne((r) => r.url.endsWith('/api/dashboard/create/issue'));
    expect(req.request.body).toEqual(jasmine.objectContaining({ boardId: 28, typeId: '10139', summary: 'New story', fields: { customfield_12001: 'https://mr', customfield_12002: '1' } }));
    req.flush({ key: 'OMS-77', url: 'https://x/browse/OMS-77' });
    f.detectChanges();
    expect(el.querySelector('[data-created]')?.textContent).toContain('OMS-77');
  });

  function fillAll(el: HTMLElement) {
    const type = el.querySelector('[data-type]') as HTMLSelectElement;
    type.value = '10139'; type.dispatchEvent(new Event('change'));
    return type;
  }
  function setIn(el: HTMLElement, sel: string, v: string) {
    const i = el.querySelector(sel) as HTMLInputElement;
    i.value = v; i.dispatchEvent(new Event(i.tagName === 'SELECT' ? 'change' : 'input'));
  }

  it('clears the DOM after "Buat lagi" and blocks a duplicate submit after success', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    fillAll(el); f.detectChanges();
    setIn(el, '[data-summary]', 'S'); setIn(el, '[data-field="customfield_12001"]', 'mr'); setIn(el, '[data-field="customfield_12002"]', '1');
    f.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    http.expectOne((r) => r.url.endsWith('/create/issue')).flush({ key: 'OMS-1', url: 'u' });
    f.detectChanges();
    expect((el.querySelector('[data-submit]') as HTMLButtonElement).disabled).toBeTrue();
    (el.querySelector('[data-created] button') as HTMLButtonElement).click();
    f.detectChanges();
    expect((el.querySelector('[data-summary]') as HTMLInputElement).value).toBe('');
    expect((el.querySelector('[data-field="customfield_12001"]') as HTMLInputElement).value).toBe('');
    expect((el.querySelector('[data-field="customfield_12002"]') as HTMLSelectElement).value).toBe('');
  });

  it('clears dynamic inputs on a type switch and sends no parentKey for a type without parent', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    const m = meta.types[0];
    f.componentRef.setInput('meta', { ...meta, types: [{ ...m, parentAllowed: false }, meta.types[1]] });
    const type = fillAll(el); f.detectChanges();
    setIn(el, '[data-field="customfield_12001"]', 'x');
    type.value = '9'; type.dispatchEvent(new Event('change')); f.detectChanges();
    type.value = '10139'; type.dispatchEvent(new Event('change')); f.detectChanges();
    expect((el.querySelector('[data-field="customfield_12001"]') as HTMLInputElement).value).toBe('');
    setIn(el, '[data-summary]', 'S'); setIn(el, '[data-field="customfield_12001"]', 'mr'); setIn(el, '[data-field="customfield_12002"]', '1');
    f.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    const req = http.expectOne((r) => r.url.endsWith('/create/issue'));
    expect(req.request.body.parentKey).toBeNull();
    req.flush({ key: 'OMS-2', url: 'u' });
  });

  it('renders a Jira 422 field error under Judul', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    fillAll(el); f.detectChanges();
    setIn(el, '[data-summary]', 'S'); setIn(el, '[data-field="customfield_12001"]', 'mr'); setIn(el, '[data-field="customfield_12002"]', '1');
    f.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    http.expectOne((r) => r.url.endsWith('/create/issue')).flush(
      { error: 'jira_rejected', fields: { summary: 'Summary terlalu panjang' }, messages: [] }, { status: 422, statusText: 'Unprocessable' });
    f.detectChanges();
    expect(el.querySelector('.cc-err')?.textContent).toContain('Summary terlalu panjang');
    expect((el.querySelector('[data-submit]') as HTMLButtonElement).disabled).toBeFalse();
  });

  it('shows unslotted Jira field errors as a message and never the raw jira_rejected', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    fillAll(el); f.detectChanges();
    setIn(el, '[data-summary]', 'S'); setIn(el, '[data-field="customfield_12001"]', 'mr'); setIn(el, '[data-field="customfield_12002"]', '1');
    f.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    http.expectOne((r) => r.url.endsWith('/create/issue')).flush(
      { error: 'jira_rejected', fields: { customfield_10016: 'Number value expected' }, messages: [] }, { status: 422, statusText: 'Unprocessable' });
    f.detectChanges();
    expect(el.textContent).toContain('customfield_10016: Number value expected');
    expect(el.textContent).not.toContain('jira_rejected');
  });

  it('shows an empty state when the project has no creatable types', () => {
    TestBed.configureTestingModule({ imports: [SingleCardFormComponent], providers: [provideHttpClient(), provideHttpClientTesting()] });
    const f = TestBed.createComponent(SingleCardFormComponent);
    f.componentRef.setInput('boardId', 28);
    f.componentRef.setInput('meta', { projectKey: 'OMS', subtaskTypeId: '', types: null } as unknown as CreateMeta);
    f.detectChanges();
    expect(f.nativeElement.textContent).toContain('Tidak ada tipe card yang bisa kamu buat di project ini.');
    expect(f.nativeElement.querySelector('[data-type]')).toBeNull();
  });

  it('disables submit for an unsupported type and shows Jira field errors', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    const type = el.querySelector('[data-type]') as HTMLSelectElement;
    type.value = '9'; type.dispatchEvent(new Event('change')); f.detectChanges();
    expect((el.querySelector('[data-submit]') as HTMLButtonElement).disabled).toBeTrue();
    expect(el.textContent).toContain('Tidak didukung');
  });
});
