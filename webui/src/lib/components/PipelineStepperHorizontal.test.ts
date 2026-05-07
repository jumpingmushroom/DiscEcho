import '@testing-library/jest-dom/vitest';
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import PipelineStepperHorizontal from './PipelineStepperHorizontal.svelte';
import type { Job } from '$lib/wire';

const baseJob: Job = {
  id: 'job-1',
  disc_id: 'disc-1',
  drive_id: 'd1',
  profile_id: 'p1',
  state: 'queued',
  progress: 0,
  created_at: '2026-05-07T12:00:00Z',
};

describe('PipelineStepperHorizontal', () => {
  it('renders all 8 step labels', () => {
    const { getByText } = render(PipelineStepperHorizontal, { job: baseJob });
    for (const label of [
      'Detect',
      'Identify',
      'Rip',
      'Transcode',
      'Compress',
      'Move',
      'Notify',
      'Eject',
    ]) {
      expect(getByText(label)).toBeInTheDocument();
    }
  });

  it('marks the active step', () => {
    const job: Job = {
      ...baseJob,
      state: 'running',
      active_step: 'rip',
      progress: 42.5,
    };
    const { container } = render(PipelineStepperHorizontal, { job });
    const active = container.querySelector('[data-step-state="active"]');
    expect(active).not.toBeNull();
    expect(active?.getAttribute('data-step')).toBe('rip');
  });

  it('renders skipped steps with the skipped state', () => {
    const job: Job = {
      ...baseJob,
      state: 'running',
      active_step: 'rip',
      progress: 10,
      steps: [{ step: 'compress', state: 'skipped', attempt_count: 0 }],
    };
    const { container } = render(PipelineStepperHorizontal, { job });
    const skipped = container.querySelector('[data-step="compress"]');
    expect(skipped?.getAttribute('data-step-state')).toBe('skipped');
  });

  it('renders done steps with the done state', () => {
    const job: Job = {
      ...baseJob,
      state: 'running',
      active_step: 'rip',
      progress: 10,
      steps: [
        { step: 'detect', state: 'done', attempt_count: 0 },
        { step: 'identify', state: 'done', attempt_count: 0 },
      ],
    };
    const { container } = render(PipelineStepperHorizontal, { job });
    const detect = container.querySelector('[data-step="detect"]');
    expect(detect?.getAttribute('data-step-state')).toBe('done');
    const identify = container.querySelector('[data-step="identify"]');
    expect(identify?.getAttribute('data-step-state')).toBe('done');
  });
});
