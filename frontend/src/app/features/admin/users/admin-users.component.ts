import { Component, OnInit, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { NgFor, NgIf } from '@angular/common';
import { AdminService, UserRow, formatActivationDate } from '../../../core/services/admin.service';
import { ChatAppService } from '../../../core/services/chat-app.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { UserAvatarComponent, adminUserAvatarUrl } from '../../../shared/ui/user-avatar.component';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';

@Component({
  selector: 'app-admin-users',
  standalone: true,
  imports: [NgFor, NgIf, FormsModule, RouterLink, IconComponent, UserAvatarComponent],
  templateUrl: './admin-users.component.html',
})
export class AdminUsersComponent implements OnInit {
  protected readonly avatarUrl = adminUserAvatarUrl;
  protected readonly activationDate = formatActivationDate;
  protected users: UserRow[] = [];
  protected loading = true;
  protected error: string | null = null;
  protected query = '';
  protected onlyNotActivated = false;
  protected rowActionInFlight: Record<number, boolean> = {};
  protected rowMessage: Record<number, string> = {};
  protected jiraAccountIdDrafts: Record<number, string> = {};

  private adminService = inject(AdminService);
  private confirm = inject(ConfirmService);
  private toast = inject(ToastService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);
  private chatApp = inject(ChatAppService);

  /** The operator account's display name (AC4.4), from the status endpoint. */
  protected operatorName(): string {
    return this.chatApp.status()?.operatorName || 'Operator';
  }

  ngOnInit(): void {
    this.onlyNotActivated = this.route.snapshot.queryParamMap.get('chatApp') === 'inactive';
    if (!this.chatApp.status()) {
      this.chatApp.load().subscribe({ error: () => undefined });
    }
    this.loadUsers();
  }

  protected get filteredUsers(): UserRow[] {
    const q = this.query.trim().toLowerCase();
    return this.users.filter(
      (u) => (!q || u.email.toLowerCase().includes(q)) && (!this.onlyNotActivated || (u.flagActive && !u.chatAppActivated)),
    );
  }

  /** Row click → the user's detail page (settings, mute, credential). */
  protected open(user: UserRow): void {
    this.router.navigate(['/admin/users', user.userId]);
  }

  protected loadUsers(): void {
    // Only the very first load shows the skeleton; reloads after an action
    // keep the table on screen so the page doesn't flicker.
    this.loading = this.users.length === 0;
    this.error = null;

    this.adminService.listUsers().subscribe({
      next: (res) => {
        this.users = res.users;
        this.jiraAccountIdDrafts = Object.fromEntries(res.users.map((u) => [u.userId, u.jiraAccountId]));
        this.loading = false;
      },
      error: (err) => {
        this.error = err?.status === 403 ? 'Kamu tidak punya akses ke halaman ini' : 'Gagal memuat daftar user, coba lagi';
        this.loading = false;
      },
    });
  }

  protected onToggleActive(user: UserRow): void {
    this.startRowAction(user);
    this.adminService.toggleActive(user.userId, !user.flagActive).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        this.toast.success(user.flagActive ? 'User dinonaktifkan' : 'User diaktifkan', user.email);
        this.loadUsers();
      },
      error: (err) => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = err?.error?.error ?? 'Gagal mengubah status user';
      },
    });
  }

  protected onSaveJiraAccountId(user: UserRow): void {
    const value = this.jiraAccountIdDrafts[user.userId];
    this.startRowAction(user);
    this.adminService.editJiraAccountId(user.userId, value).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        user.jiraAccountId = value;
        this.toast.success('Jira Account ID tersimpan', user.email);
      },
      error: (err) => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = err?.error?.error ?? 'Gagal menyimpan Jira Account ID';
      },
    });
  }

  protected async onDeleteCredential(user: UserRow): Promise<void> {
    const confirmed = await this.confirm.ask({
      title: 'Hapus credential user?',
      message: `Credential Jira pribadi milik ${user.email} akan dihapus. Aksi Jira user ini kembali memakai akun bot.`,
      confirmText: 'Hapus credential',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    this.startRowAction(user);
    this.adminService.deleteCredential(user.userId).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        this.toast.success('Credential dihapus', user.email);
        this.loadUsers();
      },
      error: () => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = 'Gagal menghapus credential';
      },
    });
  }

  protected async onDeleteUser(user: UserRow): Promise<void> {
    const confirmed = await this.confirm.ask({
      title: 'Hapus user permanen?',
      message: `${user.email} akan dihapus beserta datanya. Tindakan ini tidak bisa dibatalkan.`,
      confirmText: 'Hapus user',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    this.startRowAction(user);
    this.adminService.deleteUser(user.userId).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        this.toast.success('User dihapus', user.email);
        this.loadUsers();
      },
      error: (err) => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = err?.error?.error ?? 'Gagal menghapus user';
      },
    });
  }

  private startRowAction(user: UserRow): void {
    this.rowActionInFlight[user.userId] = true;
    delete this.rowMessage[user.userId];
  }
}
