import { describe, expect, it } from 'vitest';

import { expiryLevel, forwardSaveUrl, inboundAlreadyBound, whitelistMode } from './helpers';
import type { ForwardRule } from './types';

const rule = (over: Partial<ForwardRule>): ForwardRule => ({
  id: 0,
  inboundTag: 'in-1000-tcp',
  destType: 'socks',
  destAddress: '1.2.3.4',
  destPort: 1080,
  enable: true,
  domainLimit: false,
  expiryTime: 0,
  checkedAt: 0,
  checkOk: false,
  checkMs: 0,
  ...over,
});

describe('forwardSaveUrl', () => {
  it('uses add endpoint when id is absent', () => {
    expect(forwardSaveUrl(rule({ id: 0 }))).toBe('/panel/api/forward/add');
  });
  it('uses update endpoint when id is present', () => {
    expect(forwardSaveUrl(rule({ id: 5 }))).toBe('/panel/api/forward/update/5');
  });
});

describe('inboundAlreadyBound', () => {
  const existing = [rule({ id: 1, inboundTag: 'in-1000-tcp' })];
  it('flags a new rule binding an already-bound inbound', () => {
    expect(inboundAlreadyBound(existing, 'in-1000-tcp', 0)).toBe(true);
  });
  it('does not flag when editing the same rule', () => {
    expect(inboundAlreadyBound(existing, 'in-1000-tcp', 1)).toBe(false);
  });
  it('does not flag a free inbound', () => {
    expect(inboundAlreadyBound(existing, 'in-2000-tcp', 0)).toBe(false);
  });
});

describe('whitelistMode', () => {
  it('is off when the switch is off regardless of domains', () => {
    expect(whitelistMode({ domainLimit: false, domains: 'domain:x.com' })).toBe('off');
  });
  it('falls back to global when custom list is blank', () => {
    expect(whitelistMode({ domainLimit: true, domains: ' \n ' })).toBe('global');
    expect(whitelistMode({ domainLimit: true })).toBe('global');
  });
  it('is custom when a list is present', () => {
    expect(whitelistMode({ domainLimit: true, domains: 'domain:a.com' })).toBe('custom');
  });
});

describe('expiryLevel', () => {
  const now = 1_700_000_000_000;
  const day = 86_400_000;
  it('none for unset', () => {
    expect(expiryLevel(0, now)).toBe('none');
  });
  it('expired at or before now', () => {
    expect(expiryLevel(now, now)).toBe('expired');
    expect(expiryLevel(now - 1, now)).toBe('expired');
  });
  it('soon inside 3 days, ok at 3 days or beyond', () => {
    expect(expiryLevel(now + 2 * day, now)).toBe('soon');
    expect(expiryLevel(now + 3 * day, now)).toBe('ok');
  });
});
