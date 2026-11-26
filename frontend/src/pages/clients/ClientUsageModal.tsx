import { useState } from 'react';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { DownloadOutlined } from '@ant-design/icons';
import { Alert, Button, Flex, Input, Modal, Table, Tag, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';

import { withBasePath } from '@/api/http-init';
import { keys } from '@/api/queryKeys';
import { DateTimePicker } from '@/components/form';
import type { UsageLogDay, UsageLogEntry } from '@/generated/types';
import { useDatepicker } from '@/hooks/useDatepicker';
import { HttpUtil, IntlUtil } from '@/utils';
import type { CalendarKind } from '@/utils';

interface ClientUsageModalProps {
  open: boolean;
  email: string;
  onOpenChange: (open: boolean) => void;
}

async function fetchUsage(email: string, day: string, filter: string): Promise<UsageLogDay> {
  const msg = await HttpUtil.get<UsageLogDay>(
    `/panel/api/clients/usageLog/${encodeURIComponent(email)}`,
    { day, filter },
    { silent: true },
  );
  if (!msg?.success || !msg.obj) throw new Error(msg?.msg || 'usage log request failed');
  return msg.obj;
}

function usageColumns(t: TFunction, calendar: CalendarKind): ColumnsType<UsageLogEntry> {
  return [
    {
      title: t('pages.clients.usageLogTime'),
      dataIndex: 'time',
      width: 190,
      render: (ms: number) => IntlUtil.formatDate(ms, calendar),
    },
    {
      title: t('pages.clients.usageLogDestination'),
      dataIndex: 'destination',
      render: (dest: string) => <Typography.Text code>{dest}</Typography.Text>,
    },
    {
      title: t('pages.clients.usageLogRoute'),
      dataIndex: 'outbound',
      width: 170,
      render: (outbound: string, entry) =>
        entry.blocked ? <Tag color="red">{t('pages.clients.usageLogBlocked')}</Tag> : outbound,
    },
  ];
}

interface ToolbarProps {
  day: string;
  onDay: (day: string) => void;
  onFilter: (filter: string) => void;
  canDownload: boolean;
  onDownload: () => void;
}

// A link, not a blob: the server streams the uncapped day, and a JSON error is saved as a
// file instead of navigating the panel away.
function downloadUsage(email: string, day: string, filter: string) {
  const link = document.createElement('a');
  const query = new URLSearchParams({ day, filter });
  link.href = withBasePath(
    `/panel/api/clients/usageLog/${encodeURIComponent(email)}/download?${query}`,
  );
  link.download = '';
  link.click();
}

function UsageToolbar({ day, onDay, onFilter, canDownload, onDownload }: ToolbarProps) {
  const { t } = useTranslation();
  return (
    <Flex gap={8} wrap>
      <div style={{ width: 180 }}>
        <DateTimePicker
          aria-label={t('pages.clients.usageLogDay')}
          value={day ? dayjs(day) : null}
          onChange={(d) => onDay(d ? d.format('YYYY-MM-DD') : '')}
          showTime={false}
          format="YYYY-MM-DD"
          allowClear={false}
          maxDate={dayjs().endOf('day')}
        />
      </div>
      <Input.Search
        allowClear
        style={{ flex: 1, minWidth: 200 }}
        aria-label={t('pages.clients.usageLogSearch')}
        placeholder={t('pages.clients.usageLogSearch')}
        onSearch={(value) => onFilter(value.trim())}
      />
      <Button
        type="primary"
        icon={<DownloadOutlined aria-hidden />}
        aria-label={t('download')}
        disabled={!canDownload}
        onClick={onDownload}
      />
    </Flex>
  );
}

function useUsageDay(email: string, open: boolean) {
  const [day, setDay] = useState('');
  const [filter, setFilter] = useState('');
  const { data, error, isFetching } = useQuery({
    queryKey: keys.clients.usageLog(email, day, filter),
    queryFn: () => fetchUsage(email, day, filter),
    enabled: open && !!email,
    placeholderData: keepPreviousData,
  });
  return { day: day || data?.day || '', filter, setDay, setFilter, data, error, isFetching };
}

export default function ClientUsageModal({ open, email, onOpenChange }: ClientUsageModalProps) {
  const { t } = useTranslation();
  const { datepicker } = useDatepicker();
  const usage = useUsageDay(email, open);
  const { data, error, isFetching } = usage;
  const canDownload = !!data?.entries.length && !isFetching && !error;

  return (
    <Modal
      open={open}
      title={`${t('pages.clients.usageLog')} — ${email}`}
      footer={null}
      width={820}
      onCancel={() => onOpenChange(false)}
    >
      <Flex vertical gap={12}>
        <Typography.Text type="secondary">
          {t('pages.clients.usageLogHint', { days: data?.retentionDays ?? 30 })}
        </Typography.Text>
        {data && !data.enabled && (
          <Alert
            type="warning"
            showIcon
            title={t(
              data.clientEnabled === false
                ? 'pages.clients.usageLogClientDisabled'
                : 'pages.clients.usageLogDisabled',
            )}
          />
        )}
        {error && <Alert type="error" showIcon title={(error as Error).message} />}
        <UsageToolbar
          day={usage.day}
          onDay={usage.setDay}
          onFilter={usage.setFilter}
          canDownload={canDownload}
          onDownload={() => downloadUsage(email, usage.day, usage.filter)}
        />
        {data?.truncated && (
          <Alert
            type="info"
            showIcon
            title={t('pages.clients.usageLogTruncated', { n: data.entries.length })}
          />
        )}
        <Table<UsageLogEntry>
          size="small"
          rowKey={(entry) => `${entry.time}|${entry.destination}`}
          loading={isFetching}
          columns={usageColumns(t, datepicker)}
          dataSource={data?.entries ?? []}
          pagination={{ pageSize: 50, showSizeChanger: false }}
          scroll={{ y: 420 }}
        />
      </Flex>
    </Modal>
  );
}
