import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { BoardColumnsComponent } from './board-columns.component';

describe('BoardColumnsComponent', () => {
  let http: HttpTestingController;
  const card = { issueKey: 'OMS-1', summary: 'A', status: 'To Do', issueURL: 'u', boardId: 1, storyPoints: 2, hasBudgetDetail: false, elapsedSeconds: 0, budgetSeconds: 0, elapsedLabel: '', remainingLabel: '', deadlineLabel: '' };

  function render() {
    TestBed.configureTestingModule({ imports: [BoardColumnsComponent], providers: [provideHttpClient(), provideHttpClientTesting()] });
    http = TestBed.inject(HttpTestingController);
    const f = TestBed.createComponent(BoardColumnsComponent);
    f.componentRef.setInput('columns', [{ status: 'To Do', cards: [card] }, { status: 'Code Process', cards: [] }, { status: 'Done', cards: [] }]);
    f.detectChanges();
    return f;
  }

  function dragEvent(type: string): DragEvent {
    return new DragEvent(type, { bubbles: true, cancelable: true, dataTransfer: new DataTransfer() });
  }

  it('lights up only the columns the workflow allows and emits on drop', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    const emitted: unknown[] = [];
    f.componentInstance.moveRequested.subscribe((v) => emitted.push(v));
    el.querySelector('[data-kcard="OMS-1"]')!.dispatchEvent(dragEvent('dragstart'));
    http.expectOne((r) => r.url.endsWith('/tickets/OMS-1/transitions')).flush({ transitions: [{ id: '21', name: 'Start', toStatus: 'Code Process' }] });
    f.detectChanges();
    expect(el.querySelector('[data-kcol="Code Process"]')!.classList).toContain('drop-ok');
    expect(el.querySelector('[data-kcol="Done"]')!.classList).toContain('drop-no');
    el.querySelector('[data-kcol="Done"]')!.dispatchEvent(dragEvent('drop'));
    expect(emitted.length).toBe(0);
    el.querySelector('[data-kcol="Code Process"]')!.dispatchEvent(dragEvent('drop'));
    expect(emitted).toEqual([jasmine.objectContaining({ option: jasmine.objectContaining({ id: '21' }) })]);
  });
});
