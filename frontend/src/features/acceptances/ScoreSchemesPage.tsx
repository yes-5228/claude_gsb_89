// 评分项设置：查看评分方案版本历史，并通过新增版本调整评分项（调整带生效日期）。
import { useNavigate } from 'react-router-dom';
import { acceptanceApi } from '../../api/acceptances';
import { DataTable, type Column } from '../../components/DataTable';
import { FormField } from '../../components/FormField';
import { PageHeader } from '../../components/PageHeader';
import { SectionCard } from '../../components/SectionCard';
import { useToast } from '../../components/Toast';
import { useAsync } from '../../hooks/useAsync';
import { useForm, type FormErrors } from '../../hooks/useForm';
import type { ScoreScheme } from '../../types/domain';
import { formatDate, formatDateTime, isDateString, today } from '../../utils/format';

/** 方案分值上限之和固定为 100 分，与后端 schemeTotalScore 保持一致。 */
const SCHEME_TOTAL_SCORE = 100;

interface SchemeItemRow {
  name: string;
  maxScore: string;
  deduction: string;
}

interface SchemeFormValues {
  title: string;
  effectiveFrom: string;
  remark: string;
  items: SchemeItemRow[];
}

function emptyRow(): SchemeItemRow {
  return { name: '', maxScore: '', deduction: '' };
}

function emptyForm(): SchemeFormValues {
  return { title: '', effectiveFrom: '', remark: '', items: [emptyRow(), emptyRow(), emptyRow()] };
}

/** 以某个历史版本为基础预填表单，便于在其上微调后发布新版本。 */
function formFromScheme(scheme: ScoreScheme): SchemeFormValues {
  return {
    title: `${scheme.title}（修订）`,
    effectiveFrom: '',
    remark: '',
    items: scheme.items.map((item) => ({
      name: item.name,
      maxScore: String(item.maxScore),
      deduction: item.deduction
    }))
  };
}

function validate(values: SchemeFormValues, latestEffectiveFrom: string | null): FormErrors<SchemeFormValues> {
  const errors: FormErrors<SchemeFormValues> = {};
  if (!values.title.trim()) {
    errors.title = '方案名称不能为空';
  }
  if (!values.effectiveFrom) {
    errors.effectiveFrom = '生效日期不能为空';
  } else if (!isDateString(values.effectiveFrom)) {
    errors.effectiveFrom = '生效日期格式应为 YYYY-MM-DD';
  } else if (latestEffectiveFrom && values.effectiveFrom <= latestEffectiveFrom) {
    errors.effectiveFrom = `生效日期必须晚于现行方案的生效日期（${latestEffectiveFrom}）`;
  }

  if (values.items.length === 0) {
    errors.items = '至少需要一个评分项';
    return errors;
  }
  const names = new Set<string>();
  let total = 0;
  for (const row of values.items) {
    const name = row.name.trim();
    if (!name) {
      errors.items = '评分项名称不能为空';
      return errors;
    }
    if (names.has(name)) {
      errors.items = `评分项「${name}」重复，请合并或改名`;
      return errors;
    }
    names.add(name);
    const maxScore = Number(row.maxScore);
    if (row.maxScore === '' || !Number.isInteger(maxScore) || maxScore <= 0 || maxScore > SCHEME_TOTAL_SCORE) {
      errors.items = `评分项「${name}」的分值上限需为 1 ~ ${SCHEME_TOTAL_SCORE} 之间的整数`;
      return errors;
    }
    total += maxScore;
  }
  if (total !== SCHEME_TOTAL_SCORE) {
    errors.items = `各评分项分值上限之和必须等于 ${SCHEME_TOTAL_SCORE} 分，当前合计 ${total} 分`;
  }
  return errors;
}

/** 版本状态：生效日期晚于今天为待生效，其余最新一版为现行，更早的为历史版本。 */
function versionTag(scheme: ScoreScheme, currentId: number) {
  if (scheme.effectiveFrom && scheme.effectiveFrom > today()) {
    return <span className="tag tag-info">待生效</span>;
  }
  if (scheme.id === currentId) {
    return <span className="tag tag-success">现行</span>;
  }
  return <span className="tag tag-muted">历史版本</span>;
}

export function ScoreSchemesPage() {
  const navigate = useNavigate();
  const toast = useToast();
  const schemes = useAsync(() => acceptanceApi.schemes(), []);
  const form = useForm<SchemeFormValues>(emptyForm());

  const versions = schemes.data ?? [];
  // 现行版本：生效日期不晚于今天的最新一版（列表按生效日期倒序）。
  const currentVersion = versions.find((item) => item.effectiveFrom && item.effectiveFrom <= today()) ?? null;
  const latestEffectiveFrom = versions[0]?.effectiveFrom ?? null;

  const totalMax = form.values.items.reduce((sum, row) => {
    const parsed = Number(row.maxScore);
    return sum + (Number.isFinite(parsed) ? parsed : 0);
  }, 0);

  const setItem = (index: number, patch: Partial<SchemeItemRow>) => {
    form.setValue(
      'items',
      form.values.items.map((row, i) => (i === index ? { ...row, ...patch } : row))
    );
  };
  const addItem = () => form.setValue('items', [...form.values.items, emptyRow()]);
  const removeItem = (index: number) =>
    form.setValue('items', form.values.items.filter((_, i) => i !== index));

  const submit = () => {
    void form.handleSubmit(async () => {
      await acceptanceApi.createScheme({
        title: form.values.title.trim(),
        effectiveFrom: form.values.effectiveFrom,
        remark: form.values.remark.trim(),
        items: form.values.items.map((row) => ({
          name: row.name.trim(),
          maxScore: Number(row.maxScore),
          deduction: row.deduction.trim()
        }))
      });
      toast.success('评分方案已保存，自生效日期起用于新登记的验收');
      form.reset(emptyForm());
      schemes.reload();
    }, (values) => validate(values, latestEffectiveFrom));
  };

  const columns: Column<ScoreScheme>[] = [
    {
      key: 'title',
      title: '方案名称',
      width: '220px',
      render: (row) => (
        <>
          <span className="cell-main">{row.title}</span>
          <span className="cell-sub">登记于 {formatDateTime(row.createdAt)}</span>
        </>
      )
    },
    {
      key: 'effectiveFrom',
      title: '生效日期',
      width: '110px',
      render: (row) => formatDate(row.effectiveFrom)
    },
    {
      key: 'items',
      title: '评分项（分值上限）',
      render: (row) => (
        <>
          <span>{row.items.map((item) => `${item.name} ${item.maxScore} 分`).join(' · ')}</span>
          <span className="cell-sub">
            {row.items
              .filter((item) => item.deduction)
              .map((item) => `${item.name}：${item.deduction}`)
              .join('；') || '—'}
          </span>
        </>
      )
    },
    {
      key: 'status',
      title: '状态',
      width: '90px',
      render: (row) => versionTag(row, currentVersion?.id ?? 0)
    },
    {
      key: 'actions',
      title: '操作',
      width: '130px',
      render: (row) => (
        <div className="row-actions">
          <button type="button" className="btn-link" onClick={() => form.reset(formFromScheme(row))}>
            以此为基础调整
          </button>
        </div>
      )
    }
  ];

  return (
    <div className="page">
      <PageHeader
        title="评分项设置"
        description="评分项按方案版本管理，每次调整生成新版本并指定生效日期；历史验收记录的总分与明细保持登记时的样子，不受调整影响。"
        actions={
          <button type="button" className="btn btn-ghost" onClick={() => navigate('/acceptances')}>
            返回验收记录
          </button>
        }
      />

      <SectionCard
        title="方案版本"
        subtitle={currentVersion ? `现行方案：${currentVersion.title}（${formatDate(currentVersion.effectiveFrom)} 起生效）` : '尚未配置评分方案'}
      >
        <div className="card-body-flush">
          <DataTable
            columns={columns}
            rows={versions}
            rowKey={(row) => row.id}
            loading={schemes.loading}
            error={schemes.error}
            onRetry={schemes.reload}
            emptyText="尚未配置评分方案"
            emptyDescription="在下方新增首版评分方案后，生效日期起登记的验收将按评分项逐项打分。"
          />
        </div>
      </SectionCard>

      <SectionCard
        title="调整评分项"
        subtitle={`新增方案版本，自生效日期起用于新登记的验收；各评分项分值上限之和须等于 ${SCHEME_TOTAL_SCORE} 分`}
      >
        {form.serverError ? (
          <div className="alert alert-error">
            <p>{form.serverError}</p>
          </div>
        ) : null}
        <div className="form-grid">
          <FormField label="方案名称" required error={form.errors.title}>
            <input
              className="input"
              placeholder="例如：验收评分标准（2026 修订版）"
              value={form.values.title}
              onChange={(event) => form.setValue('title', event.target.value)}
            />
          </FormField>
          <FormField
            label="生效日期"
            required
            hint={latestEffectiveFrom ? `须晚于现行方案生效日期（${latestEffectiveFrom}）` : '首个版本可任意指定'}
            error={form.errors.effectiveFrom}
          >
            <input
              className="input"
              type="date"
              value={form.values.effectiveFrom}
              onChange={(event) => form.setValue('effectiveFrom', event.target.value)}
            />
          </FormField>
          <FormField label="备注" error={form.errors.remark}>
            <input
              className="input"
              placeholder="本次调整说明，可不填"
              value={form.values.remark}
              onChange={(event) => form.setValue('remark', event.target.value)}
            />
          </FormField>
        </div>

        <div className="form-field">
          <span className="form-label">
            评分项
            <em className="form-required">*</em>
          </span>
          <div className="scheme-item-head">
            <span>评分项名称</span>
            <span>分值上限</span>
            <span>扣分说明</span>
            <span />
          </div>
          {form.values.items.map((row, index) => (
            <div key={index} className="scheme-item-row">
              <input
                className="input"
                placeholder="例如：清淤洁净度"
                value={row.name}
                onChange={(event) => setItem(index, { name: event.target.value })}
              />
              <input
                className="input"
                inputMode="numeric"
                placeholder="如 40"
                value={row.maxScore}
                onChange={(event) => setItem(index, { maxScore: event.target.value })}
              />
              <input
                className="input"
                placeholder="例如：残留淤积厚度每超 5mm 扣 5 分"
                value={row.deduction}
                onChange={(event) => setItem(index, { deduction: event.target.value })}
              />
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                disabled={form.values.items.length <= 1}
                onClick={() => removeItem(index)}
              >
                删除
              </button>
            </div>
          ))}
          <div className="scheme-item-footer">
            <button type="button" className="btn btn-ghost btn-sm" onClick={addItem}>
              + 添加评分项
            </button>
            <span className={totalMax === SCHEME_TOTAL_SCORE ? 'scheme-total scheme-total-ok' : 'scheme-total'}>
              分值上限合计：{totalMax} / {SCHEME_TOTAL_SCORE} 分
            </span>
          </div>
          {form.errors.items ? <span className="form-error">{form.errors.items}</span> : null}
        </div>

        <div className="form-actions">
          <button type="button" className="btn btn-ghost" onClick={() => form.reset(emptyForm())}>
            清空
          </button>
          <button type="button" className="btn btn-primary" disabled={form.submitting} onClick={submit}>
            {form.submitting ? '保存中…' : '保存新版本'}
          </button>
        </div>
      </SectionCard>
    </div>
  );
}
