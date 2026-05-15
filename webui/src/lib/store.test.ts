import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';
import {
  drives,
  jobs,
  profiles,
  settings,
  discs,
  logs,
  liveStatus,
  pendingDiscID,
  selectedJobID,
  selectedProfileID,
  notifications,
  bootstrap,
  handleSSEEvent,
  startDisc,
  cancelJob,
  createProfile,
  updateProfile,
  deleteProfile,
  createNotification,
  updateNotification,
  deleteNotification,
  validateNotification,
  testNotification,
  history,
  historyTotal,
  historyLoading,
  historyError,
  historyFilter,
  fetchHistoryPage,
  clearHistory,
  manualIdentify,
} from './store';
import type { Drive, Job, Disc, Profile } from './wire';

const seedDrive: Drive = {
  id: 'd1',
  model: 'X',
  bus: 'USB · sr0',
  dev_path: '/dev/sr0',
  state: 'idle',
  last_seen_at: '2026-05-07T12:00:00Z',
};
const seedProfile: Profile = {
  id: 'p1',
  disc_type: 'AUDIO_CD',
  name: 'CD-FLAC',
  engine: 'whipper',
  format: 'FLAC',
  preset: 'AccurateRip',
  container: 'FLAC',
  video_codec: '',
  quality_preset: 'AccurateRip',
  hdr_pipeline: '',
  drive_policy: 'any',
  auto_eject: true,
  options: {},
  output_path_template: '{{.Title}}',
  enabled: true,
  step_count: 6,
  created_at: '2026-05-07T12:00:00Z',
  updated_at: '2026-05-07T12:00:00Z',
};
const seedDisc: Disc = {
  id: 'disc-1',
  drive_id: 'd1',
  type: 'AUDIO_CD',
  candidates: [{ source: 'MusicBrainz', title: 'X', confidence: 90, mbid: 'mb-1' }],
  created_at: '2026-05-07T12:00:00Z',
};
const seedJob: Job = {
  id: 'job-1',
  disc_id: 'disc-1',
  drive_id: 'd1',
  profile_id: 'p1',
  state: 'queued',
  progress: 0,
  created_at: '2026-05-07T12:00:00Z',
};

function reset() {
  drives.set([]);
  jobs.set([]);
  profiles.set([]);
  settings.set({});
  discs.set({});
  logs.set({});
  liveStatus.set('connecting');
  pendingDiscID.set(null);
}

describe('bootstrap', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    reset();
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
    const store = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => store.set(k, v),
      removeItem: (k: string) => store.delete(k),
      clear: () => store.clear(),
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('hydrates stores from /api/state', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        drives: [seedDrive],
        jobs: [seedJob],
        profiles: [seedProfile],
        settings: { library_path: '/srv/media' },
      }),
    });
    // bootstrap also fetches /api/notifications separately
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => [],
    });

    await bootstrap();
    expect(get(drives)).toHaveLength(1);
    expect(get(drives)[0].id).toBe('d1');
    expect(get(jobs)).toHaveLength(1);
    expect(get(profiles)).toHaveLength(1);
    expect(get(settings)).toEqual({ library_path: '/srv/media' });
  });
});

describe('handleSSEEvent', () => {
  beforeEach(reset);

  it('state.snapshot replaces all stores', () => {
    drives.set([{ ...seedDrive, id: 'old' }]);
    handleSSEEvent('state.snapshot', {
      drives: [seedDrive],
      jobs: [],
      profiles: [],
      settings: {},
    });
    expect(get(drives)).toEqual([seedDrive]);
  });

  it('drive.changed patches state on the matching drive (partial payload)', () => {
    // Daemon publishes {drive_id, state}, NOT a full Drive. The handler
    // must patch the existing row, not upsert a (partially-typed) row.
    drives.set([seedDrive]);
    handleSSEEvent('drive.changed', { drive_id: seedDrive.id, state: 'ripping' });
    expect(get(drives)[0].state).toBe('ripping');
    expect(get(drives)).toHaveLength(1);
    // Unrelated fields preserved.
    expect(get(drives)[0].model).toBe(seedDrive.model);
  });

  it('disc.detected stores disc and sets pendingDiscID', () => {
    handleSSEEvent('disc.detected', { disc: seedDisc });
    expect(get(discs)['disc-1']).toEqual(seedDisc);
    expect(get(pendingDiscID)).toBe('disc-1');
  });

  it('disc.identified replaces disc with full candidates', () => {
    discs.set({ 'disc-1': { ...seedDisc, candidates: [] } });
    pendingDiscID.set('disc-1');
    handleSSEEvent('disc.identified', { disc: seedDisc, candidates: seedDisc.candidates });
    expect(get(discs)['disc-1'].candidates).toHaveLength(1);
  });

  it('job.created prepends and clears pendingDiscID for matching disc', () => {
    pendingDiscID.set('disc-1');
    handleSSEEvent('job.created', { job: seedJob });
    expect(get(jobs)).toEqual([seedJob]);
    expect(get(pendingDiscID)).toBeNull();
  });

  it('job.progress merges by id without losing other fields', () => {
    jobs.set([{ ...seedJob, state: 'running' }]);
    handleSSEEvent('job.progress', {
      job_id: 'job-1',
      step: 'rip',
      pct: 42.5,
      speed: '8×',
      eta_seconds: 30,
    });
    const j = get(jobs)[0];
    expect(j.progress).toBe(42.5);
    expect(j.speed).toBe('8×');
    expect(j.eta_seconds).toBe(30);
    expect(j.state).toBe('running');
  });

  it('job.step updates active_step + step state', () => {
    jobs.set([
      {
        ...seedJob,
        state: 'running',
        steps: [{ step: 'rip', state: 'pending', attempt_count: 0 }],
      },
    ]);
    handleSSEEvent('job.step', { job_id: 'job-1', step: 'rip', state: 'running' });
    const j = get(jobs)[0];
    expect(j.active_step).toBe('rip');
    expect(j.steps?.[0].state).toBe('running');
  });

  it('job.log ring-buffers up to 300 lines per job', () => {
    for (let i = 0; i < 320; i++) {
      handleSSEEvent('job.log', {
        job_id: 'job-1',
        t: '2026-05-07T12:00:00.000Z',
        level: 'info',
        message: `line ${i}`,
      });
    }
    const buf = get(logs)['job-1'];
    expect(buf).toHaveLength(300);
    expect(buf[0].message).toBe('line 20');
    expect(buf[299].message).toBe('line 319');
  });

  it('job.log carries the daemon-provided step tag', () => {
    handleSSEEvent('job.log', {
      job_id: 'job-1',
      t: '2026-05-07T12:00:00.000Z',
      step: 'rip',
      level: 'info',
      message: 'rip line',
    });
    const buf = get(logs)['job-1'];
    expect(buf?.[0].step).toBe('rip');
  });

  it('job.done sets state=done', () => {
    jobs.set([{ ...seedJob, state: 'running' }]);
    handleSSEEvent('job.done', { job_id: 'job-1' });
    expect(get(jobs)[0].state).toBe('done');
  });

  it('job.failed sets state and error_message', () => {
    jobs.set([{ ...seedJob, state: 'running' }]);
    handleSSEEvent('job.failed', { job_id: 'job-1', error: 'whipper exit 1' });
    expect(get(jobs)[0].state).toBe('failed');
    expect(get(jobs)[0].error_message).toBe('whipper exit 1');
  });

  it('job.failed with state=cancelled sets cancelled', () => {
    jobs.set([{ ...seedJob, state: 'running' }]);
    handleSSEEvent('job.failed', { job_id: 'job-1', state: 'cancelled' });
    expect(get(jobs)[0].state).toBe('cancelled');
  });
});

describe('imperatives', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    reset();
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('startDisc POSTs and returns the new job', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => seedJob,
    });
    const got = await startDisc('disc-1', 'p1', 0);
    expect(got).toEqual(seedJob);
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/discs/disc-1/start',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('cancelJob POSTs', async () => {
    fetchSpy.mockResolvedValueOnce({ ok: true, status: 204 });
    await cancelJob('job-1');
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/jobs/job-1/cancel',
      expect.objectContaining({ method: 'POST' }),
    );
  });
});

const seedHistoryRow = {
  job: {
    id: 'job-h1',
    disc_id: 'disc-h1',
    profile_id: 'p1',
    state: 'done' as const,
    progress: 100,
    finished_at: '2026-05-07T12:00:00Z',
    created_at: '2026-05-07T11:55:00Z',
  },
  disc: {
    id: 'disc-h1',
    type: 'AUDIO_CD' as const,
    title: 'Kind of Blue',
    candidates: [],
    created_at: '2026-05-07T11:55:00Z',
  },
};

describe('fetchHistoryPage', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    history.set([]);
    historyTotal.set(0);
    historyLoading.set(false);
    historyError.set(null);
    historyFilter.set('');
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('offset=0 replaces history and sets historyTotal', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        rows: [seedHistoryRow],
        total: 1,
        limit: 50,
        offset: 0,
      }),
    });

    const got = await fetchHistoryPage('', 0);
    expect(got).toBe(1);
    expect(get(history)).toHaveLength(1);
    expect(get(historyTotal)).toBe(1);
    expect(get(historyError)).toBeNull();
  });

  it('offset>0 appends without losing earlier rows', async () => {
    history.set([seedHistoryRow]);
    historyTotal.set(2);

    const second = { ...seedHistoryRow, job: { ...seedHistoryRow.job, id: 'job-h2' } };
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ rows: [second], total: 2, limit: 50, offset: 1 }),
    });

    await fetchHistoryPage('', 1);
    expect(get(history)).toHaveLength(2);
    expect(get(history)[0].job.id).toBe('job-h1');
    expect(get(history)[1].job.id).toBe('job-h2');
  });

  it('passes filter as ?type query param', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ rows: [], total: 0, limit: 50, offset: 0 }),
    });
    await fetchHistoryPage('DVD', 0);
    expect(fetchSpy).toHaveBeenCalled();
    const url = fetchSpy.mock.calls[0][0] as string;
    expect(url).toContain('type=DVD');
  });

  it('error sets historyError but keeps existing rows', async () => {
    history.set([seedHistoryRow]);
    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 500,
      text: async () => 'boom',
    });

    await fetchHistoryPage('', 0);
    expect(get(historyError)).toContain('500');
    expect(get(history)).toHaveLength(1); // kept
  });

  it('clears historyLoading after success', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ rows: [], total: 0, limit: 50, offset: 0 }),
    });
    await fetchHistoryPage('', 0);
    expect(get(historyLoading)).toBe(false);
  });
});

describe('manualIdentify', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    discs.set({});
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('POSTs and updates discs[id] with new candidates', async () => {
    discs.set({
      'disc-1': {
        id: 'disc-1',
        type: 'DVD',
        candidates: [],
        created_at: '2026-05-07T12:00:00Z',
      },
    });
    const newCands = [
      {
        source: 'TMDB',
        title: 'Found',
        year: 2020,
        confidence: 80,
        tmdb_id: 99,
        media_type: 'movie' as const,
      },
    ];
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        disc: {
          id: 'disc-1',
          type: 'DVD',
          candidates: newCands,
          created_at: '2026-05-07T12:00:00Z',
        },
        candidates: newCands,
      }),
    });

    const got = await manualIdentify('disc-1', 'Found', 'movie');
    expect(got).toEqual(newCands);
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/discs/disc-1/identify',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ query: 'Found', media_type: 'movie' }),
      }),
    );
    const updated = get(discs)['disc-1'];
    expect(updated.candidates).toEqual(newCands);
  });

  it('defaults media_type to both when omitted', async () => {
    discs.set({
      'disc-1': {
        id: 'disc-1',
        type: 'DVD',
        candidates: [],
        created_at: '2026-05-07T12:00:00Z',
      },
    });
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        disc: {
          id: 'disc-1',
          type: 'DVD',
          candidates: [],
          created_at: '2026-05-07T12:00:00Z',
        },
        candidates: [],
      }),
    });
    await manualIdentify('disc-1', 'X');
    const body = JSON.parse(fetchSpy.mock.calls[0][1].body);
    expect(body.media_type).toBe('both');
  });
});

describe('selectedJobID', () => {
  beforeEach(() => {
    selectedJobID.set(null);
  });

  it('defaults to null', () => {
    expect(get(selectedJobID)).toBeNull();
  });

  it('can be set and cleared', () => {
    selectedJobID.set('job-1');
    expect(get(selectedJobID)).toBe('job-1');
    selectedJobID.set(null);
    expect(get(selectedJobID)).toBeNull();
  });
});

const seedProfileFixture: Profile = {
  id: 'p1',
  disc_type: 'AUDIO_CD',
  name: 'CD-FLAC',
  engine: 'whipper',
  format: 'FLAC',
  preset: 'AccurateRip',
  container: 'FLAC',
  video_codec: '',
  quality_preset: 'AccurateRip',
  hdr_pipeline: '',
  drive_policy: 'any',
  auto_eject: true,
  options: {},
  output_path_template: '{{.Title}}.flac',
  enabled: true,
  step_count: 6,
  created_at: '2026-05-07T12:00:00Z',
  updated_at: '2026-05-07T12:00:00Z',
};

describe('selectedProfileID', () => {
  beforeEach(() => {
    selectedProfileID.set(null);
  });

  it('defaults to null', () => {
    expect(get(selectedProfileID)).toBeNull();
  });

  it('can be set and cleared', () => {
    selectedProfileID.set('p1');
    expect(get(selectedProfileID)).toBe('p1');
    selectedProfileID.set(null);
    expect(get(selectedProfileID)).toBeNull();
  });
});

describe('profile mutations', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    profiles.set([]);
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('createProfile POSTs and returns the response Profile', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => seedProfileFixture,
    });
    const got = await createProfile({
      disc_type: 'AUDIO_CD',
      name: 'CD-FLAC',
      engine: 'whipper',
      format: 'FLAC',
      preset: 'AccurateRip',
      container: 'FLAC',
      video_codec: '',
      quality_preset: 'AccurateRip',
      hdr_pipeline: '',
      drive_policy: 'any',
      auto_eject: true,
      options: {},
      output_path_template: '{{.Title}}.flac',
      enabled: true,
      step_count: 6,
    });
    expect(got.id).toBe('p1');
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe('/api/profiles');
    expect((init as RequestInit).method).toBe('POST');
  });

  it('updateProfile PUTs to /api/profiles/:id', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ ...seedProfileFixture, name: 'renamed' }),
    });
    const got = await updateProfile('p1', { ...seedProfileFixture, name: 'renamed' });
    expect(got.name).toBe('renamed');
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe('/api/profiles/p1');
    expect((init as RequestInit).method).toBe('PUT');
  });

  it('deleteProfile DELETEs and resolves to undefined on 204', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 204,
    });
    await deleteProfile('p1');
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe('/api/profiles/p1');
    expect((init as RequestInit).method).toBe('DELETE');
  });
});

describe('profile.changed SSE handler', () => {
  beforeEach(() => {
    profiles.set([seedProfileFixture]);
    selectedProfileID.set(null);
  });

  it('upserts on profile payload', () => {
    handleSSEEvent('profile.changed', {
      profile: { ...seedProfileFixture, name: 'renamed' },
    });
    const arr = get(profiles);
    expect(arr).toHaveLength(1);
    expect(arr[0].name).toBe('renamed');
  });

  it('inserts a new profile when ID is unknown', () => {
    handleSSEEvent('profile.changed', {
      profile: { ...seedProfileFixture, id: 'p2', name: 'new' },
    });
    const arr = get(profiles);
    expect(arr).toHaveLength(2);
    expect(arr.find((p) => p.id === 'p2')?.name).toBe('new');
  });

  it('removes on deleted payload AND clears selectedProfileID if matching', () => {
    selectedProfileID.set('p1');
    handleSSEEvent('profile.changed', { profile_id: 'p1', deleted: true });
    expect(get(profiles)).toHaveLength(0);
    expect(get(selectedProfileID)).toBeNull();
  });

  it('removes on deleted payload but leaves selectedProfileID alone if different', () => {
    selectedProfileID.set('p9');
    handleSSEEvent('profile.changed', { profile_id: 'p1', deleted: true });
    expect(get(profiles)).toHaveLength(0);
    expect(get(selectedProfileID)).toBe('p9');
  });
});

const seedNotification = {
  id: 'n-1',
  name: 'x',
  url: 'ntfy://a',
  tags: '',
  triggers: 'done',
  enabled: true,
  created_at: '',
  updated_at: '',
};

describe('notifications imperatives', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('createNotification POSTs and returns the row', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => seedNotification,
    });
    const got = await createNotification({
      name: 'x',
      url: 'ntfy://a',
      tags: '',
      triggers: 'done',
      enabled: true,
    });
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/notifications',
      expect.objectContaining({ method: 'POST' }),
    );
    expect(got.id).toBe('n-1');
  });

  it('updateNotification PUTs to /api/notifications/:id', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ ...seedNotification, name: 'updated' }),
    });
    await updateNotification('n-1', { ...seedNotification, name: 'updated' });
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/notifications/n-1',
      expect.objectContaining({ method: 'PUT' }),
    );
  });

  it('deleteNotification DELETEs', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 204,
    });
    await deleteNotification('n-1');
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/notifications/n-1',
      expect.objectContaining({ method: 'DELETE' }),
    );
  });

  it('validateNotification returns ok+error from JSON body', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ ok: false, error: 'bad url' }),
    });
    const res = await validateNotification('n-1');
    expect(res.ok).toBe(false);
    expect(res.error).toBe('bad url');
  });

  it('testNotification returns sent=true on 200', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ sent: true }),
    });
    const res = await testNotification('n-1');
    expect(res.sent).toBe(true);
  });

  it('testNotification handles 502 sent=false', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 502,
      text: async () => '{"sent":false,"error":"delivery failed"}',
    });
    const res = await testNotification('n-1');
    expect(res.sent).toBe(false);
    expect(res.error).toContain('delivery failed');
  });
});

describe('notification.changed SSE handler', () => {
  beforeEach(() => {
    notifications.set([]);
  });

  it('upserts on update payload', () => {
    handleSSEEvent('notification.changed', { notification: seedNotification });
    const arr = get(notifications);
    expect(arr.length).toBe(1);
    expect(arr[0].id).toBe('n-1');
  });

  it('removes on delete payload', () => {
    notifications.set([seedNotification]);
    handleSSEEvent('notification.changed', { notification_id: 'n-1', deleted: true });
    expect(get(notifications).length).toBe(0);
  });
});

describe('prefs', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('updateRetention PUTs the right keys', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ ok: true }),
    });
    const { updateRetention } = await import('./store');
    await updateRetention({ forever: false, days: 60 });
    const call = fetchSpy.mock.calls[0];
    const body = JSON.parse((call[1] as RequestInit).body as string);
    expect(body['retention.forever']).toBe(false);
    expect(body['retention.days']).toBe(60);
  });
});

describe('clearHistory', () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('POSTs to /api/history/clear and returns the deleted count', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ deleted: 7 }),
    });
    const got = await clearHistory();
    expect(got).toBe(7);
    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/history/clear',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('throws on HTTP error', async () => {
    fetchSpy.mockResolvedValueOnce({
      ok: false,
      status: 500,
      text: async () => 'boom',
    });
    await expect(clearHistory()).rejects.toThrow('500');
  });
});
