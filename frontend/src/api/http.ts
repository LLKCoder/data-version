import type { Dashboard, DataSource, DataSourceInput, QueryConfig, QueryResult, TableColumn, TableInfo, TablePreview } from '@/types/dashboard'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
    ...init,
  })

  if (!response.ok) {
    throw new Error(`请求失败：${response.status}`)
  }

  return response.json() as Promise<T>
}

async function requestJson<T>(path: string, body: unknown, method = 'POST'): Promise<T> {
  return request<T>(path, { method, body: JSON.stringify(body) })
}

export const dashboardApi = {
  list: () => request<Dashboard[]>('/api/v1/dashboards'),
  get: (uid: string) => request<Dashboard>(`/api/v1/dashboards/${uid}`),
  create: (payload: Omit<Dashboard, 'uid' | 'id' | 'createdAt' | 'updatedAt'>) => requestJson<Dashboard>('/api/v1/dashboards', payload),
  update: (uid: string, payload: Dashboard) => requestJson<Dashboard>(`/api/v1/dashboards/${uid}`, payload, 'PUT'),
  exportPdf: (uid: string) => fetch(`/api/v1/dashboards/${uid}/export/pdf`, { method: 'POST' }),
  exportJson: (uid: string) => fetch(`/api/v1/dashboards/${uid}/export`),
  importJson: (payload: unknown) => requestJson<{ dashboard: Dashboard; datasourceMap: Record<string, string> }>('/api/v1/dashboards/import', payload),
}

export const datasourceApi = {
  list: () => request<DataSource[]>('/api/v1/datasources'),
  get: (uid: string) => request<DataSource>(`/api/v1/datasources/${uid}`),
  create: (payload: DataSourceInput) => requestJson<DataSource>('/api/v1/datasources', payload),
  update: (uid: string, payload: DataSourceInput) => requestJson<DataSource>(`/api/v1/datasources/${uid}`, payload, 'PUT'),
  remove: async (uid: string) => { const response = await fetch(`/api/v1/datasources/${uid}`, { method: 'DELETE' }); if (!response.ok) throw new Error(`请求失败：${response.status}`) },
  test: (uid: string, payload: DataSourceInput) => requestJson<{ ok: boolean; message: string }>(`/api/v1/datasources/${uid}/test`, payload),
  tables: (uid: string, search = '') => request<{ tables: TableInfo[] }>(`/api/v1/datasources/${uid}/tables?search=${encodeURIComponent(search)}`),
  schema: (uid: string, schema: string, table: string) => request<{ columns: TableColumn[] }>(`/api/v1/datasources/${uid}/tables/schema?schema=${encodeURIComponent(schema)}&table=${encodeURIComponent(table)}`),
  preview: (uid: string, params: { schema: string; table: string; page: number; pageSize: number; sort?: string; order?: string }) => request<TablePreview>(`/api/v1/datasources/${uid}/table-preview?schema=${encodeURIComponent(params.schema)}&table=${encodeURIComponent(params.table)}&page=${params.page}&pageSize=${params.pageSize}&sort=${encodeURIComponent(params.sort ?? '')}&order=${encodeURIComponent(params.order ?? '')}`),
}

export const queryApi = {
  execute: (config: QueryConfig) => requestJson<QueryResult>('/api/v1/query/execute', config),
}
