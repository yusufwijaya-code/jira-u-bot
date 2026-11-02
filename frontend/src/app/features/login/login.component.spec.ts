import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { LoginComponent } from './login.component';

async function setup(queryParams: Record<string, string>) {
  await TestBed.configureTestingModule({
    imports: [LoginComponent],
    providers: [
      {
        provide: ActivatedRoute,
        useValue: { snapshot: { queryParamMap: convertToParamMap(queryParams) } },
      },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(LoginComponent);
  fixture.detectChanges();
  return fixture;
}

describe('LoginComponent', () => {
  it('does not show an error alert when no error query param is present', async () => {
    const fixture = await setup({});
    expect(fixture.nativeElement.querySelector('.alert-error')).toBeNull();
  });

  it('shows the error message verbatim when an error query param is present', async () => {
    const fixture = await setup({ error: 'Sesi login tidak valid atau kedaluwarsa, coba lagi' });
    const alert = fixture.nativeElement.querySelector('.alert-error');
    expect(alert?.textContent).toContain('Sesi login tidak valid atau kedaluwarsa, coba lagi');
  });

  it('points the Sign in with Google link at /jira-notif/auth/google/login', async () => {
    const fixture = await setup({});
    const link: HTMLAnchorElement = fixture.nativeElement.querySelector('.google-signin-btn');
    expect(link.getAttribute('href')).toBe('/jira-notif/auth/google/login');
  });

  it('renders the Sign in button as a plain anchor element, not a button with a click handler', async () => {
    const fixture = await setup({});
    const link = fixture.nativeElement.querySelector('.google-signin-btn');
    expect(link.tagName).toBe('A');
  });
});
