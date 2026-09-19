import type { ForwardRule } from './types';

// forwardSaveUrl picks the add vs update endpoint based on whether the rule
// already has a persisted id.
export function forwardSaveUrl(rule: Pick<ForwardRule, 'id'>): string {
  return rule.id ? `/panel/api/forward/update/${rule.id}` : '/panel/api/forward/add';
}

// inboundAlreadyBound reports whether another rule (not editingId) already binds
// inboundTag — enforces "one forward rule per inbound" in the UI before saving.
export function inboundAlreadyBound(
  rules: ForwardRule[],
  inboundTag: string,
  editingId: number,
): boolean {
  return rules.some((r) => r.inboundTag === inboundTag && r.id !== editingId);
}

export type WhitelistMode = 'off' | 'global' | 'custom';

// whitelistMode mirrors the server's three-state resolution (effectiveDomains).
export function whitelistMode(r: Pick<ForwardRule, 'domainLimit' | 'domains'>): WhitelistMode {
  if (!r.domainLimit) return 'off';
  return (r.domains ?? '').trim() ? 'custom' : 'global';
}

export type ExpiryLevel = 'none' | 'ok' | 'soon' | 'expired';

const SOON_MS = 3 * 86_400_000;

// expiryLevel matches the daily reminder job: within 3 days = soon.
export function expiryLevel(expiryTime: number, now: number): ExpiryLevel {
  if (!expiryTime || expiryTime <= 0) return 'none';
  if (expiryTime <= now) return 'expired';
  if (expiryTime - now < SOON_MS) return 'soon';
  return 'ok';
}
