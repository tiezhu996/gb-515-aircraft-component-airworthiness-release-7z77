import { useEffect, useMemo, useState } from 'react';
import type { DomainRecord, PartAssemblyView } from '../../types/domain';
import { getPartAssembly, registerPartAssembly, removePartAssembly } from '../../api/part-assembly';
import { listAircraftPart } from '../../api/aircraft-part';
import { useAuth } from '../../hooks/useAuth';
import { PartStatusBadge } from './PartStatusBadge';
import { UiButton } from './UiButton';

// AssemblyPanel 渲染部件页的“装配清单”和逐级“核对结果”，并提供登记/拆除
// 装配关系的入口。同一子件只能挂在一个组件下，绕回自己的关系由后端拦截。
export function AssemblyPanel({ part }: { part: DomainRecord }) {
  const { hasRole } = useAuth();
  const canOperate = hasRole('operator');
  const [view, setView] = useState<PartAssemblyView | null>(null);
  const [allParts, setAllParts] = useState<DomainRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const [childId, setChildId] = useState<number | ''>('');

  const load = async () => {
    setLoading(true);
    setError('');
    try {
      const [assembly, partsPage] = await Promise.all([
        getPartAssembly(part.id),
        listAircraftPart(1, 100),
      ]);
      setView(assembly.data);
      setAllParts(partsPage.data);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [part.id]);

  const mountedIds = useMemo(() => {
    const ids = new Set<number>([part.id]);
    view?.children.forEach((child) => ids.add(child.id));
    if (view?.parent) ids.add(view.parent.id);
    return ids;
  }, [part.id, view]);

  // 只能选择没有挂到其他组件下的部件；组件自己也不能挂给自己。
  const availableChildren = useMemo(
    () => allParts
      .filter((candidate) => !mountedIds.has(candidate.id))
      .sort((a, b) => a.code.localeCompare(b.code)),
    [allParts, mountedIds],
  );

  const register = async () => {
    if (!childId) return;
    setError(''); setMessage('');
    try {
      await registerPartAssembly(part.id, Number(childId));
      setChildId('');
      setMessage('装配关系已登记');
      await load();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    }
  };

  const unmount = async (linkId: number) => {
    setError(''); setMessage('');
    try {
      await removePartAssembly(linkId);
      setMessage('装配关系已拆除');
      await load();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    }
  };

  return <section className="certificate-section assembly-section">
    <header>
      <h2>装配清单与核对结果</h2>
      <span>组件 {part.code} 下的子件逐级核对；暂停 / 退役 / 未放行将阻断组件放行</span>
    </header>
    {error && <div className="alert" role="alert">{error}</div>}
    {message && <div className="assembly-ok" role="status">{message}</div>}

    {view?.parent && <p className="assembly-parent">
      上级组件：<strong>{view.parent.code}</strong> {view.parent.name}
      <PartStatusBadge status={view.parent.status} />
    </p>}

    <div className="assembly-table-shell">
      <table className="assembly-table">
        <thead><tr><th>子件编号</th><th>名称</th><th>状态</th><th>核对层级</th><th>操作</th></tr></thead>
        <tbody>
          {view?.children.map((child) => {
            const link = view.links.find((item) => item.child.id === child.id);
            return <tr key={child.id}>
              <td><strong>{child.code}</strong></td>
              <td>{child.name}</td>
              <td><PartStatusBadge status={child.status} /></td>
              <td>直接子件 · L1</td>
              <td>{canOperate && link ? <button className="table-action" onClick={() => void unmount(link.id)}>拆除</button> : <span className="muted">-</span>}</td>
            </tr>;
          })}
          {!loading && !view?.children.length && <tr><td colSpan={5} className="empty">该部件尚未登记子件</td></tr>}
        </tbody>
      </table>
    </div>

    <div className={`assembly-check ${view?.check.ready === false ? 'is-blocked' : 'is-ready'}`}>
      {loading ? <span className="muted">正在逐级核对…</span>
        : view?.check.ready
          ? <><strong>核对通过：</strong>已逐级核对 {view.check.checked} 个下级部件，全部已放行，组件可以进入批准。</>
          : <>
            <strong>核对未通过（{view?.check.blocked.length ?? 0} 个卡住）：</strong>
            以下下级部件处于暂停、退役或未放行状态，复核员批准时授权将留在待复核。
            <ul className="blocked-list">
              {view?.check.blocked.map((blocked) => <li key={`${blocked.level}-${blocked.id}`}>
                <strong>{blocked.code}</strong> · {blocked.name} · L{blocked.level}
                <PartStatusBadge status={blocked.status} />
                <em>{blocked.reason}</em>
              </li>)}
            </ul>
          </>}
    </div>

    {canOperate && <div className="assembly-register">
      <label htmlFor={`child-select-${part.id}`}>登记子件</label>
      <select id={`child-select-${part.id}`} value={childId} onChange={(event) => setChildId(event.target.value ? Number(event.target.value) : '')}>
        <option value="">选择一个未挂载的部件…</option>
        {availableChildren.map((candidate) => <option key={candidate.id} value={candidate.id}>{candidate.code} · {candidate.name}（{candidate.status}）</option>)}
      </select>
      <UiButton onClick={() => void register()} disabled={!childId}>登记装配关系</UiButton>
      {availableChildren.length === 0 && <span className="muted">没有可登记的空闲部件</span>}
    </div>}
  </section>;
}
