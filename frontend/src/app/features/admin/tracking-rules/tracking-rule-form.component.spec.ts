import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import { Subject, of, throwError } from 'rxjs';
import { TrackingRuleFormComponent } from './tracking-rule-form.component';
import { JiraProjectLookup, TrackingRule, TrackingRuleService } from '../../../core/services/tracking-rule.service';
import { ToastService } from '../../../core/ui/toast.service';

const POS: JiraProjectLookup = {
  projectKey: 'POS', projectName: 'ESB POS', avatarUrl: '',
  statuses: ['To Do', 'In Progress', 'Code Review', 'Done'],
  numberFields: [
    { id: 'customfield_10016', name: 'Story point estimate', storyPointLike: true },
    { id: 'customfield_200', name: 'Effort', storyPointLike: false },
  ],
};

const CORE_RULE: TrackingRule = {
  projectKey: 'CORE', projectName: 'ESB Core', trackedStatuses: ['Code Process', 'CODE REVIEW'], doneStatus: 'Code Completed', reviewStatus: 'CODE REVIEW',
  storyPointFields: [{ id: 'customfield_10125', name: 'Story Point Estimate' }, { id: 'customfield_11053', name: 'Story Points' }],
  storyPointWriteField: 'customfield_10125', storyPointWriteFieldSubtask: 'customfield_11053', isActive: true, editedBy: 'seed', editedDate: '2026-11-16T09:00:00+07:00',
};

describe('TrackingRuleFormComponent', () => {
  let api: jasmine.SpyObj<TrackingRuleService>;
  let toast: jasmine.SpyObj<ToastService>;

  function setup(key: string | null): void {
    api = jasmine.createSpyObj('TrackingRuleService', ['list', 'lookup', 'create', 'update', 'setActive', 'recalculate']);
    toast = jasmine.createSpyObj('ToastService', ['show', 'success', 'error', 'info']);
    TestBed.configureTestingModule({
      imports: [TrackingRuleFormComponent],
      providers: [
        provideRouter([]),
        { provide: ActivatedRoute, useValue: { snapshot: { paramMap: convertToParamMap(key ? { key } : {}) } } },
        { provide: TrackingRuleService, useValue: api },
        { provide: ToastService, useValue: toast },
      ],
    });
  }

  function create() {
    const fixture = TestBed.createComponent(TrackingRuleFormComponent);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, c: fixture.componentInstance as any };
  }

  it('looks the project up only for a valid key and normalizes it (AC3.1, AC3.8)', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    const { fixture, el, c } = create();
    const button = () => el.querySelector('[data-lookup]') as HTMLButtonElement;
    c.onKeyInput('x');
    fixture.detectChanges();
    expect(button().disabled).toBeTrue();
    c.onKeyInput(' pos ');
    fixture.detectChanges();
    button().click();
    fixture.detectChanges();
    expect(api.lookup).toHaveBeenCalledWith('POS', false);
    expect(el.querySelector('[data-project]')?.textContent).toContain('ESB POS');
    expect(el.querySelectorAll('[data-status]').length).toBe(4);
  });

  it('shows a loading skeleton while the Jira lookup runs', () => {
    setup(null);
    const pending = new Subject<JiraProjectLookup>();
    api.lookup.and.returnValue(pending);
    const { fixture, el, c } = create();
    c.onKeyInput('POS');
    c.runLookup();
    fixture.detectChanges();
    expect(el.querySelector('[data-lookup-loading]')).not.toBeNull();
    expect((el.querySelector('[data-lookup]') as HTMLButtonElement).classList).toContain('loading');
    pending.next(POS);
    pending.complete();
    fixture.detectChanges();
    expect(el.querySelector('[data-lookup-loading]')).toBeNull();
    expect(el.querySelector('[data-project]')).not.toBeNull();
  });

  it('shows the not-found message from Jira (AC3.5)', () => {
    setup(null);
    api.lookup.and.returnValue(throwError(() => ({ status: 404 })));
    const { fixture, el, c } = create();
    c.onKeyInput('NOPE');
    c.runLookup();
    fixture.detectChanges();
    expect(el.querySelector('[data-lookup-error]')?.textContent).toContain('Project NOPE tidak ditemukan di Jira');
  });

  it('keeps done out of the tracked statuses and review inside them (AC3.2)', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    const { c } = create();
    c.onKeyInput('POS');
    c.runLookup();
    c.toggleTracked('In Progress');
    c.toggleTracked('Code Review');
    c.setDone('Done');
    c.setReview('Code Review');
    expect(c.doneOptions().map((o: { value: string }) => o.value)).toEqual(['', 'To Do', 'Done']);
    c.toggleTracked('Code Review');
    expect(c.review()).toBe('');
    c.toggleTracked('Done');
    expect(c.done()).toBe('');
  });

  it('orders Story Point fields and resets a write field whose field is removed', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    const { c } = create();
    c.onKeyInput('POS');
    c.runLookup();
    c.addField('customfield_200');
    c.addField('customfield_10016');
    expect(c.writeField()).toBe('customfield_200'); // default = the first field
    c.moveField(1, -1);
    expect(c.spFields()).toEqual(['customfield_10016', 'customfield_200']);
    c.removeField('customfield_200');
    expect(c.writeField()).toBe('');
    expect(c.writeFieldSubtask()).toBe('');
  });

  it('previews the rule as a sentence and saves it with the recalculation toast (AC3.3, AC3.4)', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    api.create.and.returnValue(of({ ...CORE_RULE, projectKey: 'POS' }));
    const { fixture, el, c } = create();
    const router = TestBed.inject(Router);
    spyOn(router, 'navigate').and.resolveTo(true);
    c.onKeyInput('POS');
    c.runLookup();
    c.toggleTracked('In Progress');
    c.setDone('Done');
    c.addField('customfield_10016');
    fixture.detectChanges();

    expect(el.querySelector('[data-preview]')?.textContent?.trim()).toBe(
      'Budget berjalan selama In Progress; selesai saat Done. Review tidak dihitung. ' +
        'SP dibaca dari Story point estimate (customfield_10016); ' +
        'ditulis ke Story point estimate (customfield_10016) (card biasa) / Story point estimate (customfield_10016) (subtask).',
    );
    (el.querySelector('[data-save]') as HTMLButtonElement).click();
    expect(api.create).toHaveBeenCalledWith({
      projectKey: 'POS', trackedStatuses: ['In Progress'], doneStatus: 'Done', reviewStatus: null,
      storyPointFields: ['customfield_10016'], storyPointWriteField: 'customfield_10016', storyPointWriteFieldSubtask: 'customfield_10016',
    });
    expect(toast.show.calls.mostRecent().args[4]!.label).toBe('Hitung ulang semua user');
    expect(router.navigate).toHaveBeenCalledWith(['/admin/tracking-rules']);
  });

  it('links to Edit on 409 and shows server field errors (AC3.6, AC3.7)', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    const { fixture, el, c } = create();
    c.onKeyInput('POS');
    c.runLookup();
    c.toggleTracked('In Progress');
    c.setDone('Done');
    c.addField('customfield_10016');

    api.create.and.returnValue(throwError(() => ({ status: 409, error: { error: 'rule_exists' } })));
    c.save();
    fixture.detectChanges();
    expect(el.querySelector('[data-exists] a')?.getAttribute('href')).toBe('/admin/tracking-rules/POS');

    api.create.and.returnValue(throwError(() => ({ status: 400, error: { error: 'validation_failed', fields: { doneStatus: 'Status selesai tidak boleh termasuk status yang dihitung' } } })));
    c.save();
    fixture.detectChanges();
    expect(el.textContent).toContain('Status selesai tidak boleh termasuk status yang dihitung');
  });

  it('loads a stored rule on Edit and blocks saving while a stored status is gone from Jira (AC4.1, AC4.4)', () => {
    setup('CORE');
    api.list.and.returnValue(of({ rules: [CORE_RULE] }));
    api.lookup.and.returnValue(of({
      projectKey: 'CORE', projectName: 'ESB Core', avatarUrl: '',
      statuses: ['To Do', 'Code Process', 'CODE REVIEW', 'Done'],
      numberFields: [
        { id: 'customfield_10125', name: 'Story Point Estimate', storyPointLike: true },
        { id: 'customfield_11053', name: 'Story Points', storyPointLike: true },
      ],
    }));
    api.update.and.returnValue(of(CORE_RULE));
    const { fixture, el, c } = create();

    expect((el.querySelector('[data-key-input]') as HTMLInputElement).readOnly).toBeTrue();
    expect(api.lookup).toHaveBeenCalledWith('CORE', true);
    expect(el.querySelector('[data-status="Code Process"]')?.classList).toContain('on');
    expect(el.querySelector('[data-stale-statuses]')?.textContent).toContain('Code Completed');
    expect((el.querySelector('[data-save]') as HTMLButtonElement).disabled).toBeTrue();

    c.dropStaleStatus('Code Completed');
    c.setDone('Done');
    fixture.detectChanges();
    expect(el.querySelector('[data-stale-statuses]')).toBeNull();
    (el.querySelector('[data-save]') as HTMLButtonElement).click();
    expect(api.update).toHaveBeenCalledWith('CORE', jasmine.objectContaining({ doneStatus: 'Done', reviewStatus: 'CODE REVIEW' }));

    // "Muat ulang dari Jira" bypasses the server's lookup cache too.
    (el.querySelector('[data-lookup]') as HTMLButtonElement).click();
    expect(api.lookup.calls.count()).toBe(2);
    expect(api.lookup.calls.mostRecent().args).toEqual(['CORE', true]);
  });
});
