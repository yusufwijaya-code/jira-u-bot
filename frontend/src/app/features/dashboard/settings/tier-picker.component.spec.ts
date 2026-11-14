import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TierPickerComponent } from './tier-picker.component';
import { tierCode, tierDuration, tierMinutes, validateCustomTier } from './tier-codes';

describe('tier codes', () => {
  it('parses and renders codes like the backend', () => {
    expect(tierMinutes('45m')).toBe(45);
    expect(tierMinutes('2h')).toBe(120);
    expect(tierMinutes('x')).toBe(0);
    expect(tierCode(60)).toBe('1h');
    expect(tierCode(90)).toBe('90m');
    expect(tierDuration('90m')).toBe('1,5 jam');
    expect(tierDuration('45m')).toBe('45 menit');
  });

  it('validates custom tiers', () => {
    expect(validateCustomTier(0, [])).toContain('1 menit sampai 8 jam');
    expect(validateCustomTier(481, [])).toContain('1 menit sampai 8 jam');
    expect(validateCustomTier(60, ['1h'])).toBe('Tier ini sudah ada');
    expect(validateCustomTier(7, ['1m', '2m', '3m', '4m', '5m', '6m', '8m', '9m', '10m', '11m'])).toContain('Maksimal');
    expect(validateCustomTier(45, ['2h'])).toBeNull();
  });
});

describe('TierPickerComponent', () => {
  let fixture: ComponentFixture<TierPickerComponent>;
  const el = () => fixture.nativeElement as HTMLElement;
  const selected = () => fixture.componentInstance.selected();
  const labels = () => Array.from(el().querySelectorAll('.tier-tile strong')).map((n) => n.textContent!.trim());

  beforeEach(async () => {
    await TestBed.configureTestingModule({ imports: [TierPickerComponent] }).compileComponents();
    fixture = TestBed.createComponent(TierPickerComponent);
    fixture.componentRef.setInput('presets', ['5m', '15m', '1h']);
    fixture.componentRef.setInput('prefix', 'T+');
    fixture.componentRef.setInput('order', 'asc');
    fixture.componentRef.setInput('selected', ['15m', '45m']);
    fixture.detectChanges();
  });

  it('renders presets plus selected custom tiers in the requested order', () => {
    expect(labels()).toEqual(['T+5 menit', 'T+15 menit', 'T+45 menit', 'T+1 jam']);
    expect(el().querySelectorAll('.tier-tile.custom').length).toBe(1);
  });

  it('toggles a preset on and off', () => {
    const tiles = el().querySelectorAll('.tier-tile input');
    (tiles[0] as HTMLInputElement).click(); // 5m on
    expect(selected()).toEqual(['45m', '15m', '5m']);
    (tiles[1] as HTMLInputElement).click(); // 15m off
    expect(selected()).toEqual(['45m', '5m']);
  });

  it('adds a custom tier from the input row, converting hours', () => {
    const component = fixture.componentInstance as any;
    component.addValue = 3;
    component.addUnit = 'h';
    component.add();
    fixture.detectChanges();

    expect(selected()).toEqual(['3h', '45m', '15m']);
    expect(labels()).toContain('T+3 jam');
  });

  it('shows a validation error and does not add a duplicate', () => {
    const component = fixture.componentInstance as any;
    component.addValue = 15;
    component.addUnit = 'm';
    component.add();
    fixture.detectChanges();

    expect(selected()).toEqual(['15m', '45m']);
    expect(el().querySelector('.field-error')?.textContent).toContain('sudah ada');
  });

  it('removes a custom tier with its remove button', () => {
    (el().querySelector('.tile-remove') as HTMLButtonElement).click();
    fixture.detectChanges();

    expect(selected()).toEqual(['15m']);
    expect(labels()).not.toContain('T+45 menit');
  });
});
