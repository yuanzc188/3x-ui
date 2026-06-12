import { describe, expect, it } from 'vitest';

import { forwardSaveUrl, inboundAlreadyBound } from './helpers';
import type { ForwardRule } from './types';

const rule = (over: Partial<ForwardRule>): ForwardRule => ({
  id: 0, inboundTag: 'in-1000-tcp', destType: 'socks',
  destAddress: '1.2.3.4', destPort: 1080, enable: true, ...over,
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
