import { request } from './client';
import type { PartAssemblyView } from '../types/domain';

// 部件装配关系挂在独立前缀 /part-assembly 下，避免与 /parts/:id 的路由段冲突。
export async function getPartAssembly(partId: number) {
  return request<PartAssemblyView>(`/part-assembly/${partId}`);
}

export async function registerPartAssembly(parentPartId: number, childPartId: number) {
  return request<PartAssemblyView['links'][number]>('/part-assembly', {
    method: 'POST',
    body: JSON.stringify({ parentPartId, childPartId }),
  });
}

export async function removePartAssembly(linkId: number) {
  return request<void>(`/part-assembly/${linkId}`, { method: 'DELETE' });
}
