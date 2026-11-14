import { Component, computed, input, model, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { IconComponent } from '../../../shared/ui/icon.component';
import { MAX_TIERS, sortTiers, tierCode, tierDuration, tierMinutes, validateCustomTier } from './tier-codes';

interface TierTile {
  code: string;
  custom: boolean;
  checked: boolean;
}

/**
 * Selectable tier tiles (presets + user-added custom tiers) with an
 * "add custom tier" row. `selected` is two-way bound.
 */
@Component({
  selector: 'app-tier-picker',
  standalone: true,
  imports: [FormsModule, IconComponent],
  templateUrl: './tier-picker.component.html',
})
export class TierPickerComponent {
  /** Preset codes always shown as tiles. */
  presets = input.required<string[]>();
  /** "T-" before the budget runs out, "T+" after it. */
  prefix = input<'T-' | 'T+'>('T-');
  /** Tile order: largest first for pre-budget, smallest first for overage. */
  order = input<'desc' | 'asc'>('desc');
  /** Hint under each tile, e.g. "sebelum budget habis". */
  hint = input<string>('');
  selected = model<string[]>([]);

  protected readonly max = MAX_TIERS;
  protected addValue: number | null = null;
  protected addUnit: 'm' | 'h' = 'm';
  protected addError = signal<string | null>(null);

  protected tiles = computed<TierTile[]>(() => {
    const presets = this.presets();
    const selected = this.selected();
    const presetMinutes = new Set(presets.map(tierMinutes));
    const custom = selected.filter((code) => !presetMinutes.has(tierMinutes(code)));
    const selectedMinutes = new Set(selected.map(tierMinutes));
    return sortTiers([...presets, ...custom], this.order()).map((code) => ({
      code,
      custom: !presetMinutes.has(tierMinutes(code)),
      checked: selectedMinutes.has(tierMinutes(code)),
    }));
  });

  protected label(code: string): string {
    return `${this.prefix()}${tierDuration(code)}`;
  }

  protected toggle(tile: TierTile): void {
    const current = this.selected();
    if (tile.checked) {
      this.selected.set(current.filter((code) => tierMinutes(code) !== tierMinutes(tile.code)));
      return;
    }
    if (current.length >= MAX_TIERS) {
      this.addError.set(`Maksimal ${MAX_TIERS} tier`);
      return;
    }
    this.addError.set(null);
    this.selected.set(sortTiers([...current, tile.code], 'desc'));
  }

  protected remove(tile: TierTile, event: Event): void {
    event.preventDefault();
    event.stopPropagation();
    this.selected.set(this.selected().filter((code) => tierMinutes(code) !== tierMinutes(tile.code)));
  }

  protected add(): void {
    const minutes = (this.addValue ?? 0) * (this.addUnit === 'h' ? 60 : 1);
    const error = validateCustomTier(minutes, this.selected());
    this.addError.set(error);
    if (error) return;
    this.selected.set(sortTiers([...this.selected(), tierCode(minutes)], 'desc'));
    this.addValue = null;
  }
}
