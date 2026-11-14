import { ComponentFixture, TestBed } from '@angular/core/testing';
import { UserAvatarComponent, adminUserAvatarUrl, myAvatarUrl } from './user-avatar.component';

describe('UserAvatarComponent', () => {
  let fixture: ComponentFixture<UserAvatarComponent>;
  const el = () => fixture.nativeElement as HTMLElement;

  beforeEach(() => {
    fixture = TestBed.createComponent(UserAvatarComponent);
    fixture.componentRef.setInput('name', 'yusuf@esb.co.id');
  });

  it('shows the picture when a src is given', () => {
    fixture.componentRef.setInput('src', '/nudge/api/dashboard/me/avatar');
    fixture.detectChanges();

    expect(el().querySelector('img')!.getAttribute('src')).toBe('/nudge/api/dashboard/me/avatar');
    expect(el().classList).toContain('has-photo');
  });

  it('falls back to the initial when the picture fails to load', () => {
    fixture.componentRef.setInput('src', '/nudge/api/dashboard/me/avatar');
    fixture.detectChanges();

    el().querySelector('img')!.dispatchEvent(new Event('error'));
    fixture.detectChanges();

    expect(el().querySelector('img')).toBeNull();
    expect(el().textContent!.trim()).toBe('Y');
  });

  it('shows only the initial without a src, with the size class', () => {
    fixture.componentRef.setInput('size', 'xl');
    fixture.detectChanges();

    expect(el().querySelector('img')).toBeNull();
    expect(el().textContent!.trim()).toBe('Y');
    expect(el().classList).toContain('user-avatar');
    expect(el().classList).toContain('xl');
  });

  it('builds the avatar URLs', () => {
    expect(myAvatarUrl()).toBe('/nudge/api/dashboard/me/avatar');
    expect(adminUserAvatarUrl(12)).toBe('/nudge/api/dashboard/admin/users/12/avatar');
  });
});
