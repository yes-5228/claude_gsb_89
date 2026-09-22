// 与后端接口一一对应的领域类型定义。

export type PipeType = 'rainwater' | 'sewage' | 'combined';
export type SegmentStatus = 'normal' | 'attention' | 'blocked';
export type TaskStatus = 'pending' | 'in_progress' | 'completed' | 'accepted' | 'cancelled';
export type TaskPriority = 'low' | 'normal' | 'high' | 'urgent';
export type TaskSource = 'plan' | 'inspection' | 'complaint' | 'flood';
export type CleaningMethod = 'high_pressure' | 'winch' | 'grab' | 'manual' | 'robot';
export type Weather = 'sunny' | 'cloudy' | 'overcast' | 'light_rain' | 'heavy_rain';
export type AcceptanceResult = 'pass' | 'rework';

/** 任务可执行的操作标识，由后端 allowedActions 下发。 */
export type TaskAction = 'start' | 'complete' | 'accept' | 'cancel' | 'edit';

export interface Option {
  value: string;
  label: string;
}

export interface PageResult<T> {
  list: T[];
  total: number;
  page: number;
  pageSize: number;
}

// ---------- 管段台账 ----------

export interface PipeSegment {
  id: number;
  code: string;
  name: string;
  district: string;
  roadName: string;
  pipeType: PipeType;
  material: string;
  diameterMm: number;
  lengthM: number;
  depthM: number;
  startManhole: string;
  endManhole: string;
  buildYear: number;
  ownerUnit: string;
  status: SegmentStatus;
  lastCleanedAt: string | null;
  cleanedTimes: number;
  remark: string;
  createdAt: string;
  updatedAt: string;
}

export interface SegmentBrief {
  id: number;
  code: string;
  name: string;
  district: string;
  roadName: string;
}

export interface TaskStats {
  total: number;
  pending: number;
  inProgress: number;
  completed: number;
  accepted: number;
  cancelled: number;
}

export interface TaskRef {
  id: number;
  code: string;
  title: string;
  status: TaskStatus;
  priority: TaskPriority;
  teamName: string;
  planStartDate: string | null;
  planEndDate: string | null;
  recordCount: number;
  sludgeVolumeM3: number;
}

export interface SegmentDetail {
  segment: PipeSegment;
  taskStats: TaskStats;
  recentTasks: TaskRef[];
}

export interface SegmentHistoryItem {
  taskId: number;
  taskCode: string;
  title: string;
  status: TaskStatus;
  priority: TaskPriority;
  teamName: string;
  planStartDate: string | null;
  planEndDate: string | null;
  recordCount: number;
  sludgeVolumeM3: number;
  cleanedLengthM: number;
  acceptanceResult: AcceptanceResult | '';
  acceptedAt: string | null;
}

export interface SegmentOptions {
  items: SegmentBrief[];
  districts: string[];
}

export interface SegmentPayload {
  code: string;
  name: string;
  district: string;
  roadName: string;
  pipeType: PipeType;
  material: string;
  diameterMm: number;
  lengthM: number;
  depthM: number;
  startManhole: string;
  endManhole: string;
  buildYear: number;
  ownerUnit: string;
  status?: SegmentStatus;
  remark: string;
}

// ---------- 清淤任务 ----------

export interface CleaningTask {
  id: number;
  code: string;
  title: string;
  pipeSegmentId: number;
  priority: TaskPriority;
  source: TaskSource;
  method: CleaningMethod | '';
  planStartDate: string | null;
  planEndDate: string | null;
  teamName: string;
  leaderName: string;
  leaderPhone: string;
  status: TaskStatus;
  description: string;
  startedAt: string | null;
  finishedAt: string | null;
  acceptedAt: string | null;
  cancelReason: string;
  createdAt: string;
  updatedAt: string;
}

export interface RecordTotals {
  recordCount: number;
  sludgeVolumeM3: number;
  cleanedLengthM: number;
  latestCleanedAt: string | null;
}

export interface AcceptanceBrief {
  id: number;
  code: string;
  result: AcceptanceResult;
  acceptedAt: string | null;
  inspectorName: string;
  inspectorOrg: string;
  score: number;
  issues: string;
  rectifyDeadline: string | null;
  rectifiedAt: string | null;
}

export interface TaskListItem extends CleaningTask {
  segment: SegmentBrief | null;
  recordTotals: RecordTotals;
}

export interface TaskDetail {
  task: CleaningTask;
  segment: SegmentBrief | null;
  recordTotals: RecordTotals;
  acceptance: AcceptanceBrief | null;
  allowedActions: TaskAction[];
}

export interface TaskPayload {
  title: string;
  pipeSegmentId: number;
  priority: TaskPriority;
  source: TaskSource;
  method: CleaningMethod | '';
  planStartDate: string;
  planEndDate: string;
  teamName: string;
  leaderName: string;
  leaderPhone: string;
  description: string;
}

// ---------- 清淤记录 ----------

export interface TaskBrief {
  id: number;
  code: string;
  title: string;
  status: TaskStatus;
  priority: TaskPriority;
  pipeSegmentId: number;
  teamName: string;
  segmentCode: string;
  segmentName: string;
  segmentDistrict: string;
}

export interface CleaningRecord {
  id: number;
  code: string;
  taskId: number;
  cleanedAt: string | null;
  lengthM: number;
  sludgeVolumeM3: number;
  waterVolumeM3: number;
  personnelCount: number;
  method: CleaningMethod | '';
  equipment: string;
  weather: Weather | '';
  sludgeDisposalSite: string;
  safetyMeasures: string;
  problemFound: string;
  recorderName: string;
  remark: string;
  createdAt: string;
  updatedAt: string;
}

export interface RecordListItem extends CleaningRecord {
  task: TaskBrief | null;
}

export interface RecordDetail {
  record: CleaningRecord;
  task: TaskBrief | null;
}

export interface RecordPayload {
  taskId: number;
  cleanedAt: string;
  lengthM: number;
  sludgeVolumeM3: number;
  waterVolumeM3: number;
  personnelCount: number;
  method: CleaningMethod | '';
  equipment: string;
  weather: Weather | '';
  sludgeDisposalSite: string;
  safetyMeasures: string;
  problemFound: string;
  recorderName: string;
  remark: string;
}

// ---------- 验收记录 ----------

/** 评分模板版本下的单个评分项定义。 */
export interface ScoreTemplateItem {
  id?: number;
  templateId?: number;
  name: string;
  /** 分值上限，同一版本各评分项上限合计 100。 */
  maxScore: number;
  sortOrder: number;
  criteria: string;
}

/** 评分模板版本（调整评分项 = 发布带生效日期的新版本，旧版本不可变）。 */
export interface ScoreTemplate {
  id: number;
  effectiveFrom: string;
  remark: string;
  items: ScoreTemplateItem[];
  /** 是否为当前日期生效的版本（列表接口附带）。 */
  active?: boolean;
  createdAt?: string;
  updatedAt?: string;
}

/** 登记验收时提交的单个评分项打分结果。 */
export interface ScoreItemInput {
  name: string;
  actualScore: number | null;
  deductionReason: string;
}

/** 验收记录上的评分项明细快照（详情 / 列表 / 统计共用同一份固化数据）。 */
export interface AcceptanceScoreItem {
  id: number;
  acceptanceId: number;
  templateId: number;
  name: string;
  maxScore: number;
  actualScore: number;
  deduction: number;
  deductionReason: string;
  sortOrder: number;
}

export interface AcceptanceRecord {
  id: number;
  code: string;
  taskId: number;
  cleaningRecordId: number | null;
  acceptedAt: string | null;
  inspectorName: string;
  inspectorOrg: string;
  result: AcceptanceResult;
  score: number;
  scoreTemplateId: number | null;
  residualSludgeMm: number;
  issues: string;
  rectification: string;
  rectifyDeadline: string | null;
  rectifiedAt: string | null;
  remark: string;
  createdAt: string;
  updatedAt: string;
}

export interface AcceptanceListItem extends AcceptanceRecord {
  task: TaskBrief | null;
  scoreItems: AcceptanceScoreItem[];
}

export interface AcceptanceDetail {
  acceptance: AcceptanceRecord;
  scoreItems: AcceptanceScoreItem[];
  template: Pick<ScoreTemplate, 'id' | 'effectiveFrom' | 'remark'> | null;
  task: TaskBrief | null;
  recordTotals: RecordTotals;
}

export interface AcceptancePayload {
  taskId: number;
  cleaningRecordId: number | null;
  acceptedAt: string;
  inspectorName: string;
  inspectorOrg: string;
  result: AcceptanceResult;
  residualSludgeMm: number;
  scoreItems: ScoreItemInput[];
  issues: string;
  rectification: string;
  rectifyDeadline: string | null;
  remark: string;
}

export interface TemplateItemPayload {
  name: string;
  maxScore: number;
  sortOrder: number;
  criteria: string;
}

export interface CreateTemplatePayload {
  effectiveFrom: string;
  items: TemplateItemPayload[];
  remark: string;
}

export interface RectifyPayload {
  rectifiedAt: string;
  rectification: string;
  remark: string;
}

/** 运行看板：评分项逐项统计。 */
export interface ItemScoreStat {
  templateId: number;
  sortOrder: number;
  name: string;
  maxScore: number;
  sampleCount: number;
  averageScore: number;
  deductedCount: number;
  deductedRate: number;
}

export interface AcceptanceScoreStats {
  total: number;
  average: number;
  itemStats: ItemScoreStat[];
}

// ---------- 看板与元数据 ----------

export interface Overview {
  segmentTotal: number;
  segmentTotalLengthM: number;
  segmentByStatus: Record<string, number>;
  uncleanedSegmentCount: number;
  taskTotal: number;
  taskByStatus: Record<string, number>;
  taskOverdue: number;
  recordTotal: number;
  sludgeTotalM3: number;
  sludgeThisMonthM3: number;
  cleanedLengthM: number;
  acceptanceTotal: number;
  acceptancePassCount: number;
  /** 验收合格率，后端已按百分比返回（66.67 表示 66.67%）。 */
  acceptancePassRate: number;
  /** 验收平均总分，取验收记录主表固化的 score。 */
  averageAcceptanceScore: number;
  pendingAcceptanceCount: number;
  pendingRectifyCount: number;
}

export interface DistrictStat {
  district: string;
  segmentCount: number;
  segmentLengthM: number;
  uncleanedSegmentCount: number;
  lastCleanedAt: string | null;
  taskCount: number;
  acceptedTaskCount: number;
  sludgeVolumeM3: number;
}

export interface PendingAcceptanceItem {
  taskId: number;
  code: string;
  title: string;
  segmentCode: string;
  segmentName: string;
  segmentDistrict: string;
  teamName: string;
  planEndDate: string | null;
  finishedAt: string | null;
  recordCount: number;
  sludgeVolumeM3: number;
  overdueDays: number;
}

export interface RecentRecordItem {
  recordId: number;
  code: string;
  cleanedAt: string | null;
  taskId: number;
  taskCode: string;
  taskTitle: string;
  segmentCode: string;
  segmentName: string;
  teamName: string;
  recorderName: string;
  lengthM: number;
  sludgeVolumeM3: number;
}

export interface Enums {
  pipeTypes: Option[];
  segmentStatuses: Option[];
  materials: Option[];
  taskStatuses: Option[];
  taskPriorities: Option[];
  taskSources: Option[];
  cleaningMethods: Option[];
  weathers: Option[];
  acceptanceResults: Option[];
}
