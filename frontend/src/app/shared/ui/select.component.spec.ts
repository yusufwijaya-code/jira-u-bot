import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { FormsModule } from '@angular/forms';
import { SelectComponent, SelectOption } from './select.component';
import { TimePickerComponent } from './time-picker.component';
import { shouldDropUp } from './popover-position';

const at = (top: number, height = 40) => ({ getBoundingClientRect: () => new DOMRect(0, top, 200, height) }) as unknown as Element;

describe('shouldDropUp', () => {
  it('opens downward when the popover fits below', () => {
    expect(shouldDropUp(at(100), 260, 800)).toBeFalse();
  });
  it('opens upward when it would overflow the viewport and there is more room above', () => {
    expect(shouldDropUp(at(700), 260, 800)).toBeTrue();
  });
  it('stays downward when there is even less room above', () => {
    expect(shouldDropUp(at(60), 260, 300)).toBeFalse();
  });
});

@Component({
  standalone: true,
  imports: [FormsModule, SelectComponent],
  template: `<app-select [(ngModel)]="value" [options]="options" ariaLabel="Satuan" />`,
})
class SelectHostComponent {
  value: number | null = 5;
  options: SelectOption<number | null>[] = [
    { value: 5, label: 'Ulang tiap 5 menit' },
    { value: 30, label: 'Ulang tiap 30 menit' },
    { value: null, label: 'Berhenti' },
  ];
}

describe('SelectComponent', () => {
  let fixture: ComponentFixture<SelectHostComponent>;
  const el = () => fixture.nativeElement as HTMLElement;
  const trigger = () => el().querySelector('.ui-select-trigger') as HTMLButtonElement;
  const options = () => Array.from(el().querySelectorAll('.ui-select-option')) as HTMLElement[];

  beforeEach(async () => {
    await TestBed.configureTestingModule({ imports: [SelectHostComponent] }).compileComponents();
    fixture = TestBed.createComponent(SelectHostComponent);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
  });

  it('shows the selected label and no native <select>', () => {
    expect(trigger().textContent).toContain('Ulang tiap 5 menit');
    expect(el().querySelector('select')).toBeNull();
  });

  it('opens a styled list and picks an option with the mouse (including null)', () => {
    trigger().click();
    fixture.detectChanges();
    expect(options().length).toBe(3);
    expect(options()[0].classList).toContain('selected');

    options()[2].click();
    fixture.detectChanges();
    expect(fixture.componentInstance.value).toBeNull();
    expect(trigger().textContent).toContain('Berhenti');
    expect(el().querySelector('.ui-select-list')).toBeNull();
  });

  it('supports the keyboard: arrows to move, Enter to pick, Escape to close', () => {
    trigger().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }));
    fixture.detectChanges();
    expect(el().querySelector('.ui-select-list')).not.toBeNull();

    trigger().dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }));
    trigger().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
    fixture.detectChanges();
    expect(fixture.componentInstance.value).toBe(30);

    trigger().click();
    fixture.detectChanges();
    trigger().dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    fixture.detectChanges();
    expect(el().querySelector('.ui-select-list')).toBeNull();
  });

  it('flips the list above the trigger near the bottom of the viewport', () => {
    const host = el().querySelector('app-select') as HTMLElement;
    trigger().click();
    fixture.detectChanges();
    expect(host.classList).not.toContain('drop-up');
    trigger().click();
    fixture.detectChanges();

    spyOn(host, 'getBoundingClientRect').and.returnValue(new DOMRect(0, window.innerHeight - 44, 200, 40));
    trigger().click();
    fixture.detectChanges();
    expect(host.classList).toContain('drop-up');
    expect(el().querySelector('.ui-select-list')).not.toBeNull();
  });

  it('closes on an outside click', () => {
    trigger().click();
    fixture.detectChanges();
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    fixture.detectChanges();
    expect(el().querySelector('.ui-select-list')).toBeNull();
  });

  it('renders no icon when options have none', () => {
    trigger().click();
    fixture.detectChanges();
    expect(el().querySelector('.ui-select-icon')).toBeNull();
  });

  it('shows an option icon in the list and on the trigger for the selected one', () => {
    fixture.componentInstance.options = [
      { value: 5, label: 'Bug', icon: 'https://jira.example/bug.svg' },
      { value: 30, label: 'Story' },
    ];
    fixture.detectChanges();
    const triggerIcon = trigger().querySelector('img.ui-select-icon') as HTMLImageElement;
    expect(triggerIcon.getAttribute('src')).toBe('https://jira.example/bug.svg');
    trigger().click();
    fixture.detectChanges();
    expect(options()[0].querySelector('img.ui-select-icon')).not.toBeNull();
    expect(options()[1].querySelector('img.ui-select-icon')).toBeNull();
    options()[1].click();
    fixture.detectChanges();
    expect(trigger().querySelector('img.ui-select-icon')).toBeNull();
  });
});

describe('TimePickerComponent', () => {
  let fixture: ComponentFixture<TimePickerComponent>;
  let emitted: string[];
  const el = () => fixture.nativeElement as HTMLElement;

  beforeEach(async () => {
    await TestBed.configureTestingModule({ imports: [TimePickerComponent] }).compileComponents();
    fixture = TestBed.createComponent(TimePickerComponent);
    fixture.componentRef.setInput('value', '12:30');
    emitted = [];
    fixture.componentInstance.valueChange.subscribe((v) => emitted.push(v));
    fixture.detectChanges();
  });

  it('shows the value and opens hour/minute columns instead of a native time input', () => {
    expect(el().querySelector('input[type=time]')).toBeNull();
    expect(el().querySelector('.ui-time-trigger')?.textContent).toContain('12:30');

    (el().querySelector('.ui-time-trigger') as HTMLButtonElement).click();
    fixture.detectChanges();
    const cols = el().querySelectorAll('.ui-time-col');
    expect(cols.length).toBe(2);
    expect(cols[0].querySelectorAll('.ui-time-cell').length).toBe(24);
    expect(cols[1].querySelectorAll('.ui-time-cell').length).toBe(12);
    expect(el().querySelectorAll('.ui-time-cell.selected').length).toBe(2);
  });

  it('emits HH:MM when an hour or a minute is picked', () => {
    (el().querySelector('.ui-time-trigger') as HTMLButtonElement).click();
    fixture.detectChanges();
    const [hours, minutes] = Array.from(el().querySelectorAll('.ui-time-col'));

    (hours.querySelectorAll('.ui-time-cell')[22] as HTMLButtonElement).click();
    (minutes.querySelectorAll('.ui-time-cell')[3] as HTMLButtonElement).click();

    expect(emitted).toEqual(['22:30', '12:15']);
  });

  it('opens the popup upward when there is no room below', () => {
    const host = fixture.nativeElement as HTMLElement;
    spyOn(host, 'getBoundingClientRect').and.returnValue(new DOMRect(0, window.innerHeight - 44, 120, 40));
    (el().querySelector('.ui-time-trigger') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(host.classList).toContain('drop-up');
    (el().querySelector('.ui-time-trigger') as HTMLButtonElement).dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    fixture.detectChanges();
    expect(el().querySelector('.ui-time-popup')).toBeNull();
    expect(host.classList).not.toContain('drop-up');
  });

  it('keeps an off-grid minute selectable', () => {
    fixture.componentRef.setInput('value', '09:07');
    fixture.detectChanges();
    (el().querySelector('.ui-time-trigger') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(el().querySelector('.ui-time-col:last-child .ui-time-cell.selected')?.textContent?.trim()).toBe('07');
  });
});
