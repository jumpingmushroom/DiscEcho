import { writable } from 'svelte/store';

export type ToastKind = 'success' | 'error';

export interface Toast {
  id: number;
  kind: ToastKind;
  message: string;
}

export const toasts = writable<Toast[]>([]);

// How long a toast stays on screen before auto-dismiss.
const TOAST_TTL_MS = 3500;

let nextId = 1;

export function pushToast(kind: ToastKind, message: string): void {
  const id = nextId++;
  toasts.update((cur) => [...cur, { id, kind, message }]);
  setTimeout(() => dismissToast(id), TOAST_TTL_MS);
}

export function dismissToast(id: number): void {
  toasts.update((cur) => cur.filter((t) => t.id !== id));
}
