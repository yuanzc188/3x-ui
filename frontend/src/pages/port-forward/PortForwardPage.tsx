import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, ConfigProvider, Layout, Space } from 'antd';
import { PlusOutlined, SettingOutlined } from '@ant-design/icons';

import { useTheme } from '@/hooks/useTheme';
import AppSidebar from '@/layouts/AppSidebar';
import { usePortForward } from './usePortForward';
import ForwardList from './list/ForwardList';
import ForwardFormModal from './form/ForwardFormModal';
import ForwardSettingsModal from './form/ForwardSettingsModal';
import type { ForwardRule } from './types';

export default function PortForwardPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { rules, loading, settings, save, remove, setEnable, checkNow, saveSettings } =
    usePortForward();
  const [modalOpen, setModalOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [editing, setEditing] = useState<ForwardRule | null>(null);

  const pageClass = useMemo(() => {
    const classes = ['port-forward-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const openAdd = () => {
    setEditing(null);
    setModalOpen(true);
  };
  const openEdit = (rule: ForwardRule) => {
    setEditing(rule);
    setModalOpen(true);
  };

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <Card
              title={t('pages.portForward.title')}
              extra={
                <Space>
                  <Button icon={<SettingOutlined />} onClick={() => setSettingsOpen(true)}>
                    {t('pages.portForward.settings')}
                  </Button>
                  <Button type="primary" icon={<PlusOutlined />} onClick={openAdd}>
                    {t('pages.portForward.addRule')}
                  </Button>
                </Space>
              }
            >
              <ForwardList
                rules={rules}
                loading={loading}
                onEdit={openEdit}
                onDelete={(id) => remove(id)}
                onToggle={(id, enable) => setEnable(id, enable)}
                onCheck={checkNow}
              />
              <ForwardFormModal
                open={modalOpen}
                editing={editing}
                rules={rules}
                onCancel={() => setModalOpen(false)}
                onSave={save}
              />
              <ForwardSettingsModal
                open={settingsOpen}
                settings={settings}
                onCancel={() => setSettingsOpen(false)}
                onSave={saveSettings}
              />
            </Card>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
