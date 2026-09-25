import { useEffect, useState } from 'react';
import { EntityPage } from '../components/EntityPage';
import { ENTITY_CONFIGS } from '../types/status';
import { useReleaseAuthorizationStore } from '../stores/release-authorization';
import { listAircraftPart } from '../api/aircraft-part';
import type { DomainRecord } from '../types/domain';

export default function ReleaseAuthorizationPage() {
	const [parts, setParts] = useState<DomainRecord[]>([]);
	const [partId, setPartId] = useState<string>('');

	useEffect(() => {
		void listAircraftPart(1, 100).then((result) => setParts(result.data)).catch(() => undefined);
	}, []);

	return <EntityPage
		config={ENTITY_CONFIGS[3]}
		useStore={useReleaseAuthorizationStore}
		createExtras={() => ({
			fields: <label className="create-extra">
				<span>关联组件部件（批准放行前逐级核对其下层子件）</span>
				<select value={partId} onChange={(event) => setPartId(event.target.value)}>
					<option value="">不关联具体部件</option>
					{parts.map((part) => <option key={part.id} value={part.id}>
						{part.code} — {part.name}（{part.status}）
					</option>)}
				</select>
			</label>,
			value: () => ({ partId: partId ? Number(partId) : null }),
		})}
	/>;
}
