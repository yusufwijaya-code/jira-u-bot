import { ChangeDetectionStrategy, Component, computed, effect, input, signal } from '@angular/core';

/**
 * The Jira issue-type icon shown before a ticket key (as in Buat Card's
 * parent picker): the type's own 16px icon, or a neutral glyph when Jira
 * gave no icon or it fails to load. The type name is the tooltip.
 */
@Component({
  selector: 'app-issue-type-icon',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    class: 'issue-type-icon',
    role: 'img',
    '[attr.title]': 'label()',
    '[attr.aria-label]': 'label()',
    '[attr.data-issue-type]': 'name() || null',
  },
  template: `
    @if (showImage()) {
      <img [src]="iconUrl()" alt="" width="16" height="16" loading="lazy" decoding="async" draggable="false" (error)="failed.set(true)" />
    } @else {
      <svg class="issue-type-fallback" viewBox="0 0 16 16" aria-hidden="true">
        <rect x="1" y="1" width="14" height="14" rx="3.5" />
        <path d="M5 8.2 7 10l4-4.2" />
      </svg>
    }
  `,
})
export class IssueTypeIconComponent {
  name = input<string | null | undefined>('');
  iconUrl = input<string | null | undefined>('');

  protected failed = signal(false);
  protected showImage = computed(() => !!this.iconUrl() && !this.failed());
  protected label = computed(() => this.name() || 'Tipe tidak diketahui');

  constructor() {
    // A new url gets a fresh chance to load.
    effect(() => {
      this.iconUrl();
      this.failed.set(false);
    });
  }
}
