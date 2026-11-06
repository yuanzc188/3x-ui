import { fireEvent, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import ClientUsageModal from '@/pages/clients/ClientUsageModal';
import type { UsageLogDay } from '@/generated/types';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';

function serve(day: UsageLogDay) {
  return vi.mocked(HttpUtil.get).mockImplementation(async () => new Msg(true, '', day));
}

describe('ClientUsageModal', () => {
  it('lists the day newest first and marks blocked destinations', async () => {
    const get = serve({
      day: '2026-11-06',
      enabled: true,
      retentionDays: 30,
      truncated: false,
      entries: [
        {
          time: 1793944920000,
          destination: 'udp:8.8.8.8:53',
          outbound: 'forward-block',
          blocked: true,
        },
        {
          time: 1793944860000,
          destination: 'www.tiktok.com:443',
          outbound: 'fwd-1',
          blocked: false,
        },
      ],
    });
    renderWithProviders(<ClientUsageModal open email="shop a@x" onOpenChange={() => {}} />);

    await screen.findByText('www.tiktok.com:443');
    expect(get).toHaveBeenCalledWith(
      '/panel/api/clients/usageLog/shop%20a%40x',
      { day: '', filter: '' },
      { silent: true },
    );
    const rows = Array.from(document.querySelectorAll('.ant-table-tbody tr.ant-table-row'));
    expect(rows.map((r) => r.querySelector('code')?.textContent)).toEqual([
      'udp:8.8.8.8:53',
      'www.tiktok.com:443',
    ]);
    expect(rows[0].textContent).toContain('Blocked');
    expect(rows[1].textContent).toContain('fwd-1');
    await waitFor(() =>
      expect((document.querySelector('.ant-picker input') as HTMLInputElement).value).toBe(
        '2026-11-06',
      ),
    );

    fireEvent.change(screen.getByPlaceholderText('Filter by domain'), {
      target: { value: ' tiktok ' },
    });
    fireEvent.keyDown(screen.getByPlaceholderText('Filter by domain'), { key: 'Enter' });
    await waitFor(() =>
      expect(get).toHaveBeenLastCalledWith(
        '/panel/api/clients/usageLog/shop%20a%40x',
        { day: '', filter: 'tiktok' },
        { silent: true },
      ),
    );
  });

  it('warns when the access log is off and when the list was cut short', async () => {
    serve({ day: '2026-11-06', enabled: false, retentionDays: 30, truncated: true, entries: [] });
    renderWithProviders(<ClientUsageModal open email="alice" onOpenChange={() => {}} />);

    expect(await screen.findByText(/Xray access log is off/)).toBeTruthy();
    expect(screen.getByText(/Showing the newest 0 entries/)).toBeTruthy();
  });
});
