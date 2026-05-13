// DiscEcho M1.1 wire types — manually mirrored from daemon/state/types.go.
// Update both files when the contract changes.

export type DiscType =
  | 'AUDIO_CD'
  | 'DVD'
  | 'BDMV'
  | 'UHD'
  | 'PSX'
  | 'PS2'
  | 'XBOX'
  | 'SAT'
  | 'DC'
  | 'VCD'
  | 'DATA';

export type DriveState = 'idle' | 'identifying' | 'ripping' | 'ejecting' | 'error';

export type JobState =
  | 'queued'
  | 'identifying'
  | 'running'
  | 'paused'
  | 'done'
  | 'failed'
  | 'cancelled'
  | 'interrupted';

export type StepID =
  | 'detect'
  | 'identify'
  | 'rip'
  | 'transcode'
  | 'compress'
  | 'move'
  | 'notify'
  | 'eject';

export type JobStepState = 'pending' | 'running' | 'done' | 'skipped' | 'failed';

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface Drive {
  id: string;
  model: string;
  bus: string;
  dev_path: string;
  state: DriveState;
  last_seen_at: string;
  notes?: string;
  current_disc_id?: string;
}

export interface Candidate {
  source: string;
  title: string;
  artist?: string;
  year?: number;
  confidence: number;
  mbid?: string;
  tmdb_id?: number;
  media_type?: 'movie' | 'tv' | '';
}

export interface HistoryRow {
  job: Job;
  disc: Disc;
}

export interface HistoryResponse {
  rows: HistoryRow[];
  total: number;
  limit: number;
  offset: number;
}

export interface Disc {
  id: string;
  drive_id?: string;
  type: DiscType;
  title?: string;
  year?: number;
  runtime_seconds?: number;
  size_bytes_raw?: number;
  toc_hash?: string;
  metadata_provider?: string;
  metadata_id?: string;
  metadata_json?: string; // raw JSON blob with per-disc-type extended fields
  candidates: Candidate[];
  created_at: string;
}

export interface JobStep {
  step: StepID;
  state: JobStepState;
  attempt_count: number;
  started_at?: string;
  finished_at?: string;
  notes?: Record<string, unknown>;
}

export interface Job {
  id: string;
  disc_id: string;
  drive_id?: string;
  profile_id: string;
  state: JobState;
  active_step?: StepID;
  progress: number;
  speed?: string;
  eta_seconds?: number;
  elapsed_seconds?: number;
  started_at?: string;
  finished_at?: string;
  error_message?: string;
  created_at: string;
  steps?: JobStep[];
}

export interface Profile {
  id: string;
  disc_type: string;
  name: string;
  engine: string;
  format?: string;
  preset?: string;
  container: string;
  video_codec: string;
  quality_preset: string;
  hdr_pipeline: string;
  drive_policy: string;
  auto_eject: boolean;
  options: Record<string, unknown>;
  output_path_template: string;
  enabled: boolean;
  step_count: number;
  created_at: string;
  updated_at: string;
}

export interface Notification {
  id: string;
  name: string;
  url: string;
  tags: string;
  triggers: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface SnapshotPayload {
  drives: Drive[];
  jobs: Job[];
  discs: Disc[];
  profiles: Profile[];
  settings: Record<string, string>;
}

// SSE event types
export type SSEEvent =
  | { event: 'drive.changed'; data: { drive: Drive } }
  | { event: 'disc.detected'; data: { disc: Disc } }
  | { event: 'disc.identified'; data: { disc: Disc; candidates: Candidate[] } }
  | { event: 'job.created'; data: { job: Job } }
  | {
      event: 'job.step';
      data: { job_id: string; step: StepID; state: JobStepState; notes?: Record<string, unknown> };
    }
  | {
      event: 'job.progress';
      data: { job_id: string; step: StepID; pct: number; speed: string; eta_seconds: number };
    }
  | { event: 'job.log'; data: { job_id: string; t: string; level: LogLevel; message: string } }
  | { event: 'job.done'; data: { job_id: string } }
  | { event: 'job.failed'; data: { job_id: string; error?: string; state?: 'cancelled' } }
  | { event: 'state.snapshot'; data: SnapshotPayload };
