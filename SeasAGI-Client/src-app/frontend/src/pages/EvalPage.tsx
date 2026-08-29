import { useState, useEffect } from 'react';
import { useTranslation } from '../i18n';
import { listEvalSuites, createEvalSuite, runEvalSuite, getEvalScorecard } from '../utils/commands';

interface EvalSuite {
  id: string;
  name: string;
  description: string;
  target_type: string;
  target_ref: string;
  cases_count: number;
  created_at: string;
}

interface EvalCaseInput {
  id: string;
  name: string;
  prompt: string;
  expected: string;
  match: string;
}

interface RunResult {
  id: string;
  total: number;
  passed: number;
  failed: number;
  errors: number;
  pass_rate: number;
  avg_latency: number;
}

export function EvalPage() {
  const { t } = useTranslation();
  const [suites, setSuites] = useState<EvalSuite[]>([]);
  const [loading, setLoading] = useState(false);
  const [runResult, setRunResult] = useState<RunResult | null>(null);
  const [scorecard, setScorecard] = useState<Record<string, any> | null>(null);
  const [showCreate, setShowCreate] = useState(false);

  // 创建表单
  const [newSuite, setNewSuite] = useState({
    name: '',
    description: '',
    target_type: 'suite-default',
    target_ref: '',
  });
  const [cases, setCases] = useState<EvalCaseInput[]>([
    { id: 'c1', name: '', prompt: '', expected: '', match: 'contains' },
  ]);

  const loadSuites = async () => {
    setLoading(true);
    try {
      const res = await listEvalSuites();
      setSuites(res as EvalSuite[]);
    } catch (e) {
      // ignore
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadSuites();
  }, []);

  const handleCreate = async () => {
    try {
      await createEvalSuite({
        ...newSuite,
        cases: cases.map(c => ({
          id: c.id,
          name: c.name,
          prompt: c.prompt,
          expected: c.expected,
          match: c.match,
        })),
      });
      setShowCreate(false);
      loadSuites();
    } catch (e: any) {
      alert(e?.message || String(e));
    }
  };

  const handleRun = async (suiteID: string) => {
    try {
      const result = await runEvalSuite(suiteID);
      setRunResult(result as RunResult);
      const sc = await getEvalScorecard(suiteID);
      setScorecard(sc);
    } catch (e: any) {
      alert(e?.message || String(e));
    }
  };

  const addCase = () => {
    setCases([...cases, { id: `c${cases.length + 1}`, name: '', prompt: '', expected: '', match: 'contains' }]);
  };

  const removeCase = (idx: number) => {
    setCases(cases.filter((_, i) => i !== idx));
  };

  const updateCase = (idx: number, field: keyof EvalCaseInput, value: string) => {
    const updated = [...cases];
    updated[idx] = { ...updated[idx], [field]: value };
    setCases(updated);
  };

  return (
    <div className="eval-page">
      <div className="page-header">
        <h2>{t('eval.title')}</h2>
        <p className="page-subtitle">{t('eval.subtitle')}</p>
        <button className="btn btn-primary" onClick={() => setShowCreate(!showCreate)}>
          {t('eval.createSuite')}
        </button>
      </div>

      {/* 创建 Suite 表单 */}
      {showCreate && (
        <div className="eval-create-form">
          <h3>{t('eval.newSuite')}</h3>
          <div className="form-row">
            <label>{t('eval.suiteName')}</label>
            <input type="text" value={newSuite.name} onChange={e => setNewSuite({ ...newSuite, name: e.target.value })} />
          </div>
          <div className="form-row">
            <label>{t('eval.description')}</label>
            <input type="text" value={newSuite.description} onChange={e => setNewSuite({ ...newSuite, description: e.target.value })} />
          </div>
          <div className="form-row">
            <label>{t('eval.targetType')}</label>
            <select value={newSuite.target_type} onChange={e => setNewSuite({ ...newSuite, target_type: e.target.value })}>
              <option value="suite-default">{t('eval.targetDefault')}</option>
              <option value="model">{t('eval.targetModel')}</option>
              <option value="combo">{t('eval.targetCombo')}</option>
            </select>
          </div>
          {newSuite.target_type !== 'suite-default' && (
            <div className="form-row">
              <label>{t('eval.targetRef')}</label>
              <input type="text" value={newSuite.target_ref} onChange={e => setNewSuite({ ...newSuite, target_ref: e.target.value })} />
            </div>
          )}

          <h4>{t('eval.cases')}</h4>
          {cases.map((c, idx) => (
            <div key={idx} className="eval-case-row">
              <input type="text" placeholder={t('eval.caseName')} value={c.name} onChange={e => updateCase(idx, 'name', e.target.value)} />
              <input type="text" placeholder={t('eval.prompt')} value={c.prompt} onChange={e => updateCase(idx, 'prompt', e.target.value)} />
              <input type="text" placeholder={t('eval.expected')} value={c.expected} onChange={e => updateCase(idx, 'expected', e.target.value)} />
              <select value={c.match} onChange={e => updateCase(idx, 'match', e.target.value)}>
                <option value="contains">{t('eval.matchContains')}</option>
                <option value="exact">{t('eval.matchExact')}</option>
                <option value="regex">{t('eval.matchRegex')}</option>
                <option value="custom">{t('eval.matchCustom')}</option>
              </select>
              <button className="btn btn-sm" onClick={() => removeCase(idx)}>-</button>
            </div>
          ))}
          <button className="btn btn-sm" onClick={addCase}>{t('eval.addCase')}</button>
          <div className="form-actions">
            <button className="btn btn-primary" onClick={handleCreate}>{t('eval.save')}</button>
            <button className="btn" onClick={() => setShowCreate(false)}>{t('eval.cancel')}</button>
          </div>
        </div>
      )}

      {/* Suite 列表 */}
      <div className="eval-suites-list">
        {loading && <p>{t('eval.loading')}</p>}
        {!loading && suites.length === 0 && <p className="diag-empty">{t('eval.noSuites')}</p>}
        {suites.map(s => (
          <div key={s.id} className="eval-suite-card">
            <div className="eval-suite-header">
              <h4>{s.name}</h4>
              <span className="badge-blue">{t(`eval.target${s.target_type === 'suite-default' ? 'Default' : s.target_type === 'model' ? 'Model' : 'Combo'}`)}</span>
            </div>
            {s.description && <p className="eval-suite-desc">{s.description}</p>}
            <div className="eval-suite-meta">
              <span>{t('eval.casesCount')}: {s.cases_count}</span>
              {s.target_ref && <span>{t('eval.target')}: {s.target_ref}</span>}
              <span className="diag-timestamp">{s.created_at}</span>
            </div>
            <button className="btn btn-primary btn-sm" onClick={() => handleRun(s.id)}>
              {t('eval.runSuite')}
            </button>
          </div>
        ))}
      </div>

      {/* 运行结果 */}
      {runResult && (
        <div className="eval-run-result">
          <h3>{t('eval.runResult')}</h3>
          <div className="diag-grid">
            <div className="diag-card">
              <span className="diag-label">{t('eval.total')}</span>
              <span className="diag-value">{runResult.total}</span>
            </div>
            <div className="diag-card">
              <span className="diag-label">{t('eval.passed')}</span>
              <span className="badge-green">{runResult.passed}</span>
            </div>
            <div className="diag-card">
              <span className="diag-label">{t('eval.failed')}</span>
              <span className="badge-red">{runResult.failed}</span>
            </div>
            <div className="diag-card">
              <span className="diag-label">{t('eval.errors')}</span>
              <span className="badge-red">{runResult.errors}</span>
            </div>
            <div className="diag-card">
              <span className="diag-label">{t('eval.passRate')}</span>
              <span className="diag-value">{runResult.pass_rate.toFixed(1)}%</span>
            </div>
            <div className="diag-card">
              <span className="diag-label">{t('eval.avgLatency')}</span>
              <span className="diag-value">{runResult.avg_latency}ms</span>
            </div>
          </div>
        </div>
      )}

      {/* 记分卡 */}
      {scorecard && !scorecard.error && (
        <div className="eval-scorecard">
          <h3>{t('eval.scorecard')}</h3>
          <div className="diag-grid">
            <div className="diag-card">
              <span className="diag-label">{t('eval.passRate')}</span>
              <span className="diag-value">{(scorecard.pass_rate as number).toFixed(1)}%</span>
            </div>
            <div className="diag-card">
              <span className="diag-label">{t('eval.total')}</span>
              <span className="diag-value">{scorecard.total}</span>
            </div>
            <div className="diag-card">
              <span className="diag-label">{t('eval.passed')}</span>
              <span className="badge-green">{scorecard.passed}</span>
            </div>
            <div className="diag-card">
              <span className="diag-label">{t('eval.failed')}</span>
              <span className="badge-red">{scorecard.failed}</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
