// 验收记录登记表单。
import { useEffect, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { acceptanceApi } from '../../api/acceptances';
import { recordApi } from '../../api/records';
import { taskApi } from '../../api/tasks';
import { FormField } from '../../components/FormField';
import { PageHeader } from '../../components/PageHeader';
import { SectionCard } from '../../components/SectionCard';
import { useToast } from '../../components/Toast';
import { useAsync } from '../../hooks/useAsync';
import { useForm, type FormErrors } from '../../hooks/useForm';
import { useMeta } from '../../providers/MetaProvider';
import type { AcceptancePayload, ScoreScheme } from '../../types/domain';
import { formatDate, isDateString, today } from '../../utils/format';
import { optionLabel } from '../../utils/options';

/** 合格验收的最低评分，与后端 passScoreThreshold 保持一致。 */
const PASS_SCORE_THRESHOLD = 60;

interface AcceptanceFormValues {
  taskId: string;
  cleaningRecordId: string;
  acceptedAt: string;
  inspectorName: string;
  inspectorOrg: string;
  result: string;
  /** 手工总分：验收日期没有生效评分方案时使用。 */
  score: string;
  /** 逐项打分：key 为评分项 ID，有生效评分方案时使用。 */
  itemScores: Record<string, string>;
  residualSludgeMm: string;
  issues: string;
  rectification: string;
  rectifyDeadline: string;
  remark: string;
}

function emptyForm(taskId = ''): AcceptanceFormValues {
  return {
    taskId,
    cleaningRecordId: '',
    acceptedAt: today(),
    inspectorName: '',
    inspectorOrg: '',
    result: 'pass',
    score: '90',
    itemScores: {},
    residualSludgeMm: '0',
    issues: '',
    rectification: '',
    rectifyDeadline: '',
    remark: ''
  };
}

/** 汇总逐项打分；空值与非数字按 0 处理（校验阶段会逐项拦截）。 */
function sumItemScores(values: AcceptanceFormValues, scheme: ScoreScheme): number {
  return scheme.items.reduce((sum, item) => {
    const parsed = Number(values.itemScores[String(item.id)]);
    return sum + (Number.isFinite(parsed) ? parsed : 0);
  }, 0);
}

function toPayload(values: AcceptanceFormValues, scheme: ScoreScheme | null): AcceptancePayload {
  return {
    taskId: Number(values.taskId),
    cleaningRecordId: values.cleaningRecordId ? Number(values.cleaningRecordId) : null,
    acceptedAt: values.acceptedAt,
    inspectorName: values.inspectorName.trim(),
    inspectorOrg: values.inspectorOrg.trim(),
    result: values.result as AcceptancePayload['result'],
    score: scheme ? sumItemScores(values, scheme) : Number(values.score),
    scoreItems: scheme
      ? scheme.items.map((item) => ({ itemId: item.id, score: Number(values.itemScores[String(item.id)] ?? '0') || 0 }))
      : [],
    residualSludgeMm: values.residualSludgeMm === '' ? 0 : Number(values.residualSludgeMm),
    issues: values.issues.trim(),
    rectification: values.rectification.trim(),
    rectifyDeadline: values.result === 'rework' && values.rectifyDeadline ? values.rectifyDeadline : null,
    remark: values.remark.trim()
  };
}

function validate(values: AcceptanceFormValues, scheme: ScoreScheme | null): FormErrors<AcceptanceFormValues> {
  const errors: FormErrors<AcceptanceFormValues> = {};
  if (!values.taskId) {
    errors.taskId = '请选择待验收的清淤任务';
  }
  if (!values.acceptedAt) {
    errors.acceptedAt = '验收日期不能为空';
  } else if (!isDateString(values.acceptedAt)) {
    errors.acceptedAt = '验收日期格式应为 YYYY-MM-DD';
  } else if (values.acceptedAt > today()) {
    errors.acceptedAt = '验收日期不能晚于今天';
  }
  if (!values.inspectorName.trim()) {
    errors.inspectorName = '验收人不能为空';
  }
  if (!values.result) {
    errors.result = '请选择验收结论';
  }

  let total: number;
  if (scheme) {
    total = sumItemScores(values, scheme);
    for (const item of scheme.items) {
      const raw = values.itemScores[String(item.id)] ?? '';
      const parsed = Number(raw);
      if (raw === '' || !Number.isInteger(parsed) || parsed < 0 || parsed > item.maxScore) {
        errors.itemScores = `评分项「${item.name}」得分需为 0 ~ ${item.maxScore} 之间的整数`;
        break;
      }
    }
  } else {
    total = Number(values.score);
    if (values.score === '' || !Number.isInteger(total) || total < 0 || total > 100) {
      errors.score = '验收评分需为 0 ~ 100 之间的整数';
    }
  }
  if (total < PASS_SCORE_THRESHOLD && values.result !== 'rework') {
    errors.result = `总分低于 ${PASS_SCORE_THRESHOLD} 分时结论只能选需整改`;
  }

  const residual = Number(values.residualSludgeMm);
  if (values.residualSludgeMm === '' || Number.isNaN(residual) || residual < 0 || residual > 1000) {
    errors.residualSludgeMm = '残留淤积厚度需在 0 ~ 1000 之间（mm）';
  }

  if (values.result === 'rework') {
    if (!values.issues.trim()) {
      errors.issues = '验收结论为需整改时，必须填写存在问题';
    }
    if (!values.rectifyDeadline) {
      errors.rectifyDeadline = '验收结论为需整改时，必须填写整改期限';
    } else if (values.acceptedAt && values.rectifyDeadline < values.acceptedAt) {
      errors.rectifyDeadline = '整改期限不能早于验收日期';
    }
  }
  return errors;
}

export function AcceptanceFormPage() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const toast = useToast();
  const { enums } = useMeta();

  const form = useForm<AcceptanceFormValues>(emptyForm(searchParams.get('taskId') ?? ''));
  const { values, setValue } = form;
  const tasks = useAsync(() => taskApi.list({ pageSize: 100 }), []);

  const selectedTaskId = Number(values.taskId || '0');
  const records = useAsync(
    () => (selectedTaskId > 0 ? recordApi.list({ taskId: selectedTaskId, pageSize: 100 }) : Promise.resolve(null)),
    [selectedTaskId]
  );

  // 评分方案按验收日期匹配：调整评分项带生效时间，不同日期可能适用不同版本。
  const schemeQuery = useAsync(
    () => acceptanceApi.effectiveScheme(isDateString(values.acceptedAt) ? values.acceptedAt : undefined),
    [values.acceptedAt]
  );
  const scheme = schemeQuery.data ?? null;

  // 方案切换时把逐项打分重置为满分，由验收人按扣分说明逐项扣减。
  const [scoredSchemeId, setScoredSchemeId] = useState(0);
  useEffect(() => {
    if (!scheme || scheme.id === scoredSchemeId) {
      return;
    }
    const next: Record<string, string> = {};
    scheme.items.forEach((item) => {
      next[String(item.id)] = String(item.maxScore);
    });
    setValue('itemScores', next);
    setScoredSchemeId(scheme.id);
  }, [scheme, scoredSchemeId, setValue]);

  const total = scheme ? sumItemScores(values, scheme) : Number(values.score) || 0;
  const belowThreshold = total < PASS_SCORE_THRESHOLD;

  // 总分低于合格线时结论只能选需整改：自动切换并锁定，提交前校验兜底。
  useEffect(() => {
    if (belowThreshold && values.result !== 'rework') {
      setValue('result', 'rework');
    }
  }, [belowThreshold, values.result, setValue]);

  const [submitting, setSubmitting] = useState(false);

  const submit = () => {
    void form.handleSubmit(async () => {
      setSubmitting(true);
      try {
        const created = await acceptanceApi.create(toPayload(form.values, scheme));
        toast.success('验收记录已登记');
        navigate(`/acceptances/${created.id}`);
      } finally {
        setSubmitting(false);
      }
    }, (current) => validate(current, scheme));
  };

  // 只有「待验收」的任务可以登记验收。
  const assignable = (tasks.data?.list ?? []).filter((item) => item.status === 'completed');
  const currentTask = (tasks.data?.list ?? []).find((item) => item.id === selectedTaskId);
  const taskOptions =
    currentTask && !assignable.some((item) => item.id === currentTask.id) ? [currentTask, ...assignable] : assignable;
  const selectedTask = currentTask;
  const recordItems = records.data?.list ?? [];
  const isRework = values.result === 'rework';

  const setItemScore = (itemId: number, raw: string) => {
    setValue('itemScores', { ...values.itemScores, [String(itemId)]: raw });
  };

  return (
    <form
      className="page"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <PageHeader
        title="登记验收记录"
        description="验收对象必须是已完成清淤并报验的任务；验收合格会同步把管段置为正常并累计清淤次数。"
        actions={
          <button type="button" className="btn btn-ghost" onClick={() => navigate(-1)}>
            返回
          </button>
        }
      />

      {form.serverError ? (
        <div className="alert alert-error">
          <p>{form.serverError}</p>
        </div>
      ) : null}

      {tasks.error ? (
        <div className="alert alert-warn">
          <p>任务下拉加载失败：{tasks.error}</p>
        </div>
      ) : null}

      <SectionCard
        title="验收对象"
        subtitle={`仅列出「待验收」的任务，共 ${assignable.length} 条；任务需先录入清淤记录再完工报验`}
        extra={<Link className="link" to="/tasks?status=completed">查看全部待验收任务</Link>}
      >
        <div className="form-grid">
          <FormField label="待验收任务" required span={2} error={form.errors.taskId}>
            <select
              className="select"
              value={values.taskId}
              onChange={(event) => setValue('taskId', event.target.value)}
            >
              <option value="">请选择任务</option>
              {taskOptions.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.code} · {item.title}（{optionLabel(enums?.taskStatuses, item.status)}）
                </option>
              ))}
            </select>
          </FormField>
          <FormField
            label="关联清淤记录"
            hint={
              selectedTaskId === 0
                ? '请先选择任务'
                : `该任务共 ${recordItems.length} 条清淤记录，可不指定`
            }
            error={form.errors.cleaningRecordId}
          >
            <select
              className="select"
              value={values.cleaningRecordId}
              onChange={(event) => setValue('cleaningRecordId', event.target.value)}
            >
              <option value="">不指定</option>
              {recordItems.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.code}（{item.cleanedAt ?? '未填日期'}）
                </option>
              ))}
            </select>
          </FormField>
        </div>
        {selectedTask ? (
          <>
            <div style={{ height: 12 }} />
            <div className="alert alert-info">
              <p>
                任务 {selectedTask.code} · 管段 {selectedTask.segment?.code ?? '—'}{' '}
                {selectedTask.segment?.name ?? ''} · 班组 {selectedTask.teamName || '—'} · 清淤记录{' '}
                {selectedTask.recordTotals?.recordCount ?? 0} 条 · 清淤量{' '}
                {selectedTask.recordTotals?.sludgeVolumeM3 ?? 0} m³
              </p>
            </div>
          </>
        ) : null}
      </SectionCard>

      <SectionCard title="验收结论" subtitle="带 * 的字段为必填项">
        <div className="form-grid">
          <FormField label="验收日期" required error={form.errors.acceptedAt}>
            <input
              className="input"
              type="date"
              value={values.acceptedAt}
              onChange={(event) => setValue('acceptedAt', event.target.value)}
            />
          </FormField>
          <FormField label="验收人" required error={form.errors.inspectorName}>
            <input
              className="input"
              value={values.inspectorName}
              onChange={(event) => setValue('inspectorName', event.target.value)}
            />
          </FormField>
          <FormField label="验收单位" error={form.errors.inspectorOrg}>
            <input
              className="input"
              value={values.inspectorOrg}
              onChange={(event) => setValue('inspectorOrg', event.target.value)}
            />
          </FormField>
          <FormField
            label="验收结论"
            required
            hint={belowThreshold ? `总分低于 ${PASS_SCORE_THRESHOLD} 分，结论只能选需整改` : undefined}
            error={form.errors.result}
          >
            <select
              className="select"
              value={values.result}
              onChange={(event) => setValue('result', event.target.value)}
            >
              {(enums?.acceptanceResults ?? []).map((item) => (
                <option key={item.value} value={item.value} disabled={item.value === 'pass' && belowThreshold}>
                  {item.label}
                </option>
              ))}
            </select>
          </FormField>
          {scheme ? (
            <div className="form-field form-span-3">
              <span className="form-label">
                逐项评分
                <em className="form-required">*</em>
              </span>
              <div className="score-item-grid">
                {scheme.items.map((item) => (
                  <div key={item.id} className="score-item">
                    <div className="score-item-head">
                      <span className="score-item-name">{item.name}</span>
                      <span className="score-item-max">满分 {item.maxScore} 分</span>
                    </div>
                    <input
                      className="input"
                      inputMode="numeric"
                      placeholder={`0 ~ ${item.maxScore}`}
                      value={values.itemScores[String(item.id)] ?? ''}
                      onChange={(event) => setItemScore(item.id, event.target.value)}
                    />
                    {item.deduction ? <span className="form-hint">{item.deduction}</span> : null}
                  </div>
                ))}
              </div>
              <span className="form-hint">
                评分方案：{scheme.title}（{formatDate(scheme.effectiveFrom)} 起生效），总分由评分项自动汇总
              </span>
              {form.errors.itemScores ? <span className="form-error">{form.errors.itemScores}</span> : null}
            </div>
          ) : (
            <FormField
              label="验收评分"
              required
              hint={
                schemeQuery.loading
                  ? '正在查询生效的评分方案…'
                  : `该验收日期没有生效的评分方案，沿用手工总分；合格判定不低于 ${PASS_SCORE_THRESHOLD} 分`
              }
              error={form.errors.score}
            >
              <input
                className="input"
                inputMode="numeric"
                value={values.score}
                onChange={(event) => setValue('score', event.target.value)}
              />
            </FormField>
          )}
          <FormField label="残留淤积厚度（mm）" error={form.errors.residualSludgeMm}>
            <input
              className="input"
              inputMode="decimal"
              value={values.residualSludgeMm}
              onChange={(event) => setValue('residualSludgeMm', event.target.value)}
            />
          </FormField>
          <FormField
            label="整改期限"
            hint={isRework ? '需整改时必填，且不早于验收日期' : '仅「需整改」结论需要填写'}
            error={form.errors.rectifyDeadline}
          >
            <input
              className="input"
              type="date"
              disabled={!isRework}
              value={values.rectifyDeadline}
              onChange={(event) => setValue('rectifyDeadline', event.target.value)}
            />
          </FormField>
          <FormField label="存在问题" span={3} error={form.errors.issues}>
            <textarea
              className="textarea"
              value={values.issues}
              placeholder="需整改时必填，例如：K0+320 处残留淤积厚度超标"
              onChange={(event) => setValue('issues', event.target.value)}
            />
          </FormField>
          <FormField label="整改要求" span={3} error={form.errors.rectification}>
            <textarea
              className="textarea"
              value={values.rectification}
              onChange={(event) => setValue('rectification', event.target.value)}
            />
          </FormField>
          <FormField label="备注" span={3} error={form.errors.remark}>
            <textarea
              className="textarea"
              value={values.remark}
              onChange={(event) => setValue('remark', event.target.value)}
            />
          </FormField>
        </div>

        <div className={`alert ${belowThreshold ? 'alert-warn' : 'alert-info'}`}>
          <p>
            当前总分 <strong>{total}</strong> 分（满分 100 分，合格线 {PASS_SCORE_THRESHOLD} 分）
            {belowThreshold ? '，低于合格线，结论只能登记为「需整改」，并需填写存在问题与整改期限。' : '。'}
          </p>
        </div>

        <div className="form-actions">
          <button type="button" className="btn btn-ghost" onClick={() => navigate(-1)}>
            取消
          </button>
          <button type="submit" className="btn btn-primary" disabled={form.submitting || submitting}>
            {form.submitting || submitting ? '提交中…' : '提交验收结论'}
          </button>
        </div>
      </SectionCard>
    </form>
  );
}
