import { useState, useEffect } from 'react';
import { useTranslation } from '../i18n';
import { listPlugins, togglePlugin, getPluginHooks, getPluginAuditLog } from '../utils/commands';

interface Plugin {
  name: string;
  priority: number;
  enabled: boolean;
}

interface PluginHook {
  event: string;
  hook_count: number;
}

export function PluginsPage() {
  const { t } = useTranslation();
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [hooks, setHooks] = useState<PluginHook[]>([]);
  const [auditLog, setAuditLog] = useState<Record<string, any>[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [toggling, setToggling] = useState<string | null>(null);

  const loadData = async () => {
    setLoading(true);
    setError('');
    try {
      const [pluginRes, hookRes, auditRes] = await Promise.all([
        listPlugins(),
        getPluginHooks(),
        getPluginAuditLog(),
      ]);
      setPlugins(pluginRes as Plugin[]);
      setHooks(hookRes as PluginHook[]);
      setAuditLog(auditRes || []);
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleToggle = async (plugin: Plugin) => {
    setToggling(plugin.name);
    try {
      await togglePlugin(plugin.name, !plugin.enabled);
      setPlugins(plugins.map((p) =>
        p.name === plugin.name ? { ...p, enabled: !p.enabled } : p
      ));
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setToggling(null);
    }
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>{t('plugin.title')}</h1>
        <p className="page-subtitle">{t('plugin.subtitle')}</p>
      </div>

      {error && <div className="alert alert-error">{error}</div>}

      {loading && <p className="diag-empty">{t('plugin.loading')}</p>}

      {!loading && (
        <>
          {/* 插件列表 */}
          <section className="diag-section">
            <h2>{t('plugin.list')}</h2>
            {plugins.length > 0 ? (
              <table className="diag-table">
                <thead>
                  <tr>
                    <th>{t('plugin.name')}</th>
                    <th>{t('plugin.priority')}</th>
                    <th>{t('plugin.enabled')}</th>
                    <th>{t('plugin.actions')}</th>
                  </tr>
                </thead>
                <tbody>
                  {plugins.map((p) => (
                    <tr key={p.name}>
                      <td>{p.name}</td>
                      <td>{p.priority}</td>
                      <td>
                        <span className={p.enabled ? 'badge-green' : 'badge-red'}>
                          {p.enabled ? t('plugin.enabled') : t('plugin.disabled')}
                        </span>
                      </td>
                      <td>
                        <button
                          className="btn btn-sm"
                          onClick={() => handleToggle(p)}
                          disabled={toggling === p.name}
                        >
                          {toggling === p.name
                            ? t('plugin.toggling')
                            : p.enabled
                            ? t('plugin.disable')
                            : t('plugin.enable')}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="diag-empty">{t('plugin.noPlugins')}</p>
            )}
          </section>

          {/* Hook 事件 */}
          <section className="diag-section">
            <h2>{t('plugin.hooks')}</h2>
            {hooks.length > 0 ? (
              <table className="diag-table">
                <thead>
                  <tr>
                    <th>{t('plugin.hookEvent')}</th>
                    <th>{t('plugin.hookCount')}</th>
                  </tr>
                </thead>
                <tbody>
                  {hooks.map((h) => (
                    <tr key={h.event}>
                      <td><span className="badge-blue">{h.event}</span></td>
                      <td>{h.hook_count}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="diag-empty">{t('plugin.noHooks')}</p>
            )}
          </section>

          {/* 审计日志 */}
          <section className="diag-section">
            <h2>{t('plugin.auditLog')}</h2>
            {auditLog.length > 0 ? (
              <div className="diag-grid">
                {auditLog.map((entry, idx) => (
                  <div key={idx} className="diag-card">
                    <span className="diag-label">{entry.plugin || entry.name || '-'}</span>
                    <span className="diag-value">{entry.action || entry.event || '-'}</span>
                    {entry.timestamp && (
                      <span className="diag-timestamp">{entry.timestamp}</span>
                    )}
                  </div>
                ))}
              </div>
            ) : (
              <p className="diag-empty">{t('plugin.noAuditLog')}</p>
            )}
          </section>
        </>
      )}
    </div>
  );
}
