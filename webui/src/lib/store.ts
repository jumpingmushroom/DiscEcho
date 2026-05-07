import { writable } from 'svelte/store';
import { apiGet, apiPost, apiPut, apiDelete } from './api';
import { connectSSE, type LiveStatus } from './sse';
import type {
  Drive,
  Disc,
  Job,
  Profile,
  LogLevel,
  SnapshotPayload,
  HistoryRow,
  HistoryResponse,
  DiscType,
  Candidate,
} from './wire';

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
export const selectedJobID = writable<string | null>(null);
export const selectedProfileID = writable<string | null>(null);

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
  'profile.changed',
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

    case 'profile.changed': {
      if (p.deleted) {
        const id = p.profile_id as string;
        profiles.update((arr) => arr.filter((q) => q.id !== id));
        selectedProfileID.update((cur) => (cur === id ? null : cur));
      } else {
        const prof = p.profile as Profile;
        profiles.update((arr) => upsertById(arr, prof));
      }
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

// ----- History --------------------------------------------------------------

export const history = writable<HistoryRow[]>([]);
export const historyTotal = writable<number>(0);
export const historyLoading = writable<boolean>(false);
export const historyError = writable<string | null>(null);
export const historyFilter = writable<DiscType | ''>('');

const HISTORY_PAGE_SIZE = 50;

/**
 * fetchHistoryPage replaces (offset=0) or appends (offset>0) into the
 * history store. Tracks loading/error state. Returns the count of rows
 * appended/replaced so callers can detect "no more pages".
 *
 * On HTTP error: historyError is set with a short message; previous
 * rows stay so retry doesn't blank the screen.
 */
export async function fetchHistoryPage(filter: DiscType | '', offset: number): Promise<number> {
  historyLoading.set(true);
  historyError.set(null);
  try {
    const params = new URLSearchParams();
    if (filter) params.set('type', filter);
    params.set('limit', String(HISTORY_PAGE_SIZE));
    params.set('offset', String(offset));

    const payload = await apiGet<HistoryResponse>(`/api/history?${params.toString()}`);
    const rows = payload.rows ?? [];

    if (offset === 0) {
      history.set(rows);
    } else {
      history.update((cur) => [...cur, ...rows]);
    }
    historyTotal.set(payload.total);
    return rows.length;
  } catch (e) {
    historyError.set((e as Error).message);
    return 0;
  } finally {
    historyLoading.set(false);
  }
}

// ----- Manual identify ------------------------------------------------------

/**
 * manualIdentify hits POST /api/discs/:id/identify with a search query
 * and updates the disc in the discs store with the refreshed candidates.
 * Throws on HTTP error so the caller (the disc-id sheet) can render the
 * failure inline; contrast fetchHistoryPage which swallows.
 */
export async function manualIdentify(
  discID: string,
  query: string,
  mediaType: 'movie' | 'tv' | 'both' = 'both',
): Promise<Candidate[]> {
  const payload = await apiPost<{ disc: Disc; candidates: Candidate[] }>(
    `/api/discs/${discID}/identify`,
    { query, media_type: mediaType },
  );
  const cands = payload.candidates ?? [];

  discs.update((m) => {
    const existing = m[discID];
    if (!existing) return m;
    return { ...m, [discID]: { ...existing, candidates: cands } };
  });

  return cands;
}

// ----- Profile mutations -----------------------------------------------------

/**
 * createProfile POSTs a new profile. Daemon generates id + timestamps.
 * On success the daemon broadcasts profile.changed; the local
 * $profiles store updates via the SSE handler — callers don't need
 * to update it manually.
 *
 * On 422: throws an Error whose message starts "HTTP 422: ..."; the
 * caller can use parseValidationErrors() from api.ts to extract the
 * field map.
 */
export async function createProfile(
  p: Omit<Profile, 'id' | 'created_at' | 'updated_at'>,
): Promise<Profile> {
  return apiPost<Profile>('/api/profiles', p);
}

/**
 * updateProfile PUTs to /api/profiles/:id with the full profile.
 */
export async function updateProfile(id: string, p: Profile): Promise<Profile> {
  return apiPut<Profile>(`/api/profiles/${id}`, p);
}

/**
 * deleteProfile DELETEs /api/profiles/:id. Resolves on 204 success.
 * 404 surfaces as "HTTP 404: ..."; 409 (active job conflict) as
 * "HTTP 409: profile is referenced by ...".
 */
export async function deleteProfile(id: string): Promise<void> {
  await apiDelete(`/api/profiles/${id}`);
}
