import { Component, inject } from '@angular/core';
import { NgIf } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { environment } from '../../../environments/environment';
import { IconComponent } from '../../shared/ui/icon.component';

@Component({
  selector: 'app-login',
  standalone: true,
  imports: [NgIf, IconComponent],
  templateUrl: './login.component.html',
})
export class LoginComponent {
  private route = inject(ActivatedRoute);

  protected errorMessage = this.route.snapshot.queryParamMap.get('error');
  protected googleLoginHref = `${environment.apiBasePath}/auth/google/login`;
}
