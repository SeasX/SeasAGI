import { useState, useEffect } from 'react';
import { useTranslation } from '../i18n';
import { getMITMTargets, searchNotion, searchObsidian } from '../utils/commands';

interface MITMTarget {
  id: string;
  name: string;
  icon: string;
  color: string;
  hosts: string[];
  port: number;
  endpoint_patterns: string[];
  default_models: { id: string; name: string }[];
  viability: string;
}

export function KnowledgeBasePage() {
  const { t } = useTranslation();
  const [targets, setTargets] = useState<MITMTarget[]>([]);
  const [loadingTargets, setLoadingTargets] = useState(true);
  const [targetsError, setTargetsError] = useState('');

  // Notion
  const [notionKey, setNotionKey] = useState('');
  const [notionQuery, setNotionQuery] = useState('');
  const [notionResults, setNotionResults] = useState<Record<string, any> | null>(null);
  const [notionSearching, setNotionSearching] = useState(false);
  const [notionError, setNotionError] = useState('');

  // Obsidian
  const [obsidianKey, setObsidianKey] = useState('');
  const [obsidianBaseURL, setObsidianBaseURL] = useState('');
  const [obsidianQuery, setObsidianQuery] = useState('');
  const [obsidianResults, setObsidianResults] = useState<Record<string, any>[]>([]);
  const [obsidianSearching, setObsidianSearching] = useState(false);
  const [obsidianError, setObsidianError] = useState('');

  const loadTargets = async () => {
    setLoadingTargets(true);
    setTargetsError('');
    try {
      const res = await getMITMTargets();
      setTargets(res as MITMTarget[]);
    } catch (e: any) {
      setTargetsError(e?.message || String(e));
    } finally {
      setLoadingTargets(false);
    }
  };

  useEffect(() => {
    loadTargets();
  }, []);

  const handleNotionSearch = async () => {
    if (!notionKey || !notionQuery) return;
    setNotionSearching(true);
    setNotionError('');
    setNotionResults(null);
    try {
      const res = await searchNotion(notionKey, notionQuery);
      setNotionResults(res);
    } catch (e: any) {
      setNotionError(e?.message || String(e));
    } finally {
      setNotionSearching(false);
    }
  };

  const handleObsidianSearch = async () => {
    if (!obsidianKey || !obsidianBaseURL || !obsidianQuery) return;
    setObsidianSearching(true);
    setObsidianError('');
    setObsidianResults([]);
    try {
      const res = await searchObsidian(obsidianKey, obsidianBaseURL, obsidianQuery);
      setObsidianResults(res || []);
    } catch (e: any) {
      setObsidianError(e?.message || String(e));
    } finally {
      setObsidianSearching(false);
    }
  };

  const renderViabilityBadge = (viability: string) => {
    const lower = (viability || '').toLowerCase();
    if (lower === 'high' || lower === 'good' || lower === 'stable') {
      return <span className="badge-green">{viability}</span>;
    }
    if (lower === 'low' || lower === 'poor' || lower === 'unstable') {
      return <span className="badge-red">{viability}</span>;
    }
    return <span className="badge-blue">{viability}</span>;
  };

  const notionResultItems: any[] = (() => {
    if (!notionResults) return [];
    if (Array.isArray(notionResults)) return notionResults;
    if (Array.isArray(notionResults.results)) return notionResults.results;
    if (Array.isArray(notionResults.data)) return notionResults.data;
    return [];
  })();

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>{t('kb.title')}</h1>
        <p className="page-subtitle">{t('kb.subtitle')}</p>
      </div>

      {/* Section 1: MITM Target 预设 */}
      <section className="diag-section">
        <h2>{t('kb.mitmTargets')}</h2>
        {targetsError && <div className="alert alert-error">{targetsError}</div>}
        {loadingTargets && <p className="diag-empty">{t('kb.loading')}</p>}
        {!loadingTargets && targets.length > 0 ? (
          <div className="diag-grid">
            {targets.map((target) => (
              <div key={target.id} className="diag-card">
                <div className="diag-label" style={{ color: target.color || undefined }}>
                  {target.icon} {target.name}
                </div>
                <span className="diag-value">{target.id}</span>
                {target.hosts && target.hosts.length > 0 && (
                  <div className="diag-timestamp">
                    {t('kb.hosts')}: {target.hosts.join(', ')}
                  </div>
                )}
                <div className="form-actions">
                  {renderViabilityBadge(target.viability)}
                </div>
              </div>
            ))}
          </div>
        ) : (
          !loadingTargets && <p className="diag-empty">{t('kb.noTargets')}</p>
        )}
      </section>

      {/* Section 2: Notion 集成 */}
      <section className="diag-section">
        <h2>{t('kb.notion')}</h2>
        <div className="eval-create-form">
          <div className="form-row">
            <label>{t('kb.notionApiKey')}</label>
            <input
              type="password"
              value={notionKey}
              onChange={(e) => setNotionKey(e.target.value)}
              placeholder={t('kb.notionApiKeyPlaceholder')}
            />
          </div>
          <div className="form-row">
            <label>{t('kb.notionQuery')}</label>
            <input
              type="text"
              value={notionQuery}
              onChange={(e) => setNotionQuery(e.target.value)}
              placeholder={t('kb.notionQueryPlaceholder')}
            />
          </div>
          <div className="form-actions">
            <button
              className="btn btn-primary"
              onClick={handleNotionSearch}
              disabled={notionSearching || !notionKey || !notionQuery}
            >
              {notionSearching ? t('kb.searching') : t('kb.search')}
            </button>
          </div>
        </div>

        {notionError && <div className="alert alert-error">{notionError}</div>}

        {notionResults && (
          <div className="diag-section">
            <h3>{t('kb.notionResults')}</h3>
            {notionResultItems.length > 0 ? (
              <div className="diag-grid">
                {notionResultItems.map((item, idx) => (
                  <div key={item.id || idx} className="diag-card">
                    <span className="diag-label">
                      {item.title || item.name || item.properties?.title?.title?.[0]?.plain_text || '-'}
                    </span>
                    <span className="diag-value">{item.url || item.uri || '-'}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="diag-empty">{t('kb.notionNoResults')}</p>
            )}
          </div>
        )}
      </section>

      {/* Section 3: Obsidian 集成 */}
      <section className="diag-section">
        <h2>{t('kb.obsidian')}</h2>
        <div className="eval-create-form">
          <div className="form-row">
            <label>{t('kb.obsidianApiKey')}</label>
            <input
              type="password"
              value={obsidianKey}
              onChange={(e) => setObsidianKey(e.target.value)}
              placeholder={t('kb.obsidianApiKeyPlaceholder')}
            />
          </div>
          <div className="form-row">
            <label>{t('kb.obsidianBaseUrl')}</label>
            <input
              type="text"
              value={obsidianBaseURL}
              onChange={(e) => setObsidianBaseURL(e.target.value)}
              placeholder={t('kb.obsidianBaseUrlPlaceholder')}
            />
          </div>
          <div className="form-row">
            <label>{t('kb.obsidianQuery')}</label>
            <input
              type="text"
              value={obsidianQuery}
              onChange={(e) => setObsidianQuery(e.target.value)}
              placeholder={t('kb.obsidianQueryPlaceholder')}
            />
          </div>
          <div className="form-actions">
            <button
              className="btn btn-primary"
              onClick={handleObsidianSearch}
              disabled={obsidianSearching || !obsidianKey || !obsidianBaseURL || !obsidianQuery}
            >
              {obsidianSearching ? t('kb.searching') : t('kb.search')}
            </button>
          </div>
        </div>

        {obsidianError && <div className="alert alert-error">{obsidianError}</div>}

        {obsidianResults.length > 0 && (
          <div className="diag-section">
            <h3>{t('kb.obsidianResults')}</h3>
            <div className="diag-grid">
              {obsidianResults.map((item, idx) => (
                <div key={item.id || idx} className="diag-card">
                  <span className="diag-label">
                    {item.title || item.name || item.path || '-'}
                  </span>
                  <span className="diag-value">
                    {item.content || item.snippet || item.excerpt || '-'}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </section>
    </div>
  );
}
