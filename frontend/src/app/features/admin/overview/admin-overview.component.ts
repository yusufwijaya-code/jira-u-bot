import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { RouterLink } from '@angular/router';
import { AdminService, UserRow } from '../../../core/services/admin.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { UserAvatarComponent, adminUserAvatarUrl } from '../../../shared/ui/user-avatar.component';

/** How many users each shortcut list shows. */
const SHORTLIST_SIZE = 5;

/**
 * Admin › Dashboard (Tech Spec "Admin Panel Revamp", AC1.5): user stats plus
 * shortcuts to the users that need attention.
 */
@Component({
  selector: 'app-admin-overview',
  standalone: true,
  imports: [NgTemplateOutlet, RouterLink, IconComponent, UserAvatarComponent],
  templateUrl: './admin-overview.component.html',
})
export class AdminOverviewComponent implements OnInit {
  protected readonly avatarUrl = adminUserAvatarUrl;
  private adminService = inject(AdminService);

  protected loading = signal(true);
  protected error = signal<string | null>(null);
  protected users = signal<UserRow[]>([]);

  protected activeCount = computed(() => this.users().filter((u) => u.flagActive).length);
  protected credentialCount = computed(() => this.users().filter((u) => u.hasCredential).length);
  protected withoutCredential = computed(() => this.users().filter((u) => !u.hasCredential).slice(0, SHORTLIST_SIZE));
  protected inactive = computed(() => this.users().filter((u) => !u.flagActive).slice(0, SHORTLIST_SIZE));

  ngOnInit(): void {
    this.adminService.listUsers().subscribe({
      next: (res) => {
        this.users.set(res.users ?? []);
        this.loading.set(false);
      },
      error: (err) => {
        this.error.set(err?.status === 403 ? 'Kamu tidak punya akses ke halaman ini' : 'Gagal memuat data user, coba lagi');
        this.loading.set(false);
      },
    });
  }
}
