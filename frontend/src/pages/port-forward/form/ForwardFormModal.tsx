import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Form, Input, InputNumber, Modal, Select, message } from 'antd';

import { useInboundOptions } from '@/api/queries/useInboundOptions';
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
  destType: 'socks', enable: true, destPort: 1080,
};

export default function ForwardFormModal({ open, editing, rules, onCancel, onSave }: Props) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [form] = Form.useForm<ForwardRule>();
  const { data: inbounds = [] } = useInboundOptions();

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
          <Form.Item name="inboundTag" label={t('pages.portForward.inbound')} rules={[{ required: true }]}>
            <Select options={inboundOptions} showSearch optionFilterProp="label" />
          </Form.Item>
          <Form.Item name="destType" label={t('pages.portForward.destType')} rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'socks', label: 'SOCKS5' },
                { value: 'http', label: 'HTTP' },
              ]}
            />
          </Form.Item>
          <Form.Item name="destAddress" label={t('pages.portForward.destAddress')} rules={[{ required: true }]}>
            <Input placeholder="1.2.3.4" />
          </Form.Item>
          <Form.Item name="destPort" label={t('pages.portForward.destPort')} rules={[{ required: true }]}>
            <InputNumber min={1} max={65535} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="username" label={t('pages.portForward.username')}>
            <Input autoComplete="off" />
          </Form.Item>
          <Form.Item name="password" label={t('pages.portForward.password')}>
            <Input.Password autoComplete="new-password" />
          </Form.Item>
          <Form.Item name="remark" label={t('pages.portForward.remark')}>
            <Input />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
}
