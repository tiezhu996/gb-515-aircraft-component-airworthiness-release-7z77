import { useEffect, useMemo, useState } from 'react';
import type { DomainRecord, PartAssemblyView } from '../../types/domain';
import { getPartAssembly } from '../../api/part-assembly';
import { listAircraftPart } from '../../api/aircraft-part';
import { request } from '../../api/client';
import { useAuth } from '../../hooks/useAuth';
import { PartStatusBadge } from './PartStatusBadge';
import { UiButton } from './UiButton';

// AuthorizationGatePanel 显示放行授权关联组件的装配核对结果。复核员批准前，
// 下层任何暂停/退役/未放行部件都会把授权留在待复核并列出卡住编号。
export function AuthorizationGatePanel({ authorization, onChanged }: {
  authorization: DomainRecord;
  onChanged?: () => void;
}) {
  const { hasRole } = useAuth();
  const canOperate = hasRole('operator');
  const [parts, setParts] = useState<DomainRecord[]>([]);
  const [view, setView] = useState<PartAssemblyView | null>(null);
  const [partId, setPartId] = useState<number | ''>(authorization.aircraftPartId ?? '');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');

  const linkedPart = useMemo(
    () => parts.find((part) => part.id === authorization.aircraftPartId) || null,
    [parts, authorization.aircraftPartId],
  );

  const load = async () => {
    setError('');
    try {
      const partsPage = await listAircraftPart(1, 100);
      setParts(partsPage.data);
      if (authorization.aircraftPartId) {
        const assembly = await getPartAssembly(authorization.aircraftPartId);
        setView(assembly.data);
      } else {
        setView(null);
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    }
  };

  useEffect(() => {
    setPartId(authorization.aircraftPartId ?? '');
    void load();
    /* eslint-disable-next-line react-hooks/exhaustive-deps */
  }, [authorization.id, authorization.aircraftPartId]);

  const isDraft = authorization.status === 'draft';

  const bindPart = async () => {
    if (!partId) return;
    setSaving(true); setError(''); setMessage('');
    try {
      await request(`/authorizations/${authorization.id}`, {
        method: 'PUT',
        body: JSON.stringify({
          expectedVersion: authorization.version,
          name: authorization.name, description: authorization.description,
          facility: authorization.facility, owner: authorization.owner,
          category: authorization.category, riskLevel: authorization.riskLevel,
          metricValue: authorization.metricValue, metricUnit: authorization.metricUnit,
          effectiveAt: authorization.effectiveAt, evidence: authorization.evidence,
          relatedCode: authorization.relatedCode, aircraftPartId: Number(partId),
        }),
      });
      setMessage('已关联组件，提交复核后将逐级核对子件');
      onChanged?.();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally {
      setSaving(false);
    }
  };

  const unbindPart = async () => {
    if (!isDraft) return;
    setSaving(true); setError(''); setMessage('');
    try {
      await request(`/authorizations/${authorization.id}`, {
        method: 'PUT',
        body: JSON.stringify({
          expectedVersion: authorization.version,
          name: authorization.name, description: authorization.description,
          facility: authorization.facility, owner: authorization.owner,
          category: authorization.category, riskLevel: authorization.riskLevel,
          metricValue: authorization.metricValue, metricUnit: authorization.metricUnit,
          effectiveAt: authorization.effectiveAt, evidence: authorization.evidence,
          relatedCode: authorization.relatedCode, aircraftPartId: null,
        }),
      });
      setMessage('已解除组件关联');
      onChanged?.();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally {
      setSaving(false);
    }
  };

  return <section className="certificate-section assembly-section">
    <header>
      <h2>组件装配复核</h2>
      <span>批准放行前顺着装配关系逐级核对子件状态</span>
    </header>
    {error && <div className="alert" role="alert">{error}</div>}
    {message && <div className="assembly-ok" role="status">{message}</div>}

    {isDraft && canOperate && <div className="assembly-register">
      <label htmlFor={`auth-part-${authorization.id}`}>关联组件</label>
      <select id={`auth-part-${authorization.id}`} value={partId} onChange={(event) => setPartId(event.target.value ? Number(event.target.value) : '')}>
        <option value="">不关联组件</option>
        {[...parts].sort((a, b) => a.code.localeCompare(b.code)).map((part) =>
          <option key={part.id} value={part.id}>{part.code} · {part.name}（{part.status}）</option>)}
      </select>
      <UiButton onClick={() => void bindPart()} disabled={!partId || saving}>保存关联</UiButton>
      {authorization.aircraftPartId && <button className="link-button" disabled={saving} onClick={() => void unbindPart()}>解除关联</button>}
    </div>}

    {!authorization.aircraftPartId && <p className="muted">该授权未关联组件，批准时不做装配逐级核对。可在草稿阶段关联组件。</p>}

    {authorization.aircraftPartId && view && <>
      <p className="assembly-parent">
        关联组件：<strong>{view.part.code}</strong> {view.part.name}
        <PartStatusBadge status={view.part.status} />
      </p>
      <p className="muted">直接子件 {view.children.length} 个；逐级核对下级共 {view.check.checked} 个。</p>
      <div className={`assembly-check ${view.check.ready ? 'is-ready' : 'is-blocked'}`}>
        {view.check.ready
          ? <><strong>核对通过：</strong>全部 {view.check.checked} 个下级部件均已放行，复核员可以批准放行。</>
          : <>
            <strong>核对未通过（{view.check.blocked.length} 个卡住）：</strong>
            批准时授权将留在待复核，卡住的子件编号如下。
            <ul className="blocked-list">
              {view.check.blocked.map((blocked) => <li key={`${blocked.level}-${blocked.id}`}>
                <strong>{blocked.code}</strong> · {blocked.name} · L{blocked.level}
                <PartStatusBadge status={blocked.status} />
                <em>{blocked.reason}</em>
              </li>)}
            </ul>
          </>}
      </div>
      {linkedPart && <p className="muted">提示：先在部件页处理卡住的子件（解除暂停、退役件更换或完成放行），核对通过后再批准。</p>}
    </>}
  </section>;
}
