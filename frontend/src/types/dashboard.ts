export interface Panel {
  id?: number
  uid: string
  title: string
  type: string
  x: number
  y: number
  w: number
  h: number
  query?: string
  queryConfig?: QueryConfig | null
  visualization?: VisualizationConfig
  options?: Record<string, unknown>
}

export interface Dashboard {
  id?: number
  uid: string
  name: string
  description?: string
  timeRange: string
  refreshInterval: number
  revision: number
  panels: Panel[]
  createdAt?: string
  updatedAt?: string
}

export type QueryMode = 'sql' | 'http' | 'pipeline' | 'none'

export interface HTTPQueryConfig {
  method: string
  path: string
  params?: Record<string, unknown>
  headers?: Record<string, string>
  body?: unknown
  rowsPath?: string
  fieldMap?: Record<string, string>
}

export interface PipelineNode {
  id: string
  kind: 'source' | 'join' | 'calculate' | 'aggregate'
  alias?: string
  query?: QueryConfig
  input?: string
  left?: string
  right?: string
  joinType?: 'left' | 'inner'
  leftKeys?: string[]
  rightKeys?: string[]
  fields?: Record<string, string>
  groupBy?: string[]
  aggregates?: Record<string, { op: string; field?: string }>
}

export interface QueryConfig {
  mode: QueryMode
  datasourceUid?: string
  sql?: string
  params?: Record<string, unknown>
  request?: HTTPQueryConfig
  nodes?: PipelineNode[]
  outputNodeId?: string
}

export interface VisualizationConfig {
  type?: string
  xField?: string
  yField?: string
  valueField?: string
  nameField?: string
  options?: Record<string, unknown>
}

export interface QueryColumn {
  name: string
  type: string
}

export interface QueryResult {
  columns: QueryColumn[]
  rows: Array<Record<string, unknown>>
  meta: { durationMs: number; rowCount: number; truncated?: boolean }
}

export interface DataSource {
  id?: number
  uid: string
  name: string
  type: 'mysql' | 'postgres' | 'sqlite' | 'http'
  config: Record<string, unknown>
  status: string
  lastError?: string
  createdAt?: string
  updatedAt?: string
}

export interface DataSourceInput {
  name: string
  type: DataSource['type']
  config: Record<string, unknown>
  credentials?: { password?: string; token?: string; headers?: Record<string, string> }
}

export interface TableInfo {
  schema: string
  name: string
  type: string
}

export interface TableColumn {
  name: string
  type: string
  nullable: boolean
  primaryKey: boolean
}

export interface TablePreview {
  schema: string
  table: string
  columns: QueryColumn[]
  rows: Array<Record<string, unknown>>
  page: number
  pageSize: number
  total: number
}
