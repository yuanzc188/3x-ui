import dayjs from 'dayjs';
import { describe, expect, it } from 'vitest';

import { renewAddDays } from './renew';

const DAY = 86_400_000;

describe('renewAddDays', () => {
  const now = dayjs('2026-09-19T12:00:00Z').valueOf();

  it('extends from the current expiry when it is still in the future', () => {
    const expiry = dayjs('2026-10-01T00:00:00Z').valueOf();
    const days = renewAddDays(expiry, 1, now);
    // Oct 1 → Nov 1 = 31 days
    expect(days).toBe(31);
  });

  it('extends from today when the client has already lapsed', () => {
    const expiry = now - 40 * DAY;
    const days = renewAddDays(expiry, 1, now)!;
    const newExpiry = expiry + days * DAY;
    const target = dayjs(now).add(1, 'month').valueOf();
    expect(newExpiry).toBeGreaterThanOrEqual(target);
    expect(newExpiry - target).toBeLessThan(DAY);
  });

  it('never yields less than the requested months', () => {
    const expiry = dayjs('2026-09-19T23:30:00Z').valueOf();
    for (const m of [1, 3, 12]) {
      const days = renewAddDays(expiry, m, now)!;
      expect(expiry + days * DAY).toBeGreaterThanOrEqual(dayjs(expiry).add(m, 'month').valueOf());
    }
  });

  it('is null for unlimited or delayed-start clients', () => {
    expect(renewAddDays(0, 1, now)).toBeNull();
    expect(renewAddDays(-30 * DAY, 1, now)).toBeNull();
  });
});
