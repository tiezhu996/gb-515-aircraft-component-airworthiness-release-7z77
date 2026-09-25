
import { EntityPage } from '../components/EntityPage';
import { ENTITY_CONFIGS } from '../types/status';
import { useReleaseAuthorizationStore } from '../stores/release-authorization';
import { AuthorizationGatePanel } from '../components/common/AuthorizationGatePanel';
import type { DomainRecord } from '../types/domain';

export default function ReleaseAuthorizationPage() {
  const store = useReleaseAuthorizationStore();
  return <EntityPage
    config={ENTITY_CONFIGS[3]}
    useStore={useReleaseAuthorizationStore}
    detailLabel="装配复核"
    renderDetail={(item: DomainRecord) => <AuthorizationGatePanel authorization={item} onChanged={() => void store.load('authorizations')} />}
  />;
}
