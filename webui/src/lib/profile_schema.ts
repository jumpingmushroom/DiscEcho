// Hand-mirrored from daemon/api/profile_schema.go.
// Server is authoritative; this file drives form-field hints in the
// editor (engine select options, format select per engine, etc).
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
  options: Record<string, OptionSpec>;
  stepCount: number;
}

export const ENGINES: Record<string, EngineSpec> = {
  whipper: {
    formats: ['FLAC'],
    options: {},
    stepCount: 6,
  },
  MakeMKV: {
    formats: ['MKV'],
    options: {
      min_title_seconds: { type: 'int' },
      keep_all_tracks: { type: 'bool' },
    },
    stepCount: 6,
  },
  'MakeMKV+HandBrake': {
    formats: ['MKV'],
    options: {
      min_title_seconds: { type: 'int' },
      keep_all_tracks: { type: 'bool' },
    },
    stepCount: 7,
  },
  HandBrake: {
    formats: ['MP4', 'MKV'],
    options: {
      min_title_seconds: { type: 'int' },
      season: { type: 'int' },
    },
    stepCount: 7,
  },
  'redumper+chdman': {
    formats: ['CHD'],
    options: {},
    stepCount: 7,
  },
  redumper: {
    formats: ['ISO'],
    options: {},
    stepCount: 5,
  },
  dd: {
    formats: ['ISO'],
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

export function engineNames(): string[] {
  return Object.keys(ENGINES);
}

export function specFor(engine: string): EngineSpec | undefined {
  return ENGINES[engine];
}
