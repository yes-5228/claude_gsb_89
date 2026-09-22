import type {
  AcceptanceDetail,
  AcceptanceListItem,
  AcceptancePayload,
  CreateTemplatePayload,
  PageResult,
  RectifyPayload,
  ScoreTemplate,
  ScoreTemplateItem
} from '../types/domain';
import { buildQuery, http } from './client';

export interface AcceptanceQuery {
  keyword?: string;
  taskId?: number;
  segmentId?: number;
  result?: string;
  inspectorName?: string;
  dateFrom?: string;
  dateTo?: string;
  pendingRectify?: boolean;
  page?: number;
  pageSize?: number;
}

export const acceptanceApi = {
  list: (query: AcceptanceQuery) =>
    http.get<PageResult<AcceptanceListItem>>(`/acceptances${buildQuery({ ...query })}`),
  detail: (id: number) => http.get<AcceptanceDetail>(`/acceptances/${id}`),
  create: (payload: AcceptancePayload) => http.post<{ id: number }>('/acceptances', payload),
  rectify: (id: number, payload: RectifyPayload) => http.post<{ id: number }>(`/acceptances/${id}/rectify`, payload),
  remove: (id: number) => http.del<{ id: number }>(`/acceptances/${id}`),
  /** 全部评分模板版本（按生效日期倒序）。 */
  listTemplates: () => http.get<ScoreTemplate[]>('/acceptances/score-templates'),
  /** 指定日期（默认今天）适用的评分模板。 */
  effectiveTemplate: (date?: string) =>
    http.get<{ effectiveFrom: string; items: ScoreTemplateItem[] }>(
      `/acceptances/score-templates/effective${buildQuery({ date })}`
    ),
  /** 发布新的评分模板版本。 */
  createTemplate: (payload: CreateTemplatePayload) =>
    http.post<ScoreTemplate>('/acceptances/score-templates', payload)
};
