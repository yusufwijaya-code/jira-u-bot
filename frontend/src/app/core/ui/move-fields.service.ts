import { Injectable, signal } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { JiraField } from '../../shared/ui/jira-field-input.component';

/** The move a ticket is waiting on. */
export interface MoveTransition { id: string; name: string; to: string }

/**
 * A move that needs fields filled first — the 422 "fields_required" body of
 * POST /tickets/:key/transition and the GET move-requirements answer.
 */
export interface MoveRequirement {
  issueKey: string;
  issueUrl?: string;
  message: string;
  transition: MoveTransition;
  fields: JiraField[];
}

export interface MoveDone { message: string; status: string }

/** The fields_required body of a failed move, or null for any other error. */
export function fieldsRequired(err: unknown): MoveRequirement | null {
  const e = err as HttpErrorResponse | null;
  const body = e?.error;
  if (e?.status !== 422 || !body || body.error !== 'fields_required' || !body.transition) return null;
  return { issueKey: body.issueKey ?? '', issueUrl: body.issueUrl ?? '', message: body.message ?? '', transition: body.transition, fields: body.fields ?? [] };
}

interface Pending { req: MoveRequirement; resolve: (done: MoveDone | null) => void }

/**
 * Opens the "Lengkapi dulu sebelum pindah" dialog (rendered by
 * <app-overlays>). open() resolves with the move's result once the fields
 * were saved and the ticket moved, or null when the user cancelled.
 */
@Injectable({ providedIn: 'root' })
export class MoveFieldsService {
  readonly current = signal<Pending | null>(null);

  open(req: MoveRequirement): Promise<MoveDone | null> {
    this.current()?.resolve(null);
    return new Promise((resolve) => this.current.set({ req, resolve }));
  }

  close(done: MoveDone | null): void {
    const pending = this.current();
    if (!pending) return;
    this.current.set(null);
    pending.resolve(done);
  }
}
