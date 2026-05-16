import { writable, derived, get, type Readable } from 'svelte/store';
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
  Notification,
  Stats,
  StepID,
  JobDetailResponse,
  JobLogsResponse,
} from './wire';

export interface LogLine {
  job_id: string;
  t: string;
  step?: StepID | '';
  level: LogLevel;
  message: string;
}

// ----- Stores ---------------------------------------------------------------

export const drives = writable<Drive[]>([]);
export const jobs = writable<Job[]>([]);
export const profiles = writable<Profile[]>([]);
export const settings = writable<Record<string, string>>({});
export const notifications = writable<Notification[]>([]);
export const discs = writable<Record<string, Disc>>({});
export const logs = writable<Record<string, LogLine[]>>({});
export const liveStatus = writable<LiveStatus>('connecting');
export const pendingDiscID = writable<string | null>(null);
export const selectedJobID = writable<string | null>(null);
export const stats = writable<Stats | undefined>(undefined);

let statsRefreshTimer: ReturnType<typeof setTimeout> | null = null;

// scheduleStatsRefresh debounces a /api/stats refresh. The dashboard
// calls this on job lifecycle SSE events; a 5-minute setInterval
// (set up in connect()) also polls so library `total_bytes` drift
// from external file changes eventually surfaces.
function scheduleStatsRefresh(delayMs: number = 1000): void {
  if (statsRefreshTimer) clearTimeout(statsRefreshTimer);
  statsRefreshTimer = setTimeout(async () => {
    try {
      const fresh = await apiGet<Stats>('/api/stats');
      stats.set(fresh);
    } catch {
      // Soft fail: widget keeps last value.
    }
    statsRefreshTimer = null;
  }, delayMs);
}
export const selectedProfileID = writable<string | null>(null);

// bootCodeCounts is keyed by DiscType ('PSX' | 'PS2' | 'SAT' | 'DC' | 'XBOX')
// and reflects how many entries are loaded in the embedded boot-code maps.
// Populated from /api/system when the daemon ships the field (Phase 8).
// A value of 0 (or missing key) means auto-id is unavailable for that system.
export const bootCodeCounts = writable<Record<string, number>>({});

// LOG_RING_SIZE caps the per-job in-memory log buffer. Raised from 50
// so a chatty HandBrake transcode doesn't push earlier rip-phase lines
// out of the ring before the user can switch to the Rip filter chip
// on /jobs/[id]. ~300 lines is ~12KB per active job and GC'd when the
// job leaves $jobs.
const LOG_RING_SIZE = 300;
const SSE_EVENT_NAMES = [
  'state.snapshot',
  'drive.changed',
  'disc.detected',
  'disc.identified',
  'disc.deleted',
  'job.created',
  'job.step',
  'job.progress',
  'job.log',
  'job.done',
  'job.failed',
  'profile.changed',
  'notification.changed',
  'settings.changed',
];

// ----- Bootstrap ------------------------------------------------------------

export async function bootstrap(): Promise<void> {
  const snap = await apiGet<SnapshotPayload>('/api/state');
  drives.set(snap.drives ?? []);
  jobs.set(snap.jobs ?? []);
  profiles.set(snap.profiles ?? []);
  settings.set(snap.settings ?? {});
  discs.set(Object.fromEntries((snap.discs ?? []).map((d) => [d.id, d])));
  stats.set(snap.stats);
  // Notifications fetched separately (not in the snapshot).
  try {
    const ns = await apiGet<Notification[]>('/api/notifications');
    notifications.set(ns ?? []);
  } catch {
    // Soft-fail: settings page can still render an empty list.
    notifications.set([]);
  }
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
      discs.set(Object.fromEntries((snap.discs ?? []).map((d) => [d.id, d])));
      stats.set(snap.stats);
      break;
    }

    case 'drive.changed': {
      // Daemon publishes the partial `{drive_id, state}` shape — never
      // a full Drive object — so we patch the matching entry in place
      // instead of upserting. Without this the dashboard only learned
      // about state transitions on a full page reload.
      const driveID = p.drive_id as string;
      const newState = p.state as Drive['state'];
      drives.update((arr) => arr.map((d) => (d.id === driveID ? { ...d, state: newState } : d)));
      break;
    }

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

    case 'disc.deleted': {
      const id = p.disc_id as string;
      discs.update((m) => {
        const { [id]: _drop, ...rest } = m;
        return rest;
      });
      pendingDiscID.update((cur) => (cur === id ? null : cur));
      break;
    }

    case 'job.created': {
      const j = p.job as Job;
      jobs.update((arr) => [j, ...arr.filter((x) => x.id !== j.id)]);
      pendingDiscID.update((cur) => (cur === j.disc_id ? null : cur));
      scheduleStatsRefresh();
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
          // First step transitioning out of pending while the job is
          // still queued is the moment the orchestrator actually
          // picked it up. Flip job.state so the dashboard stops
          // reading "QUEUED" while a step is already running.
          const promoted = j.state === 'queued' && stepState === 'running' ? 'running' : j.state;
          return { ...j, active_step: step, state: promoted, steps };
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
      const step = p.step as StepID | '' | undefined;
      const line: LogLine = {
        job_id: jobID,
        t: p.t as string,
        step: step ?? '',
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
      scheduleStatsRefresh();
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
      scheduleStatsRefresh();
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

    case 'notification.changed': {
      if (p.deleted) {
        const id = p.notification_id as string;
        notifications.update((arr) => arr.filter((n) => n.id !== id));
      } else {
        const n = p.notification as Notification;
        notifications.update((arr) => upsertById(arr, n));
      }
      break;
    }

    case 'settings.changed': {
      // Daemon publishes {keys: [...]} (key names only). Re-fetch the
      // map so subscribers see the new values. Fire-and-forget — store
      // updates flow through reactive bindings.
      void refreshSettings();
      break;
    }
  }
}

async function refreshSettings(): Promise<void> {
  try {
    const fresh = await apiGet<Record<string, string>>('/api/settings');
    settings.set(fresh);
  } catch {
    // Soft-fail: settings will resync on next bootstrap.
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
  // Five-minute fallback poll so library.total_bytes drift from external
  // file changes eventually surfaces. SSE-driven refreshes are the
  // primary signal; this is belt-and-braces.
  const poller = setInterval(() => scheduleStatsRefresh(0), 5 * 60 * 1000);
  return () => {
    conn.close();
    clearInterval(poller);
    if (statsRefreshTimer) clearTimeout(statsRefreshTimer);
  };
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

// fetchJob loads a single job + its disc from the daemon. Used by
// /jobs/[id] when the requested id isn't in the live $jobs snapshot
// (i.e. a terminal job reached from /history). Side-effects: upserts
// the disc into the `discs` store so DiscArt + DiscTypeBadge can read
// it the same way they do for live jobs.
export async function fetchJob(jobID: string): Promise<JobDetailResponse> {
  const res = await apiGet<JobDetailResponse>(`/api/jobs/${jobID}`);
  discs.update((m) => ({ ...m, [res.disc.id]: res.disc }));
  return res;
}

export interface FetchJobLogsParams {
  step?: StepID | '';
  limit?: number;
  offset?: number;
}

// fetchJobLogs loads persisted log lines for one job, optionally
// filtered by phase. Used by LogPhaseViewer for terminal jobs and on
// page-reload during a live job (the SSE ring starts empty after a
// reload — the daemon stream only forwards new lines).
export async function fetchJobLogs(
  jobID: string,
  params: FetchJobLogsParams = {},
): Promise<JobLogsResponse> {
  const q = new URLSearchParams();
  if (params.step) q.set('step', params.step);
  if (params.limit !== undefined) q.set('limit', String(params.limit));
  if (params.offset !== undefined) q.set('offset', String(params.offset));
  const qs = q.toString();
  const suffix = qs ? `?${qs}` : '';
  return apiGet<JobLogsResponse>(`/api/jobs/${jobID}/logs${suffix}`);
}

// In-flight set keyed by jobID so two cards mounting for the same
// running job don't race the same HTTP fetch.
const logBackfillInFlight = new Set<string>();

// ensureLogBackfill fetches persisted log lines for a running job
// when the in-memory ring is empty — i.e. the page mounted after the
// job started and SSE has no lines for it yet. Without this the
// dashboard's log tail panel sits at "No log lines yet" for the entire
// pre-rip warmup phase (1–3 min) even though the daemon has been
// logging the whole time. Safe to call multiple times: it no-ops when
// there's anything already in the ring, when an identical fetch is
// already running, or on network failure.
export async function ensureLogBackfill(jobID: string): Promise<void> {
  if (!jobID) return;
  const existing = get(logs)[jobID];
  if (existing && existing.length > 0) return;
  if (logBackfillInFlight.has(jobID)) return;
  logBackfillInFlight.add(jobID);
  try {
    const res = await fetchJobLogs(jobID, { limit: LOG_RING_SIZE });
    const fetched = res.lines ?? [];
    if (fetched.length === 0) return;
    logs.update((m) => {
      const live = m[jobID] ?? [];
      // De-dup by (t + message): SSE may have raced in between the
      // backfill request and this merge, so an identical line could
      // appear in both. Backfill is older — prepend it.
      const seen = new Set(live.map((l) => `${l.t} ${l.message}`));
      const merged = [...fetched.filter((l) => !seen.has(`${l.t} ${l.message}`)), ...live];
      if (merged.length > LOG_RING_SIZE) {
        merged.splice(0, merged.length - LOG_RING_SIZE);
      }
      return { ...m, [jobID]: merged };
    });
  } catch {
    // Soft fail: SSE will eventually fill the tail with new lines.
  } finally {
    logBackfillInFlight.delete(jobID);
  }
}

// deleteJob removes a single terminal job from history. The daemon
// refuses non-terminal states with 409; callers should only call this
// from the terminal-state branch of /jobs/[id]. Clears the local
// $jobs / $logs entries on success so the page can navigate away
// without stale data popping back.
export async function deleteJob(jobID: string): Promise<void> {
  await apiDelete(`/api/jobs/${jobID}`);
  jobs.update((arr) => arr.filter((j) => j.id !== jobID));
  logs.update((m) => {
    const { [jobID]: _drop, ...rest } = m;
    return rest;
  });
  scheduleStatsRefresh();
}

// skipDisc removes the disc row server-side so the awaiting-decision
// card is dismissed permanently — without this, the row stays in the
// DB and AwaitingDecisionList re-derives the card on every page load.
// The daemon refuses to delete a disc that has any job history, so
// this is only callable on truly orphan rows.
export async function skipDisc(discID: string): Promise<void> {
  await apiDelete(`/api/discs/${discID}`);
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

/**
 * clearHistory wipes all finished-rip history server-side, then nudges
 * a debounced /api/stats refresh so the dashboard counters reflect the
 * now-empty history. Returns the number of jobs deleted. Throws on HTTP
 * error so the caller can surface it inline; the caller is responsible
 * for re-fetching the history list afterward.
 */
export async function clearHistory(): Promise<number> {
  const res = await apiPost<{ deleted: number }>('/api/history/clear');
  scheduleStatsRefresh();
  return res.deleted;
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

/**
 * reidentify re-runs the full classify + identify pipeline against the
 * drive the disc is currently in, replacing candidates and metadata
 * fields. Used by the drive card's "Re-identify" button when the prober
 * picked the wrong release. Updates the local discs store on success.
 */
export async function reidentify(discID: string): Promise<Candidate[]> {
  const payload = await apiPost<{ disc: Disc; candidates: Candidate[] }>(
    `/api/discs/${discID}/identify`,
    { force: true },
  );
  const cands = payload.candidates ?? [];
  const fresh = payload.disc;
  discs.update((m) => {
    const existing = m[discID];
    if (!existing) return m;
    return { ...m, [discID]: { ...existing, ...fresh, candidates: cands } };
  });
  return cands;
}

// ----- Derived settings -----------------------------------------------------

/**
 * operationMode reflects the daemon's operation.mode setting. "batch"
 * (default) keeps today's auto-confirm-after-8s behaviour; "manual"
 * suppresses the countdown so the user explicitly clicks Start on each
 * disc. Falls back to "batch" when the setting hasn't loaded yet.
 */
export const operationMode: Readable<'batch' | 'manual'> = derived(settings, ($s) =>
  $s['operation.mode'] === 'manual' ? 'manual' : 'batch',
);

/**
 * ejectOnFinish reflects rip.eject_on_finish. Defaults to true when
 * unset. Note: in manual mode the daemon ignores this and never ejects;
 * the UI surfaces it as disabled in that case.
 */
export const ejectOnFinish: Readable<boolean> = derived(settings, ($s) => {
  const v = $s['rip.eject_on_finish'];
  if (v === undefined || v === '') return true;
  return v === 'true';
});

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

// ----- Notification mutations -------------------------------------------------

export async function createNotification(
  n: Omit<Notification, 'id' | 'created_at' | 'updated_at'>,
): Promise<Notification> {
  return apiPost<Notification>('/api/notifications', n);
}

export async function updateNotification(id: string, n: Notification): Promise<Notification> {
  return apiPut<Notification>(`/api/notifications/${id}`, n);
}

export async function deleteNotification(id: string): Promise<void> {
  await apiDelete(`/api/notifications/${id}`);
}

export async function validateNotification(id: string): Promise<{ ok: boolean; error?: string }> {
  return apiPost(`/api/notifications/${id}/validate`, {});
}

/**
 * testNotification fires a test notification via the daemon. On Apprise
 * failure the daemon returns 502 with {sent:false,error}; apiPost throws
 * in that case, so we catch and normalise to the same shape.
 */
export async function testNotification(id: string): Promise<{ sent: boolean; error?: string }> {
  try {
    return await apiPost(`/api/notifications/${id}/test`, {});
  } catch (e) {
    const msg = (e as Error).message;
    // apiPost serialises non-2xx as `HTTP <status>: <body>`. Try to
    // extract the JSON body so we can return the structured shape.
    const m = msg.match(/^HTTP \d+:\s*(.*)$/s);
    if (m) {
      try {
        const parsed = JSON.parse(m[1]);
        if (typeof parsed === 'object' && parsed && 'sent' in parsed) {
          return parsed as { sent: boolean; error?: string };
        }
      } catch {
        // Body isn't JSON; fall through.
      }
    }
    return { sent: false, error: msg };
  }
}

export async function updateRetention(r: { forever: boolean; days: number }): Promise<void> {
  await apiPut('/api/settings', {
    'retention.forever': r.forever,
    'retention.days': r.days,
  });
}

/**
 * updateRipBehaviour persists the batch/manual mode toggle plus the
 * global eject-on-finish flag. The daemon broadcasts settings.changed
 * after a successful PUT, which the SSE handler folds back into
 * `$settings` and the derived `operationMode` / `ejectOnFinish` stores.
 */
export async function updateRipBehaviour(r: {
  mode: 'batch' | 'manual';
  ejectOnFinish: boolean;
}): Promise<void> {
  await apiPut('/api/settings', {
    'operation.mode': r.mode,
    'rip.eject_on_finish': r.ejectOnFinish,
  });
}
