import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { keys } from '@/api/queryKeys';
import { forwardSaveUrl } from './helpers';
import type { ForwardRule, ForwardSettings } from './types';

async function fetchRules(): Promise<ForwardRule[]> {
  const msg = await HttpUtil.get<ForwardRule[]>('/panel/api/forward/list', undefined, {
    silent: true,
  });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch forward rules');
  return Array.isArray(msg.obj) ? (msg.obj as ForwardRule[]) : [];
}

async function fetchSettings(): Promise<ForwardSettings> {
  const msg = await HttpUtil.get<ForwardSettings>('/panel/api/forward/settings', undefined, {
    silent: true,
  });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch forward settings');
  return msg.obj ?? { globalDomains: '', checkUrl: '' };
}

export function usePortForward() {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: keys.portForward.root() });

  const query = useQuery({ queryKey: keys.portForward.list(), queryFn: fetchRules });
  const settingsQuery = useQuery({ queryKey: keys.portForward.settings(), queryFn: fetchSettings });

  const saveMut = useMutation({
    mutationFn: (rule: Partial<ForwardRule>) =>
      HttpUtil.post(forwardSaveUrl({ id: rule.id ?? 0 }), rule),
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });
  const removeMut = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/forward/del/${id}`),
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });
  const enableMut = useMutation({
    mutationFn: ({ id, enable }: { id: number; enable: boolean }) =>
      HttpUtil.post(`/panel/api/forward/setEnable/${id}`, { enable }),
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });
  const checkMut = useMutation({
    mutationFn: (id: number) => HttpUtil.post<ForwardRule>(`/panel/api/forward/check/${id}`),
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });
  const settingsMut = useMutation({
    mutationFn: (s: ForwardSettings) => HttpUtil.post('/panel/api/forward/settings', s),
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });

  return {
    rules: query.data ?? [],
    loading: query.isFetching,
    error: (query.error as Error | null) ?? null,
    settings: settingsQuery.data ?? null,
    refresh: invalidate,
    save: (rule: Partial<ForwardRule>) => saveMut.mutateAsync(rule),
    remove: (id: number) => removeMut.mutateAsync(id),
    setEnable: (id: number, enable: boolean) => enableMut.mutateAsync({ id, enable }),
    checkNow: (id: number) => checkMut.mutateAsync(id),
    saveSettings: (s: ForwardSettings) => settingsMut.mutateAsync(s),
  };
}
