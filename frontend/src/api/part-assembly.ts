
import { request } from './client';
import type { AssemblyView } from '../types/domain';

export async function getAssembly(partId: number) {
	return request<AssemblyView>(`/parts/${partId}/assembly`);
}

export async function registerAssembly(partId: number, childCode: string) {
	return request<AssemblyView>(`/parts/${partId}/assembly`, {
		method: 'POST',
		body: JSON.stringify({ childCode }),
	});
}

export async function removeAssembly(partId: number, childId: number) {
	return request<void>(`/parts/${partId}/assembly/${childId}`, { method: 'DELETE' });
}
