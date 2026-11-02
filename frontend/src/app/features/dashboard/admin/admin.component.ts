import { Component, OnInit, inject } from '@angular/core';
import { NgFor, NgIf } from '@angular/common';
import { AdminService, UserRow } from '../../../core/services/admin.service';

@Component({
  selector: 'app-admin',
  standalone: true,
  imports: [NgFor, NgIf],
  templateUrl: './admin.component.html',
})
export class AdminComponent implements OnInit {
  protected users: UserRow[] = [];
  protected loading = true;
  protected error: string | null = null;

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
        this.loading = false;
      },
      error: (err) => {
        this.error = err?.status === 403 ? 'Kamu tidak punya akses ke halaman ini' : 'Gagal memuat daftar user, coba lagi';
        this.loading = false;
      },
    });
  }
}
