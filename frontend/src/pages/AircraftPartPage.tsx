
import { EntityPage } from '../components/EntityPage';
import { ENTITY_CONFIGS } from '../types/status';
import { useAircraftPartStore } from '../stores/aircraft-part';
import { AssemblyPanel } from '../components/common/AssemblyPanel';
import type { DomainRecord } from '../types/domain';

export default function AircraftPartPage() {
  return <EntityPage
    config={ENTITY_CONFIGS[0]}
    useStore={useAircraftPartStore}
    detailLabel="装配清单"
    renderDetail={(item: DomainRecord) => <AssemblyPanel part={item} />}
  />;
}
