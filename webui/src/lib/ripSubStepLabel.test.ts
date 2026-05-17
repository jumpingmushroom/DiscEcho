import { describe, it, expect } from 'vitest';
import { ripSubStepLabel } from './ripSubStepLabel';

describe('ripSubStepLabel', () => {
  it.each([
    ['DUMP', 'Read raw data'],
    ['', 'Read raw data'],
    [undefined, 'Read raw data'],
    [null, 'Read raw data'],
    ['DUMP::EXTRA', 'Reading lead-in / lead-out'],
    ['PROTECTION', 'Checking protection'],
    ['REFINE', 'Recovering damaged sectors (this can take a while)'],
    ['DVDKEY', 'Extracting DVD keys'],
    ['SPLIT', 'Splitting tracks'],
    ['unknown', 'unknown'],
  ] as const)('%s → %s', (input, want) => {
    expect(ripSubStepLabel(input)).toBe(want);
  });
});
