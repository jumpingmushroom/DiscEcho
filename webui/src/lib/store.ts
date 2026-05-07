import { writable } from 'svelte/store';
import { apiGet, apiPost } from './api';
import { connectSSE, type LiveStatus } from './sse';
import type { Drive, Disc, Job, Profile, LogLevel, SnapshotPayload } from './wire';

export interface LogLine {
  job_id: string;
  t: string;
  level: LogLevel;
  message: string;
}

// ----- Stores ---------------------------------------------------------------

export const drives = writable<Drive[]>([]);
export const jobs = writable<Job[]>([]);
export const profiles = writable<Profile[]>([]);
export const settings = writable<Record<string, string>>({});
export const discs = writable<Record<string, Disc>>({});
export const logs = writable<Record<string, LogLine[]>>({});
export const liveStatus = writable<LiveStatus>('connecting');
export const pendingDiscID = writable<string | null>(null);

const LOG_RING_SIZE = 50;
const SSE_EVENT_NAMES = [
  'state.snapshot',
  'drive.changed',
  'disc.detected',
  'disc.identified',
  'job.created',
  'job.step',
  'job.progress',
  'job.log',
  'job.done',
  'job.failed',
];

// ----- Bootstrap ------------------------------------------------------------

export async function bootstrap(): Promise<void> {
  const snap = await apiGet<SnapshotPayload>('/api/state');
  drives.set(snap.drives ?? []);
  jobs.set(snap.jobs ?? []);
  profiles.set(snap.profiles ?? []);
  settings.set(snap.settings ?? {});
}

// ----- SSE event dispatch ---------------------------------------------------

export function handleSSEEvent(name: string, payload: unknown): void {
  // Payloads are validated by the daemon; we trust the shape here and narrow
  // per-case. A typed discriminated dispatch would cost more than it saves.
  const p = payload as Record<string, unknown>;

  switch (name) {
    case 'state.snapshot': {
      const snap = p as unknown as SnapshotPayload;
      drives.set(snap.drives ?? []);
      jobs.set(snap.jobs ?? []);
      profiles.set(snap.profiles ?? []);
      settings.set(snap.settings ?? {});
      break;
    }

    case 'drive.changed':
      drives.update((arr) => upsertById(arr, p.drive as Drive));
      break;

    case 'disc.detected': {
      const d = p.disc as Disc;
      discs.update((m) => ({ ...m, [d.id]: d }));
      pendingDiscID.set(d.id);
      break;
    }

    case 'disc.identified': {
      const base = p.disc as Disc;
      const d: Disc = { ...base, candidates: (p.candidates as Disc['candidates']) ?? [] };
      discs.update((m) => ({ ...m, [d.id]: d }));
      break;
    }

    case 'job.created': {
      const j = p.job as Job;
      jobs.update((arr) => [j, ...arr.filter((x) => x.id !== j.id)]);
      pendingDiscID.update((cur) => (cur === j.disc_id ? null : cur));
      break;
    }

    case 'job.step': {
      const jobID = p.job_id as string;
      const step = p.step as Job['active_step'];
      const stepState = p.state as NonNullable<Job['steps']>[number]['state'];
      jobs.update((arr) =>
        arr.map((j) => {
          if (j.id !== jobID) return j;
          const steps = (j.steps ?? []).map((s) =>
            s.step === step ? { ...s, state: stepState } : s,
          );
          return { ...j, active_step: step, steps };
        }),
      );
      break;
    }

    case 'job.progress': {
      const jobID = p.job_id as string;
      jobs.update((arr) =>
        arr.map((j) =>
          j.id !== jobID
            ? j
            : {
                ...j,
                active_step: p.step as Job['active_step'],
                progress: p.pct as number,
                speed: p.speed as string | undefined,
                eta_seconds: p.eta_seconds as number | undefined,
              },
        ),
      );
      break;
    }

    case 'job.log': {
      const jobID = p.job_id as string;
      const line: LogLine = {
        job_id: jobID,
        t: p.t as string,
        level: p.level as LogLevel,
        message: p.message as string,
      };
      logs.update((m) => {
        const cur = m[jobID] ?? [];
        const next = [...cur, line];
        if (next.length > LOG_RING_SIZE) {
          next.splice(0, next.length - LOG_RING_SIZE);
        }
        return { ...m, [jobID]: next };
      });
      break;
    }

    case 'job.done': {
      const jobID = p.job_id as string;
      jobs.update((arr) => arr.map((j) => (j.id === jobID ? { ...j, state: 'done' as const } : j)));
      break;
    }

    case 'job.failed': {
      const jobID = p.job_id as string;
      const cancelled = p.state === 'cancelled';
      const error = p.error as string | undefined;
      jobs.update((arr) =>
        arr.map((j) =>
          j.id === jobID
            ? {
                ...j,
                state: (cancelled ? 'cancelled' : 'failed') as Job['state'],
                error_message: error,
              }
            : j,
        ),
      );
      break;
    }
  }
}

function upsertById<T extends { id: string }>(arr: T[], item: T): T[] {
  const idx = arr.findIndex((x) => x.id === item.id);
  if (idx < 0) return [...arr, item];
  const next = arr.slice();
  next[idx] = item;
  return next;
}

// ----- Connection lifecycle ------------------------------------------------

export function connect(): () => void {
  const conn = connectSSE('/api/events', SSE_EVENT_NAMES, handleSSEEvent, {
    onStatusChange: (s) => liveStatus.set(s),
  });
  return () => conn.close();
}

// ----- Imperatives ---------------------------------------------------------

export async function startDisc(
  discID: string,
  profileID: string,
  candidateIndex?: number,
): Promise<Job> {
  const body: { profile_id: string; candidate_index?: number } = { profile_id: profileID };
  if (candidateIndex !== undefined) body.candidate_index = candidateIndex;
  return apiPost<Job>(`/api/discs/${discID}/start`, body);
}

export async function cancelJob(jobID: string): Promise<void> {
  await apiPost<void>(`/api/jobs/${jobID}/cancel`);
}

export async function ejectDrive(driveID: string): Promise<void> {
  await apiPost<void>(`/api/drives/${driveID}/eject`);
}
