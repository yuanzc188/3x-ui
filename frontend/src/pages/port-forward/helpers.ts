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
