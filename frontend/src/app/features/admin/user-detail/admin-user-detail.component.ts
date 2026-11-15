import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { AdminService, UserRow, formatActivationDate } from '../../../core/services/admin.service';
import { ADMIN_TARGET_USER_ID, AdminUserScope, AdminUserSettingsApi } from '../../../core/services/admin-user-settings';
import { SETTINGS_MODE, SettingsApi, SettingsScope } from '../../../core/services/settings-api';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { UserAvatarComponent, adminUserAvatarUrl } from '../../../shared/ui/user-avatar.component';
import { SelectComponent, SelectOption } from '../../../shared/ui/select.component';
import { ChatSenderCardComponent } from '../../dashboard/settings/chat-sender-card.component';
import { SettingsComponent } from '../../dashboard/settings/settings.component';

/**
 * Admin › User › detail (Tech Spec "Admin Panel Revamp", AC3.2–AC3.6): the
 * user's profile plus the very same Pengaturan components, pointed at this
 * user through SettingsApi / SettingsScope and SETTINGS_MODE 'admin'.
 */
@Component({
  selector: 'app-admin-user-detail',
  standalone: true,
  imports: [ChatSenderCardComponent, FormsModule, RouterLink, IconComponent, UserAvatarComponent, SelectComponent, SettingsComponent],
  templateUrl: './admin-user-detail.component.html',
  providers: [
    { provide: ADMIN_TARGET_USER_ID, useFactory: () => Number(inject(ActivatedRoute).snapshot.paramMap.get('id')) },
    AdminUserSettingsApi,
    AdminUserScope,
    { provide: SettingsApi, useExisting: AdminUserSettingsApi },
    { provide: SettingsScope, useExisting: AdminUserScope },
    { provide: SETTINGS_MODE, useValue: 'admin' },
  ],
})
export class AdminUserDetailComponent implements OnInit {
  protected readonly avatarUrl = adminUserAvatarUrl;
  protected readonly activationDate = formatActivationDate;
  private adminService = inject(AdminService);
  private toast = inject(ToastService);
  protected userId = inject(ADMIN_TARGET_USER_ID);
  protected scope = inject(AdminUserScope);

  protected user = signal<UserRow | null>(null);
  protected loading = signal(true);
  protected notFound = signal(false);
  protected error = signal<string | null>(null);
  protected busy = signal(false);
  protected rowMessage = signal<string | null>(null);
  protected jiraAccountIdDraft = '';

  protected scopeOptions = computed<SelectOption<number | null>[]>(() => [
    { value: null, label: 'Default semua board' },
    ...this.scope.boards().map((b) => ({ value: b.boardId, label: `${b.name} · ${b.activeSprint.name}` })),
  ]);

  ngOnInit(): void {
    if (!Number.isInteger(this.userId) || this.userId <= 0) {
      this.notFound.set(true);
      this.loading.set(false);
      return;
    }
    this.loadUser();
    this.scope.load();
  }

  protected loadUser(): void {
    this.adminService.getUser(this.userId).subscribe({
      next: (user) => {
        this.user.set(user);
        this.jiraAccountIdDraft = user.jiraAccountId;
        this.loading.set(false);
      },
      error: (err) => {
        if (err?.status === 404) this.notFound.set(true);
        else this.error.set('Gagal memuat user, coba lagi');
        this.loading.set(false);
      },
    });
  }

  protected toggleActive(): void {
    const user = this.user();
    if (!user || this.busy()) return;
    this.busy.set(true);
    this.rowMessage.set(null);
    this.adminService.toggleActive(user.userId, !user.flagActive).subscribe({
      next: () => {
        this.busy.set(false);
        this.toast.success(user.flagActive ? 'User dinonaktifkan' : 'User diaktifkan', user.email);
        this.loadUser();
      },
      error: (err) => {
        this.busy.set(false);
        this.rowMessage.set(err?.error?.error ?? 'Gagal mengubah status user');
      },
    });
  }

  protected saveJiraAccountId(): void {
    const user = this.user();
    if (!user || this.busy()) return;
    this.busy.set(true);
    this.rowMessage.set(null);
    this.adminService.editJiraAccountId(user.userId, this.jiraAccountIdDraft).subscribe({
      next: () => {
        this.busy.set(false);
        this.toast.success('Jira Account ID tersimpan', user.email);
        this.user.set({ ...user, jiraAccountId: this.jiraAccountIdDraft });
        this.scope.load(); // boards come from the account id
      },
      error: (err) => {
        this.busy.set(false);
        this.rowMessage.set(err?.error?.error ?? 'Gagal menyimpan Jira Account ID');
      },
    });
  }
}
