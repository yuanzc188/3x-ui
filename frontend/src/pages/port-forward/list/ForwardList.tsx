import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Popconfirm, Space, Switch, Table, Tag } from 'antd';
import type { ColumnsType } from 'antd/es/table';

import { useInboundOptions } from '@/api/queries/useInboundOptions';
import type { ForwardRule } from '../types';

interface Props {
  rules: ForwardRule[];
  loading: boolean;
  onEdit: (rule: ForwardRule) => void;
  onDelete: (id: number) => void;
  onToggle: (id: number, enable: boolean) => void;
}

export default function ForwardList({ rules, loading, onEdit, onDelete, onToggle }: Props) {
  const { t } = useTranslation();
  const { data: inbounds = [] } = useInboundOptions();
  const knownTags = useMemo(() => new Set(inbounds.map((ib) => ib.tag).filter(Boolean)), [inbounds]);

  const columns: ColumnsType<ForwardRule> = [
    {
      title: t('pages.portForward.inbound'),
      dataIndex: 'inboundTag',
      render: (tag: string) =>
        knownTags.has(tag) ? (
          <span>{tag}</span>
        ) : (
          <Space>
            <span>{tag}</span>
            <Tag color="error">{t('pages.portForward.orphanInbound')}</Tag>
          </Space>
        ),
    },
    {
      title: t('pages.portForward.destType'),
      dataIndex: 'destType',
      render: (v: string) => (v === 'http' ? 'HTTP' : 'SOCKS5'),
    },
    {
      title: t('pages.portForward.destAddress'),
      render: (_: unknown, r: ForwardRule) => `${r.destAddress}:${r.destPort}`,
    },
    { title: t('pages.portForward.remark'), dataIndex: 'remark' },
    {
      title: t('pages.portForward.enable'),
      dataIndex: 'enable',
      render: (enable: boolean, r: ForwardRule) => (
        <Switch checked={enable} onChange={(v) => onToggle(r.id, v)} />
      ),
    },
    {
      title: '',
      key: 'actions',
      render: (_: unknown, r: ForwardRule) => (
        <Space>
          <Button size="small" onClick={() => onEdit(r)}>{t('edit')}</Button>
          <Popconfirm title={t('delete')} onConfirm={() => onDelete(r.id)}>
            <Button size="small" danger>{t('delete')}</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Table
      rowKey="id"
      loading={loading}
      columns={columns}
      dataSource={rules}
      pagination={false}
    />
  );
}
