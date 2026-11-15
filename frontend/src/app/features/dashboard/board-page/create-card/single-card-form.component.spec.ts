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

  it('disables submit for an unsupported type and shows Jira field errors', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    const type = el.querySelector('[data-type]') as HTMLSelectElement;
    type.value = '9'; type.dispatchEvent(new Event('change')); f.detectChanges();
    expect((el.querySelector('[data-submit]') as HTMLButtonElement).disabled).toBeTrue();
    expect(el.textContent).toContain('Tidak didukung');
  });
});
