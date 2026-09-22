// 验收记录登记表单：按验收日期适用的评分模板逐项打分，总分自动汇总；
// 总分低于合格线时结论强制为「需整改」并要求填写存在问题与整改期限。
import { useEffect, useMemo, useState } from 'react';
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
import type { AcceptancePayload, ScoreTemplateItem } from '../../types/domain';
import { isDateString, today } from '../../utils/format';
import { optionLabel } from '../../utils/options';

/** 合格验收的最低评分，与后端 passScoreThreshold 保持一致。 */
const PASS_SCORE_THRESHOLD = 60;

/** 单个评分项的表单状态：实际得分文本（便于输入控制）+ 扣分说明。 */
interface ItemFormState {
  score: string;
  reason: string;
}

interface AcceptanceFormValues {
  taskId: string;
  cleaningRecordId: string;
  acceptedAt: string;
  inspectorName: string;
  inspectorOrg: string;
  result: string;
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
    residualSludgeMm: '0',
    issues: '',
    rectification: '',
    rectifyDeadline: '',
    remark: ''
  };
}

/** 计算各项得分合计，任一项非法时返回 null。 */
function sumItems(items: ScoreTemplateItem[], states: Record<string, ItemFormState>): number | null {
  let total = 0;
  for (const item of items) {
    const state = states[item.name];
    if (!state) {
      return null;
    }
    const score = Number(state.score);
    if (state.score === '' || !Number.isInteger(score) || score < 0 || score > item.maxScore) {
      return null;
    }
    total += score;
  }
  return total;
}

export function AcceptanceFormPage() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const toast = useToast();
  const { enums } = useMeta();

  const form = useForm<AcceptanceFormValues>(emptyForm(searchParams.get('taskId') ?? ''));
  const tasks = useAsync(() => taskApi.list({ pageSize: 100 }), []);

  const acceptedAt = form.values.acceptedAt;
  // 评分模板随验收日期变化：评分项调整带生效时间，登记时锁定当时版本。
  const template = useAsync(
    () =>
      isDateString(acceptedAt)
        ? acceptanceApi.effectiveTemplate(acceptedAt)
        : Promise.resolve({ effectiveFrom: '', items: [] as ScoreTemplateItem[] }),
    [acceptedAt]
  );
  const templateItems = template.data?.items ?? [];
  const templateItemKey = templateItems.map((item) => item.name).join('|');

  const [itemStates, setItemStates] = useState<Record<string, ItemFormState>>({});
  // 模板切换时补齐新出现的评分项（默认满分），保留同名校验项已填写的数据。
  useEffect(() => {
    setItemStates((prev) => {
      const next: Record<string, ItemFormState> = {};
      templateItems.forEach((item) => {
        next[item.name] = prev[item.name] ?? { score: String(item.maxScore), reason: '' };
      });
      return next;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [templateItemKey]);

  const selectedTaskId = Number(form.values.taskId || '0');
  const records = useAsync(
    () => (selectedTaskId > 0 ? recordApi.list({ taskId: selectedTaskId, pageSize: 100 }) : Promise.resolve(null)),
    [selectedTaskId]
  );

  const [submitting, setSubmitting] = useState(false);

  const totalScore = useMemo(() => sumItems(templateItems, itemStates), [templateItems, itemStates]);
  const belowThreshold = totalScore !== null && totalScore < PASS_SCORE_THRESHOLD;

  // 总分与结论强一致：跌破合格线强制「需整改」，达到合格线回到「合格」，与后端校验保持一致。
  useEffect(() => {
    if (totalScore === null) {
      return;
    }
    const expected = totalScore < PASS_SCORE_THRESHOLD ? 'rework' : 'pass';
    if (form.values.result !== expected) {
      form.setValue('result', expected);
    }
  }, [totalScore, form.values.result]);

  const validate = (values: AcceptanceFormValues): FormErrors<AcceptanceFormValues> => {
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

    if (templateItems.length === 0) {
      errors.result = '该验收日期没有生效的评分模板，请先在评分项设置中发布模板版本';
    } else if (totalScore === null) {
      errors.result = '请为每个评分项填写 0 到分值上限之间的整数得分';
    } else if (totalScore < PASS_SCORE_THRESHOLD && values.result !== 'rework') {
      errors.result = `评分项汇总 ${totalScore} 分低于合格线，结论只能选择需整改`;
    } else if (totalScore >= PASS_SCORE_THRESHOLD && values.result === 'rework') {
      errors.result = `评分项汇总 ${totalScore} 分已达到合格线，不能选择需整改，请核对扣分`;
    }

    const residual = Number(values.residualSludgeMm);
    if (values.residualSludgeMm === '' || Number.isNaN(residual) || residual < 0 || residual > 1000) {
      errors.residualSludgeMm = '残留淤积厚度需在 0 ~ 1000 之间（mm）';
    }

    // 逐项校验扣分说明：扣分必填，满分不允许填。
    templateItems.forEach((item) => {
      const state = itemStates[item.name];
      if (!state || state.score === '') {
        return;
      }
      const score = Number(state.score);
      if (!Number.isInteger(score) || score < 0 || score > item.maxScore) {
        errors.result = `评分项「${item.name}」得分需为 0 ~ ${item.maxScore} 的整数`;
      } else if (score < item.maxScore && !state.reason.trim()) {
        errors.result = `评分项「${item.name}」扣减了 ${item.maxScore - score} 分，必须填写扣分说明`;
      } else if (score === item.maxScore && state.reason.trim()) {
        errors.result = `评分项「${item.name}」为满分时不允许填写扣分说明`;
      }
    });

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
  };

  const submit = () => {
    void form.handleSubmit(async () => {
      const payload: AcceptancePayload = {
        taskId: Number(form.values.taskId),
        cleaningRecordId: form.values.cleaningRecordId ? Number(form.values.cleaningRecordId) : null,
        acceptedAt: form.values.acceptedAt,
        inspectorName: form.values.inspectorName.trim(),
        inspectorOrg: form.values.inspectorOrg.trim(),
        result: form.values.result as AcceptancePayload['result'],
        residualSludgeMm: form.values.residualSludgeMm === '' ? 0 : Number(form.values.residualSludgeMm),
        scoreItems: templateItems.map((item) => ({
          name: item.name,
          actualScore: Number(itemStates[item.name]?.score ?? item.maxScore),
          deductionReason: itemStates[item.name]?.reason.trim() ?? ''
        })),
        issues: form.values.issues.trim(),
        rectification: form.values.rectification.trim(),
        rectifyDeadline: form.values.result === 'rework' && form.values.rectifyDeadline ? form.values.rectifyDeadline : null,
        remark: form.values.remark.trim()
      };
      setSubmitting(true);
      try {
        const created = await acceptanceApi.create(payload);
        toast.success('验收记录已登记');
        navigate(`/acceptances/${created.id}`);
      } finally {
        setSubmitting(false);
      }
    }, validate);
  };

  // 只有「待验收」的任务可以登记验收。
  const assignable = (tasks.data?.list ?? []).filter((item) => item.status === 'completed');
  const currentTask = (tasks.data?.list ?? []).find((item) => item.id === selectedTaskId);
  const taskOptions =
    currentTask && !assignable.some((item) => item.id === currentTask.id) ? [currentTask, ...assignable] : assignable;
  const selectedTask = currentTask;
  const recordItems = records.data?.list ?? [];
  const isRework = form.values.result === 'rework';

  const updateItem = (name: string, patch: Partial<ItemFormState>) => {
    setItemStates((prev) => ({ ...prev, [name]: { ...prev[name], ...patch } }));
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
        description="按评分项逐项打分，总分自动汇总；总分低于合格线时结论只能选择需整改，并须填写存在问题与整改期限。"
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
              value={form.values.taskId}
              onChange={(event) => form.setValue('taskId', event.target.value)}
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
              value={form.values.cleaningRecordId}
              onChange={(event) => form.setValue('cleaningRecordId', event.target.value)}
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

      <SectionCard
        title="评分项打分"
        subtitle={
          template.data
            ? `适用模板自 ${template.data.effectiveFrom} 起生效，共 ${templateItems.length} 个评分项，满分 100 分`
            : '按验收日期加载适用的评分模板'
        }
        extra={<Link className="link" to="/settings/score-templates">评分项设置</Link>}
      >
        {template.loading ? (
          <p className="form-note">评分模板加载中…</p>
        ) : template.error ? (
          <div className="alert alert-warn">
            <p>评分模板加载失败：{template.error}</p>
          </div>
        ) : templateItems.length === 0 ? (
          <div className="alert alert-error">
            <p>该验收日期没有生效的评分模板，请先在评分项设置中发布模板版本。</p>
          </div>
        ) : (
          <div className="score-item-table">
            <table className="data-table">
              <thead>
                <tr>
                  <th style={{ width: 48 }}>序号</th>
                  <th>评分项</th>
                  <th style={{ width: 200 }}>评分标准</th>
                  <th style={{ width: 110, textAlign: 'right' }}>分值上限</th>
                  <th style={{ width: 130 }}>实际得分</th>
                  <th>扣分说明</th>
                </tr>
              </thead>
              <tbody>
                {templateItems.map((item, index) => {
                  const state = itemStates[item.name] ?? { score: String(item.maxScore), reason: '' };
                  const score = Number(state.score);
                  const invalid =
                    state.score === '' || !Number.isInteger(score) || score < 0 || score > item.maxScore;
                  return (
                    <tr key={item.name}>
                      <td>{index + 1}</td>
                      <td>
                        <span className="cell-main">{item.name}</span>
                      </td>
                      <td>
                        <span className="cell-sub">{item.criteria || '—'}</span>
                      </td>
                      <td style={{ textAlign: 'right' }}>
                        <span className="cell-num">{item.maxScore} 分</span>
                      </td>
                      <td>
                        <input
                          className={`input input-score${invalid ? ' input-invalid' : ''}`}
                          inputMode="numeric"
                          value={state.score}
                          onChange={(event) => updateItem(item.name, { score: event.target.value })}
                        />
                      </td>
                      <td>
                        <input
                          className="input"
                          placeholder={score < item.maxScore ? '扣分时必填' : '满分时无需填写'}
                          value={state.reason}
                          onChange={(event) => updateItem(item.name, { reason: event.target.value })}
                        />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            <div className={`score-total ${belowThreshold ? 'score-total-danger' : 'score-total-pass'}`}>
              <span>
                自动汇总总分：<strong>{totalScore ?? '—'}</strong> / 100 分
              </span>
              {belowThreshold ? (
                <span className="tag tag-danger">低于合格线 {PASS_SCORE_THRESHOLD} 分，结论只能选需整改</span>
              ) : (
                <span className="tag tag-success">达到合格线 {PASS_SCORE_THRESHOLD} 分</span>
              )}
            </div>
          </div>
        )}
      </SectionCard>

      <SectionCard title="验收结论" subtitle="带 * 的字段为必填项">
        <div className="form-grid">
          <FormField label="验收日期" required error={form.errors.acceptedAt}>
            <input
              className="input"
              type="date"
              value={form.values.acceptedAt}
              onChange={(event) => form.setValue('acceptedAt', event.target.value)}
            />
          </FormField>
          <FormField label="验收人" required error={form.errors.inspectorName}>
            <input
              className="input"
              value={form.values.inspectorName}
              onChange={(event) => form.setValue('inspectorName', event.target.value)}
            />
          </FormField>
          <FormField label="验收单位" error={form.errors.inspectorOrg}>
            <input
              className="input"
              value={form.values.inspectorOrg}
              onChange={(event) => form.setValue('inspectorOrg', event.target.value)}
            />
          </FormField>
          <FormField label="验收结论" required error={form.errors.result}>
            <select
              className="select"
              value={form.values.result}
              onChange={(event) => form.setValue('result', event.target.value)}
            >
              {(enums?.acceptanceResults ?? []).map((item) => (
                <option
                  key={item.value}
                  value={item.value}
                  disabled={belowThreshold && item.value === 'pass'}
                >
                  {item.label}
                  {belowThreshold && item.value === 'pass' ? '（总分低于合格线，不可选）' : ''}
                </option>
              ))}
            </select>
          </FormField>
          <FormField label="残留淤积厚度（mm）" error={form.errors.residualSludgeMm}>
            <input
              className="input"
              inputMode="decimal"
              value={form.values.residualSludgeMm}
              onChange={(event) => form.setValue('residualSludgeMm', event.target.value)}
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
              value={form.values.rectifyDeadline}
              onChange={(event) => form.setValue('rectifyDeadline', event.target.value)}
            />
          </FormField>
          <FormField label="存在问题" span={3} error={form.errors.issues}>
            <textarea
              className="textarea"
              value={form.values.issues}
              placeholder="需整改时必填，例如：K0+320 处残留淤积厚度超标"
              onChange={(event) => form.setValue('issues', event.target.value)}
            />
          </FormField>
          <FormField label="整改要求" span={3} error={form.errors.rectification}>
            <textarea
              className="textarea"
              value={form.values.rectification}
              onChange={(event) => form.setValue('rectification', event.target.value)}
            />
          </FormField>
          <FormField label="备注" span={3} error={form.errors.remark}>
            <textarea
              className="textarea"
              value={form.values.remark}
              onChange={(event) => form.setValue('remark', event.target.value)}
            />
          </FormField>
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
