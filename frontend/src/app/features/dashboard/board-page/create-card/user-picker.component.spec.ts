import { TestBed, fakeAsync, tick } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { UserPickerComponent } from './user-picker.component';

describe('UserPickerComponent', () => {
  it('searches after 2 chars, emits the picked accountId, and emits null when cleared', fakeAsync(() => {
    TestBed.configureTestingModule({ imports: [UserPickerComponent], providers: [provideHttpClient(), provideHttpClientTesting()] });
    const http = TestBed.inject(HttpTestingController);
    const f = TestBed.createComponent(UserPickerComponent);
    f.componentRef.setInput('boardId', 26);
    const emitted: (string | null)[] = [];
    f.componentInstance.picked.subscribe((v) => emitted.push(v));
    f.detectChanges();
    const el: HTMLElement = f.nativeElement;
    const input = el.querySelector('[data-user-input]') as HTMLInputElement;
    input.dispatchEvent(new Event('focus')); f.detectChanges();
    expect(el.textContent).toContain('Saya');
    input.value = 'bu'; input.dispatchEvent(new Event('input'));
    tick(250);
    http.expectOne((r) => r.url.endsWith('/create/users') && r.params.get('q') === 'bu').flush({ users: [{ accountId: 'acc-1', displayName: 'Budi', avatarUrl: '' }] });
    f.detectChanges();
    (el.querySelector('[data-user="acc-1"]') as HTMLButtonElement).click();
    expect(emitted).toEqual(['acc-1']);
    input.value = ''; input.dispatchEvent(new Event('input'));
    expect(emitted).toEqual(['acc-1', null]);
    tick(250);
    f.detectChanges();
    input.dispatchEvent(new Event('blur')); tick(150); f.detectChanges();
    expect(el.querySelector('.up-list')).toBeNull();
  }));

  it('closes on Escape and on an outside click without changing the value', () => {
    TestBed.configureTestingModule({ imports: [UserPickerComponent], providers: [provideHttpClient(), provideHttpClientTesting()] });
    const f = TestBed.createComponent(UserPickerComponent);
    f.componentRef.setInput('boardId', 26);
    const emitted: (string | null)[] = [];
    f.componentInstance.picked.subscribe((v) => emitted.push(v));
    f.detectChanges();
    const el: HTMLElement = f.nativeElement;
    const input = el.querySelector('[data-user-input]') as HTMLInputElement;
    input.dispatchEvent(new Event('focus')); f.detectChanges();
    expect(el.querySelector('.up-list')).not.toBeNull();
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); f.detectChanges();
    expect(el.querySelector('.up-list')).toBeNull();
    input.dispatchEvent(new Event('click')); f.detectChanges();
    expect(el.querySelector('.up-list')).not.toBeNull();
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true })); f.detectChanges();
    expect(el.querySelector('.up-list')).toBeNull();
    expect(emitted).toEqual([]);
  });
});
