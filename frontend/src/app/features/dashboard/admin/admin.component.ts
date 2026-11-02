import { Component, OnInit, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { NgFor, NgIf } from '@angular/common';
import { AdminService, UserRow } from '../../../core/services/admin.service';
import { BroadcastComponent } from './broadcast.component';

@Component({
  selector: 'app-admin',
  standalone: true,
  imports: [NgFor, NgIf, FormsModule, BroadcastComponent],
  templateUrl: './admin.component.html',
})
export class AdminComponent implements OnInit {
  protected users: UserRow[] = [];
  protected loading = true;
  protected error: string | null = null;
  protected rowActionInFlight: Record<number, boolean> = {};
  protected rowMessage: Record<number, string> = {};
  protected jiraAccountIdDrafts: Record<number, string> = {};

  private adminService = inject(AdminService);

  ngOnInit(): void {
    this.loadUsers();
  }

  protected loadUsers(): void {
    this.loading = true;
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
    this.rowActionInFlight[user.userId] = true;
    this.adminService.toggleActive(user.userId, !user.flagActive).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
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
    this.rowActionInFlight[user.userId] = true;
    this.adminService.editJiraAccountId(user.userId, value).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        user.jiraAccountId = value;
      },
      error: (err) => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = err?.error?.error ?? 'Gagal menyimpan Jira Account ID';
      },
    });
  }

  protected onDeleteCredential(user: UserRow): void {
    if (!confirm('Hapus credential pribadi user ini?')) {
      return;
    }
    this.rowActionInFlight[user.userId] = true;
    this.adminService.deleteCredential(user.userId).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        this.loadUsers();
      },
      error: () => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = 'Gagal menghapus credential';
      },
    });
  }

  protected onDeleteUser(user: UserRow): void {
    if (!confirm('Hapus user ini secara permanen?')) {
      return;
    }
    this.rowActionInFlight[user.userId] = true;
    this.adminService.deleteUser(user.userId).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        this.loadUsers();
      },
      error: (err) => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = err?.error?.error ?? 'Gagal menghapus user';
      },
    });
  }
}
