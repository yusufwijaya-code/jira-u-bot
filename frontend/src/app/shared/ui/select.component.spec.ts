import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { FormsModule } from '@angular/forms';
import { SelectComponent, SelectOption } from './select.component';
import { TimePickerComponent } from './time-picker.component';

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

  it('closes on an outside click', () => {
    trigger().click();
    fixture.detectChanges();
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    fixture.detectChanges();
    expect(el().querySelector('.ui-select-list')).toBeNull();
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

  it('keeps an off-grid minute selectable', () => {
    fixture.componentRef.setInput('value', '09:07');
    fixture.detectChanges();
    (el().querySelector('.ui-time-trigger') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(el().querySelector('.ui-time-col:last-child .ui-time-cell.selected')?.textContent?.trim()).toBe('07');
  });
});
