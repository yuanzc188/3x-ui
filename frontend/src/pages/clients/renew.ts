import dayjs from 'dayjs';

export const RENEW_MONTH_OPTIONS = [1, 3, 12] as const;

// renewAddDays converts "+N calendar months" into the day delta the server's
// bulkAdjust endpoint applies to the client's current expiry. The extension
// starts from the later of expiry and now, so early renewals keep their
// remaining time and lapsed clients start counting from today. Rounded up so
// the customer never gets less than N months. null = not renewable
// (unlimited or delayed-start clients have no fixed expiry to extend).
export function renewAddDays(expiryTime: number, months: number, now: number): number | null {
  if (!expiryTime || expiryTime <= 0) return null;
  const base = Math.max(expiryTime, now);
  const target = dayjs(base).add(months, 'month').valueOf();
  return Math.ceil((target - expiryTime) / 86_400_000);
}
