import { useCallback, useEffect, useState } from 'react';
import type { AssemblyNode, AssemblyView, BlockedPart } from '../../types/domain';
import { getAssembly, registerAssembly, removeAssembly } from '../../api/part-assembly';
import { PartStatusBadge } from './PartStatusBadge';
import { UiButton } from './UiButton';
import { useAuth } from '../../hooks/useAuth';

const blockedReasonTone = (reason: string) =>
	reason.includes('退役') ? 'danger' : reason.includes('暂停') ? 'warning' : 'neutral';

function NodeRow({ node, onRemove, canOperate, parentId }: {
	node: AssemblyNode; onRemove: (childId: number) => Promise<void>; canOperate: boolean; parentId: number;
}) {
	return <>
		<tr className={node.blocked ? 'assembly-row assembly-row--blocked' : 'assembly-row'}>
			<td style={{ paddingLeft: `${16 + (node.depth - 1) * 22}px` }}>
				{node.children.length > 0 ? '▸ ' : '• '}<strong>{node.code}</strong>
			</td>
			<td>{node.name}</td>
			<td><PartStatusBadge status={node.status} /></td>
			<td>{node.blocked
				? <span className={`assembly-reason assembly-reason--${blockedReasonTone(node.blocked.reason)}`}>{node.blocked.reason}</span>
				: <span className="assembly-reason assembly-reason--ok">已放行</span>}</td>
			<td>{canOperate && <button className="link-button" onClick={() => void onRemove(node.partId)}>移出</button>}</td>
		</tr>
		{node.children.map((child) =>
			<NodeRow key={`${node.partId}-${child.partId}`} node={child} onRemove={onRemove} canOperate={canOperate} parentId={parentId} />)}
	</>;
}

function flattenBlocked(nodes: AssemblyNode[], acc: BlockedPart[] = []): BlockedPart[] {
	for (const node of nodes) {
		if (node.blocked) acc.push(node.blocked);
		flattenBlocked(node.children, acc);
	}
	return acc;
}

export function AssemblyPanel({ partId }: { partId: number }) {
	const { hasRole } = useAuth();
	const canOperate = hasRole('operator');
	const [view, setView] = useState<AssemblyView | null>(null);
	const [childCode, setChildCode] = useState('');
	const [loading, setLoading] = useState(false);
	const [error, setError] = useState('');

	const reload = useCallback(async () => {
		setLoading(true);
		setError('');
		try {
			const result = await getAssembly(partId);
			setView(result.data);
		} catch (err) {
			setError(err instanceof Error ? err.message : String(err));
		} finally {
			setLoading(false);
		}
	}, [partId]);

	useEffect(() => { void reload(); }, [reload]);

	const submit = async () => {
		setError('');
		try {
			const result = await registerAssembly(partId, childCode.trim().toUpperCase());
			setView(result.data);
			setChildCode('');
		} catch (err) {
			setError(err instanceof Error ? err.message : String(err));
		}
	};

	const unmount = async (childId: number) => {
		setError('');
		try {
			await removeAssembly(partId, childId);
			await reload();
		} catch (err) {
			setError(err instanceof Error ? err.message : String(err));
		}
	};

	if (!view) return <section className="assembly-panel" aria-busy={loading}><p className="muted">正在载入装配清单…</p>{error && <div className="alert">{error}</div>}</section>;

	const blocked = flattenBlocked(view.children);
	return <section className="assembly-panel">
		<header className="assembly-header">
			<div>
				<h2>装配清单与逐级核对</h2>
				<span className="muted">组件 {view.code} 下的全部子件，批准放行前逐层核对</span>
			</div>
			<span className={view.allClear ? 'assembly-verdict assembly-verdict--ok' : 'assembly-verdict assembly-verdict--blocked'}>
				{view.allClear ? '核对通过：下层均已放行' : `核对未通过：${blocked.length} 个部件卡住`}
			</span>
		</header>

		{view.parents.length > 0 && <p className="assembly-parent-hint">
			该部件同时挂载在：{view.parents.map((parent) => <span key={parent.partId} className="assembly-parent-code">{parent.code}</span>)}
			之下（一个子件只能属于一个组件）。
		</p>}

		{!view.allClear && <div className="assembly-blocked-box" role="alert">
			<strong>放行被以下部件卡住，授权将留在待复核：</strong>
			<ul>
				{blocked.map((item) => <li key={`${item.partId}-${item.depth}`}>
					<strong>{item.code}</strong>（{item.name}）— {item.reason}，当前状态 {item.status}
				</li>)}
			</ul>
		</div>}

		{error && <div className="alert" role="alert">{error}</div>}

		<div className="table-shell"><table>
			<thead><tr><th>部件编号 / 层级</th><th>名称</th><th>状态</th><th>核对结果</th><th>操作</th></tr></thead>
			<tbody>
				{view.children.length === 0 && <tr><td colSpan={5} className="empty">尚未登记子件</td></tr>}
				{view.children.map((node) =>
					<NodeRow key={node.partId} node={node} onRemove={unmount} canOperate={canOperate} parentId={partId} />)}
			</tbody>
		</table></div>

		{canOperate && <div className="assembly-register">
			<input
				aria-label="子件编号"
				placeholder="输入要装入的子件编号，例如 AP-003"
				value={childCode}
				onChange={(event) => setChildCode(event.target.value)}
				onKeyDown={(event) => { if (event.key === 'Enter' && childCode.trim()) void submit(); }}
			/>
			<UiButton onClick={() => childCode.trim() && void submit()}>登记子件</UiButton>
		</div>}
	</section>;
}
