import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge, Button, Popconfirm, Space, Switch, Table, Tag, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined, WarningOutlined } from '@ant-design/icons';

import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { useDatepicker } from '@/hooks/useDatepicker';
import { IntlUtil } from '@/utils';
import { expiryLevel, whitelistMode, type ExpiryLevel } from '../helpers';
import type { ForwardRule } from '../types';

interface Props {
  rules: ForwardRule[];
  loading: boolean;
  onEdit: (rule: ForwardRule) => void;
  onDelete: (id: number) => void;
  onToggle: (id: number, enable: boolean) => void;
  onCheck: (id: number) => Promise<unknown>;
}

const EXPIRY_COLOR: Record<ExpiryLevel, string> = {
  none: 'default',
  ok: 'green',
  soon: 'orange',
  expired: 'red',
};

export default function ForwardList({
  rules,
  loading,
  onEdit,
  onDelete,
  onToggle,
  onCheck,
}: Props) {
  const { t } = useTranslation();
  const { datepicker } = useDatepicker();
  const { data: inbounds = [] } = useInboundOptions();
  const knownTags = useMemo(
    () => new Set(inbounds.map((ib) => ib.tag).filter(Boolean)),
    [inbounds],
  );
  const [checkingId, setCheckingId] = useState<number | null>(null);

  const runCheck = async (id: number) => {
    setCheckingId(id);
    try {
      await onCheck(id);
    } finally {
      setCheckingId(null);
    }
  };

  const renderHealth = (r: ForwardRule) => {
    if (!r.checkedAt) {
      return <Badge status="default" text={t('pages.portForward.notChecked')} />;
    }
    const when = `${t('pages.portForward.checkedAt')}: ${IntlUtil.formatDate(r.checkedAt, datepicker)}`;
    if (r.checkOk) {
      const parts = [r.checkIp, r.checkGeo, `${r.checkMs}ms`].filter(Boolean).join(' · ');
      return (
        <Tooltip title={when}>
          <Badge status="success" text={parts} />
        </Tooltip>
      );
    }
    return (
      <Tooltip
        title={
          <div>
            <div>{r.checkErr}</div>
            <div>{when}</div>
          </div>
        }
      >
        <Badge status="error" text={t('pages.portForward.checkFailed')} />
      </Tooltip>
    );
  };

  const columns: ColumnsType<ForwardRule> = [
    {
      title: t('pages.portForward.inbound'),
      dataIndex: 'inboundTag',
      render: (tag: string, r: ForwardRule) => (
        <Space size={4} wrap>
          <span>{tag}</span>
          {!knownTags.has(tag) && <Tag color="error">{t('pages.portForward.orphanInbound')}</Tag>}
          {r.sniffingOff && (
            <Tooltip title={t('pages.portForward.sniffingOffHint')}>
              <Tag color="warning" icon={<WarningOutlined />}>
                {t('pages.portForward.sniffingOff')}
              </Tag>
            </Tooltip>
          )}
        </Space>
      ),
    },
    {
      title: t('pages.portForward.destAddress'),
      render: (_: unknown, r: ForwardRule) =>
        `${r.destType === 'http' ? 'HTTP' : 'SOCKS5'} ${r.destAddress}:${r.destPort}`,
    },
    { title: t('pages.portForward.remark'), dataIndex: 'remark' },
    {
      title: t('pages.portForward.whitelist'),
      render: (_: unknown, r: ForwardRule) => {
        const mode = whitelistMode(r);
        const color = mode === 'off' ? 'default' : mode === 'global' ? 'blue' : 'purple';
        return <Tag color={color}>{t(`pages.portForward.whitelist_${mode}`)}</Tag>;
      },
    },
    {
      title: t('pages.portForward.health'),
      render: (_: unknown, r: ForwardRule) => renderHealth(r),
    },
    {
      title: t('pages.portForward.expiry'),
      dataIndex: 'expiryTime',
      render: (v: number) => {
        const level = expiryLevel(v, Date.now());
        return (
          <Tag color={EXPIRY_COLOR[level]}>
            {level === 'none' ? '—' : IntlUtil.formatDate(v, datepicker)}
          </Tag>
        );
      },
    },
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
          <Tooltip title={t('pages.portForward.checkNow')}>
            <Button
              size="small"
              icon={<ReloadOutlined />}
              loading={checkingId === r.id}
              onClick={() => runCheck(r.id)}
            />
          </Tooltip>
          <Button size="small" onClick={() => onEdit(r)}>
            {t('edit')}
          </Button>
          <Popconfirm title={t('delete')} onConfirm={() => onDelete(r.id)}>
            <Button size="small" danger>
              {t('delete')}
            </Button>
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
      scroll={{ x: true }}
    />
  );
}
