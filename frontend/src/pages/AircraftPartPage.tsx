
import { EntityPage } from '../components/EntityPage';
import { ENTITY_CONFIGS } from '../types/status';
import { useAircraftPartStore } from '../stores/aircraft-part';
import { AssemblyPanel } from '../components/common/AssemblyPanel';
import type { DomainRecord } from '../types/domain';

export default function AircraftPartPage() {
	return <EntityPage
		config={ENTITY_CONFIGS[0]}
		useStore={useAircraftPartStore}
		detailPanel={(selected: DomainRecord | null, onClose: () => void) => (
			<div className="detail-drawer">
				<div className="detail-drawer__bar">
					<span>部件 {selected?.code} 的装配视图</span>
					<button className="link-button" onClick={onClose}>关闭装配视图</button>
				</div>
				{selected && <AssemblyPanel key={selected.id} partId={selected.id} />}
			</div>
		)}
	/>;
}
