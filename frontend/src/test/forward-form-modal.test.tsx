import { useState } from 'react';
import { act, fireEvent, screen, waitFor } from '@testing-library/react';
import dayjs from 'dayjs';
import { describe, expect, it, vi } from 'vitest';

import ForwardFormModal from '@/pages/port-forward/form/ForwardFormModal';
import type { ForwardRule } from '@/pages/port-forward/types';
import { renderWithProviders } from './test-utils';

vi.mock('@/api/queries/useInboundOptions', () => ({
  useInboundOptions: () => ({
    data: [{ id: 1, tag: 'in-1', remark: 'A', protocol: 'vless', port: 443, enable: true }],
    isLoading: false,
  }),
}));

const rule: ForwardRule = {
  id: 7,
  inboundTag: 'in-1',
  remark: 'shop A',
  destType: 'socks',
  destAddress: '1.2.3.4',
  destPort: 1080,
  enable: true,
  domainLimit: false,
  expiryTime: 0,
  checkedAt: 0,
  checkOk: false,
  checkMs: 0,
};

type SaveFn = (r: Partial<ForwardRule>) => Promise<{ success: boolean }>;

function renderModal(editing: ForwardRule | null) {
  const onSave = vi.fn<SaveFn>(async () => ({ success: true }));
  renderWithProviders(
    <ForwardFormModal open editing={editing} rules={[]} onCancel={() => {}} onSave={onSave} />,
  );
  return onSave;
}

// Let macrotask-batched form watchers and picker close effects land.
const settle = () => act(() => new Promise((resolve) => setTimeout(resolve, 30)));

async function saved(onSave: ReturnType<typeof vi.fn<SaveFn>>): Promise<Partial<ForwardRule>> {
  const ok = document.querySelector('.ant-modal-footer .ant-btn-primary');
  if (!ok) throw new Error('OK button not rendered');
  fireEvent.click(ok);
  await waitFor(() => expect(onSave).toHaveBeenCalled());
  return onSave.mock.calls[0][0];
}

describe('ForwardFormModal', () => {
  it('keeps a clicked provider-expiry date and saves it', async () => {
    const onSave = renderModal(rule);
    const input = document.querySelector('.ant-picker input') as HTMLInputElement;
    fireEvent.mouseDown(input);
    fireEvent.click(input);
    const day = dayjs().add(1, 'day');
    const cell = document.querySelector(
      `.ant-picker-cell[title="${day.format('YYYY-MM-DD')}"] .ant-picker-cell-inner`,
    );
    if (!cell) throw new Error('day cell not rendered');
    fireEvent.click(cell);
    await settle();

    expect(input.value).toBe(day.format('YYYY-MM-DD'));
    expect((await saved(onSave)).expiryTime).toBe(day.startOf('day').valueOf());
  });

  it('replaces the port with a pasted number instead of splicing it into the old one', async () => {
    const onSave = renderModal(rule);
    const port = document.getElementById('destPort') as HTMLInputElement;
    fireEvent.paste(port, { clipboardData: { getData: () => ' 10001\n' } });
    await settle();

    expect(port.value).toBe('10001');
    expect((await saved(onSave)).destPort).toBe(10001);
  });

  it.each([
    ['203.0.113.42:8080:alice:pass:word', '203.0.113.42', 8080, 'alice', 'pass:word'],
    [' [2001:db8::1]：１０８０：用户：密:码\n', '2001:db8::1', 1080, '用户', '密:码'],
  ])(
    'fills and saves all four destination fields from %s',
    async (text, destAddress, destPort, username, password) => {
      const onSave = renderModal(rule);
      const address = document.getElementById('destAddress') as HTMLInputElement;
      expect(fireEvent.paste(address, { clipboardData: { getData: () => text } })).toBe(false);
      expect(address.value).toBe(destAddress);
      expect(await saved(onSave)).toMatchObject({ destAddress, destPort, username, password });
    },
  );

  it('leaves a normal address paste and a malformed proxy to the browser', () => {
    renderModal(rule);
    const address = document.getElementById('destAddress') as HTMLInputElement;
    for (const text of ['example.com', 'proxy.example:65536:alice:password']) {
      expect(fireEvent.paste(address, { clipboardData: { getData: () => text } })).toBe(true);
    }
    expect((document.getElementById('destPort') as HTMLInputElement).value).toBe('1080');
  });

  it('opens Add with empty fields after editing another rule', async () => {
    function Harness() {
      const [state, setState] = useState({ open: true, editing: rule as ForwardRule | null });
      return (
        <>
          <button type="button" onClick={() => setState({ open: false, editing: rule })}>
            close
          </button>
          <button type="button" onClick={() => setState({ open: true, editing: null })}>
            add
          </button>
          <ForwardFormModal
            open={state.open}
            editing={state.editing}
            rules={[]}
            onCancel={() => {}}
            onSave={async () => ({ success: true })}
          />
        </>
      );
    }
    renderWithProviders(<Harness />);
    await settle();
    fireEvent.click(screen.getByText('close'));
    await settle();
    fireEvent.click(screen.getByText('add'));
    await settle();

    expect((document.getElementById('destAddress') as HTMLInputElement).value).toBe('');
    expect((document.getElementById('remark') as HTMLInputElement).value).toBe('');
  });
});
