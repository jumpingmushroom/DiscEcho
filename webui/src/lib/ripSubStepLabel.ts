/**
 * Returns a human-readable label for the current redumper sub-phase.
 * Used by drive cards to show what the rip step is actually doing when
 * redumper is in a long quiet phase (REFINE, SPLIT, DVDKEY, …).
 */
export function ripSubStepLabel(substep: string | undefined | null): string {
  switch (substep) {
    case 'DUMP':
    case '':
    case undefined:
    case null:
      return 'Read raw data';
    case 'DUMP::EXTRA':
      return 'Reading lead-in / lead-out';
    case 'PROTECTION':
      return 'Checking protection';
    case 'REFINE':
      return 'Recovering damaged sectors (this can take a while)';
    case 'DVDKEY':
      return 'Extracting DVD keys';
    case 'SPLIT':
      return 'Splitting tracks';
    default:
      return substep;
  }
}
