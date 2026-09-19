import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Form, Input, InputNumber, Modal, Select, Switch, message } from 'antd';
import dayjs from 'dayjs';

import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { DateTimePicker } from '@/components/form';
import { inboundAlreadyBound } from '../helpers';
import type { ForwardRule } from '../types';

interface Props {
  open: boolean;
  editing: ForwardRule | null;
  rules: ForwardRule[];
  onCancel: () => void;
  onSave: (rule: Partial<ForwardRule>) => Promise<unknown>;
}

const emptyValues: Partial<ForwardRule> = {
  destType: 'socks',
  enable: true,
  destPort: 1080,
  domainLimit: false,
  expiryTime: 0,
};

export default function ForwardFormModal({ open, editing, rules, onCancel, onSave }: Props) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [form] = Form.useForm<ForwardRule>();
  const { data: inbounds = [] } = useInboundOptions();
  const domainLimit = Form.useWatch('domainLimit', form);
  const expiryTime = Form.useWatch('expiryTime', form);

  useEffect(() => {
    if (!open) return;
    form.setFieldsValue(editing ?? emptyValues);
  }, [open, editing, form]);

  const inboundOptions = useMemo(
    () =>
      inbounds
        .filter((ib) => !!ib.tag)
        .map((ib) => ({
          value: ib.tag as string,
          label: `${ib.port ?? ''} · ${ib.tag}${ib.remark ? ` (${ib.remark})` : ''}`,
        })),
    [inbounds],
  );

  const handleOk = async () => {
    const values = await form.validateFields();
    const editingId = editing?.id ?? 0;
    if (inboundAlreadyBound(rules, values.inboundTag, editingId)) {
      messageApi.error(t('pages.portForward.duplicateInbound'));
      return;
    }
    const res = (await onSave({ ...editing, ...values })) as { success?: boolean } | undefined;
    if (res?.success) {
      onCancel();
    }
  };

  return (
    <>
      {messageContextHolder}
      <Modal
        open={open}
        title={editing ? t('edit') : t('pages.portForward.addRule')}
        onOk={handleOk}
        onCancel={onCancel}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" initialValues={emptyValues}>
          <Form.Item
            name="inboundTag"
            label={t('pages.portForward.inbound')}
            rules={[{ required: true }]}
          >
            <Select options={inboundOptions} showSearch optionFilterProp="label" />
          </Form.Item>
          <Form.Item
            name="destType"
            label={t('pages.portForward.destType')}
            rules={[{ required: true }]}
          >
            <Select
              options={[
                { value: 'socks', label: 'SOCKS5' },
                { value: 'http', label: 'HTTP' },
              ]}
            />
          </Form.Item>
          <Form.Item
            name="destAddress"
            label={t('pages.portForward.destAddress')}
            rules={[{ required: true }]}
          >
            <Input placeholder="1.2.3.4" />
          </Form.Item>
          <Form.Item
            name="destPort"
            label={t('pages.portForward.destPort')}
            rules={[{ required: true }]}
          >
            <InputNumber min={1} max={65535} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="username" label={t('pages.portForward.username')}>
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item name="password" label={t('pages.portForward.password')}>
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item
            name="remark"
            label={t('pages.portForward.remark')}
            extra={t('pages.portForward.remarkHint')}
          >
            <Input />
          </Form.Item>

          <Form.Item
            name="domainLimit"
            label={t('pages.portForward.domainLimit')}
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
          {domainLimit && (
            <Form.Item
              name="domains"
              label={t('pages.portForward.domains')}
              extra={t('pages.portForward.domainsHint')}
            >
              <Input.TextArea rows={5} placeholder={'domain:tiktok.com\ngeosite:tiktok'} />
            </Form.Item>
          )}

          <Form.Item name="expiryTime" hidden>
            <InputNumber />
          </Form.Item>
          <Form.Item
            label={t('pages.portForward.expiry')}
            extra={t('pages.portForward.expiryHint')}
          >
            <DateTimePicker
              value={expiryTime && expiryTime > 0 ? dayjs(expiryTime) : null}
              onChange={(d) => form.setFieldValue('expiryTime', d ? d.valueOf() : 0)}
              showTime={false}
              format="YYYY-MM-DD"
            />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
}
