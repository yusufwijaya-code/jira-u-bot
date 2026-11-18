import { TestBed } from '@angular/core/testing';
import { IssueTypeIconComponent } from './issue-type-icon.component';

describe('IssueTypeIconComponent', () => {
  function render(name: string, iconUrl: string) {
    TestBed.configureTestingModule({ imports: [IssueTypeIconComponent] });
    const fixture = TestBed.createComponent(IssueTypeIconComponent);
    fixture.componentRef.setInput('name', name);
    fixture.componentRef.setInput('iconUrl', iconUrl);
    fixture.detectChanges();
    return fixture;
  }

  it('shows the type icon, lazy and 16px, with the type name as tooltip', () => {
    const fixture = render('Bug', 'https://jira.example/bug.svg');
    const host = fixture.nativeElement as HTMLElement;
    const img = host.querySelector('img') as HTMLImageElement;
    expect(img.getAttribute('src')).toBe('https://jira.example/bug.svg');
    expect(img.getAttribute('loading')).toBe('lazy');
    expect(img.getAttribute('width')).toBe('16');
    expect(host.getAttribute('title')).toBe('Bug');
    expect(host.getAttribute('aria-label')).toBe('Bug');
  });

  it('falls back to a generic glyph without a url', () => {
    const host = render('Story', '').nativeElement as HTMLElement;
    expect(host.querySelector('img')).toBeNull();
    expect(host.querySelector('svg.issue-type-fallback')).not.toBeNull();
    expect(host.getAttribute('title')).toBe('Story');
  });

  it('falls back when the icon fails to load, and retries a new url', () => {
    const fixture = render('Task', 'https://jira.example/broken.svg');
    const host = fixture.nativeElement as HTMLElement;
    host.querySelector('img')!.dispatchEvent(new Event('error'));
    fixture.detectChanges();
    expect(host.querySelector('img')).toBeNull();
    expect(host.querySelector('svg.issue-type-fallback')).not.toBeNull();

    fixture.componentRef.setInput('iconUrl', 'https://jira.example/task.svg');
    fixture.detectChanges();
    expect(host.querySelector('img')?.getAttribute('src')).toBe('https://jira.example/task.svg');
  });

  it('names an unknown type in the tooltip', () => {
    const host = render('', '').nativeElement as HTMLElement;
    expect(host.getAttribute('title')).toBe('Tipe tidak diketahui');
  });
});
