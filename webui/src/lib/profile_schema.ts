// Hand-mirrored from daemon/api/profile_schema.go.
// Server is authoritative; this file drives form-field hints in the
// editor (engine select options, container/codec/format select per
// engine, etc).
//
// When the server adds an engine the client schema goes stale — UI
// degrades to "engine unknown to client" hint, server still validates
// correctly.

export type OptionType = 'string' | 'int' | 'bool';

export interface OptionSpec {
  type: OptionType;
  required?: boolean;
}

export interface EngineSpec {
  formats: string[];
  containers: string[];
  videoCodecs: string[];
  options: Record<string, OptionSpec>;
  stepCount: number;
}

export const ENGINES: Record<string, EngineSpec> = {
  whipper: {
    formats: ['FLAC'],
    containers: ['FLAC'],
    videoCodecs: [],
    options: {},
    stepCount: 6,
  },
  MakeMKV: {
    formats: ['MKV'],
    containers: ['MKV'],
    videoCodecs: ['copy'],
    options: {
      min_title_seconds: { type: 'int' },
      keep_all_tracks: { type: 'bool' },
    },
    stepCount: 6,
  },
  'MakeMKV+HandBrake': {
    formats: ['MKV'],
    containers: ['MKV'],
    videoCodecs: ['x265', 'x264', 'nvenc_h265', 'nvenc_h264', 'av1', 'copy'],
    options: {
      min_title_seconds: { type: 'int' },
      keep_all_tracks: { type: 'bool' },
    },
    stepCount: 7,
  },
  HandBrake: {
    formats: ['MP4', 'MKV'],
    containers: ['MP4', 'MKV'],
    videoCodecs: ['x265', 'x264', 'nvenc_h265', 'nvenc_h264', 'av1'],
    options: {
      min_title_seconds: { type: 'int' },
      season: { type: 'int' },
    },
    stepCount: 7,
  },
  'redumper+chdman': {
    formats: ['CHD'],
    containers: ['CHD'],
    videoCodecs: [],
    options: {},
    stepCount: 7,
  },
  redumper: {
    formats: ['ISO'],
    containers: ['ISO'],
    videoCodecs: [],
    options: {},
    stepCount: 5,
  },
  dd: {
    formats: ['ISO'],
    containers: ['ISO'],
    videoCodecs: [],
    options: {},
    stepCount: 5,
  },
};

export const DISC_TYPES: ReadonlyArray<string> = [
  'AUDIO_CD',
  'DVD',
  'BDMV',
  'UHD',
  'PSX',
  'PS2',
  'SAT',
  'DC',
  'XBOX',
  'DATA',
];

// HDR_PIPELINES mirrors daemon/api/profile_schema.go HDRPipelines. The
// empty string is the per-engine default (audio/data/games engines have
// no HDR concept).
export const HDR_PIPELINES: ReadonlyArray<string> = [
  '',
  'passthrough',
  'hdr10plus',
  'tone-map-sdr',
  'strip',
];

export const HDR_PIPELINE_LABELS: Record<string, string> = {
  '': '—',
  passthrough: 'HDR10+ passthrough',
  hdr10plus: 'HDR10+ enhance',
  'tone-map-sdr': 'Tone-map to SDR',
  strip: 'Strip HDR',
};

// DRIVE_POLICIES mirrors daemon/api/profile_schema.go DrivePolicies.
// "drv-N" pins to a specific drive id (the UI also accepts whatever
// drive ids are currently attached).
export const DRIVE_POLICIES: ReadonlyArray<string> = ['any', 'drv-1', 'drv-2', 'drv-3'];

export const DRIVE_POLICY_LABELS: Record<string, string> = {
  any: 'Any available',
  'drv-1': 'Pin to drv-1',
  'drv-2': 'Pin to drv-2',
  'drv-3': 'Pin to drv-3',
};

export function engineNames(): string[] {
  return Object.keys(ENGINES);
}

export function specFor(engine: string): EngineSpec | undefined {
  return ENGINES[engine];
}
