import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Form, Input, Modal } from 'antd';

import type { ForwardSettings } from '../types';

interface Props {
  open: boolean;
  settings: ForwardSettings | null;
  onCancel: () => void;
  onSave: (s: ForwardSettings) => Promise<unknown>;
}

export default function ForwardSettingsModal({ open, settings, onCancel, onSave }: Props) {
  const { t } = useTranslation();
  const [form] = Form.useForm<ForwardSettings>();

  useEffect(() => {
    if (open && settings) form.setFieldsValue(settings);
  }, [open, settings, form]);

  const handleOk = async () => {
    const values = await form.validateFields();
    const res = (await onSave(values)) as { success?: boolean } | undefined;
    if (res?.success) onCancel();
  };

  return (
    <Modal
      open={open}
      title={t('pages.portForward.settings')}
      onOk={handleOk}
      onCancel={onCancel}
      destroyOnHidden
    >
      <Form form={form} layout="vertical">
        <Form.Item
          name="globalDomains"
          label={t('pages.portForward.globalDomains')}
          extra={t('pages.portForward.globalDomainsHint')}
        >
          <Input.TextArea
            rows={8}
            placeholder={'domain:tiktok.com\ndomain:tiktokv.com\ngeosite:tiktok'}
          />
        </Form.Item>
        <Form.Item
          name="checkUrl"
          label={t('pages.portForward.checkUrl')}
          extra={t('pages.portForward.checkUrlHint')}
        >
          <Input placeholder="http://ip-api.com/json/?fields=query,country,city" />
        </Form.Item>
      </Form>
    </Modal>
  );
}
