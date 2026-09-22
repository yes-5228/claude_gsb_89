// 验收评分项设置：评分项调整通过"发布新版本 + 生效日期"完成，历史版本不可修改。
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { acceptanceApi } from '../../api/acceptances';
import { FormField } from '../../components/FormField';
import { Modal } from '../../components/Modal';
import { PageHeader } from '../../components/PageHeader';
import { SectionCard } from '../../components/SectionCard';
import { useToast } from '../../components/Toast';
import { useAsync } from '../../hooks/useAsync';
import { useForm } from '../../hooks/useForm';
import type { ScoreTemplate } from '../../types/domain';
import { isDateString, today } from '../../utils/format';

interface NewVersionValues {
  effectiveFrom: string;
  remark: string;
}

interface ItemDraft {
  name: string;
  maxScore: string;
  criteria: string;
  sortOrder: string;
}

function emptyDrafts(): ItemDraft[] {
  return [
    { name: '', maxScore: '', criteria: '', sortOrder: '1' },
    { name: '', maxScore: '', criteria: '', sortOrder: '2' }
  ];
}

function draftTotal(drafts: ItemDraft[]): number | null {
  let total = 0;
  for (const draft of drafts) {
    const score = Number(draft.maxScore);
    if (draft.maxScore === '' || !Number.isInteger(score) || score < 1) {
      return null;
    }
    total += score;
  }
  return total;
}

export function ScoreTemplateSettingsPage() {
  const navigate = useNavigate();
  const toast = useToast();
  const list = useAsync(() => acceptanceApi.listTemplates(), []);
  const [modalOpen, setModalOpen] = useState(false);
  const [drafts, setDrafts] = useState<ItemDraft[]>(emptyDrafts());
  const [busy, setBusy] = useState(false);

  const form = useForm<NewVersionValues>({ effectiveFrom: today(), remark: '' });

  const total = draftTotal(drafts);

  const openModal = () => {
    form.reset({ effectiveFrom: today(), remark: '' });
    setDrafts(emptyDrafts());
    setModalOpen(true);
  };

  const updateDraft = (index: number, patch: Partial<ItemDraft>) => {
    setDrafts((prev) => prev.map((draft, i) => (i === index ? { ...draft, ...patch } : draft)));
  };

  const submit = async () => {
    const errors = validate(form.values, drafts);
    if (Object.keys(errors).length > 0) {
      toast.error(Object.values(errors)[0]);
      return;
    }
    setBusy(true);
    try {
      await acceptanceApi.createTemplate({
        effectiveFrom: form.values.effectiveFrom,
        remark: form.values.remark.trim(),
        items: drafts.map((draft) => ({
          name: draft.name.trim(),
          maxScore: Number(draft.maxScore),
          sortOrder: Number(draft.sortOrder),
          criteria: draft.criteria.trim()
        }))
      });
      toast.success('新评分模板版本已发布，生效日期之后登记的验收将使用该版本');
      setModalOpen(false);
      list.reload();
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : '发布失败');
    } finally {
      setBusy(false);
    }
  };

  const versions = list.data ?? [];

  return (
    <div className="page">
      <PageHeader
        title="验收评分项设置"
        description="评分项按版本管理，调整评分项即发布带生效日期的新版本；历史版本与已登记验收的评分快照均不可修改。"
        actions={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => navigate('/acceptances')}>
              返回验收记录
            </button>
            <button type="button" className="btn btn-primary" onClick={openModal}>
              发布新版本
            </button>
          </>
        }
      />

      {versions.map((version: ScoreTemplate) => (
        <SectionCard
          key={version.id}
          title={`版本 #${version.id} · 生效日期 ${version.effectiveFrom}`}
          subtitle={version.remark || '—'}
          extra={
            version.active ? <span className="tag tag-success">当前生效</span> : <span className="tag tag-muted">历史版本</span>
          }
        >
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th style={{ width: 60 }}>序号</th>
                  <th>评分项</th>
                  <th>评分标准</th>
                  <th style={{ width: 120, textAlign: 'right' }}>分值上限</th>
                </tr>
              </thead>
              <tbody>
                {version.items.map((item) => (
                  <tr key={`${version.id}-${item.sortOrder}`}>
                    <td>{item.sortOrder}</td>
                    <td>
                      <span className="cell-main">{item.name}</span>
                    </td>
                    <td>{item.criteria || '—'}</td>
                    <td style={{ textAlign: 'right' }}>
                      <span className="cell-num">{item.maxScore} 分</span>
                    </td>
                  </tr>
                ))}
              </tbody>
              <tfoot>
                <tr>
                  <td colSpan={3}>合计</td>
                  <td style={{ textAlign: 'right' }} className="cell-num">
                    100 分
                  </td>
                </tr>
              </tfoot>
            </table>
          </div>
        </SectionCard>
      ))}

      {!list.loading && versions.length === 0 && !list.error ? (
        <div className="alert alert-warn">
          <p>暂无评分模板版本，请发布第一个版本。</p>
        </div>
      ) : null}

      <Modal
        open={modalOpen}
        title="发布新评分模板版本"
        onClose={() => setModalOpen(false)}
        width={780}
        footer={
          <>
            <button type="button" className="btn btn-ghost" onClick={() => setModalOpen(false)}>
              取消
            </button>
            <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void submit()}>
              {busy ? '提交中…' : '发布新版本'}
            </button>
          </>
        }
      >
        <div className="form-grid">
          <FormField label="生效日期" required hint="该日期起登记的验收使用新版本；同日期不能重复发布">
            <input
              className="input"
              type="date"
              value={form.values.effectiveFrom}
              onChange={(event) => form.setValue('effectiveFrom', event.target.value)}
            />
          </FormField>
          <FormField label="版本备注" span={2}>
            <input
              className="input"
              placeholder="例如：2026 年汛后调整残留淤积项权重"
              value={form.values.remark}
              onChange={(event) => form.setValue('remark', event.target.value)}
            />
          </FormField>
        </div>

        <div style={{ height: 16 }} />
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th style={{ width: 60 }}>序号</th>
                <th>评分项名称</th>
                <th>评分标准</th>
                <th style={{ width: 120 }}>分值上限</th>
                <th style={{ width: 70 }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {drafts.map((draft, index) => (
                <tr key={index}>
                  <td>
                    <input
                      className="input"
                      value={draft.sortOrder}
                      onChange={(event) => updateDraft(index, { sortOrder: event.target.value })}
                    />
                  </td>
                  <td>
                    <input
                      className="input"
                      value={draft.name}
                      placeholder="例如：管道淤积清理"
                      onChange={(event) => updateDraft(index, { name: event.target.value })}
                    />
                  </td>
                  <td>
                    <input
                      className="input"
                      value={draft.criteria}
                      placeholder="评分标准说明（可选）"
                      onChange={(event) => updateDraft(index, { criteria: event.target.value })}
                    />
                  </td>
                  <td>
                    <input
                      className="input"
                      inputMode="numeric"
                      value={draft.maxScore}
                      onChange={(event) => updateDraft(index, { maxScore: event.target.value })}
                    />
                  </td>
                  <td>
                    <button
                      type="button"
                      className="btn btn-ghost btn-sm"
                      disabled={drafts.length <= 1}
                      onClick={() => setDrafts((prev) => prev.filter((_, i) => i !== index))}
                    >
                      删除
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="form-actions">
          <button
            type="button"
            className="btn btn-ghost"
            disabled={drafts.length >= 20}
            onClick={() =>
              setDrafts((prev) => [
                ...prev,
                { name: '', maxScore: '', criteria: '', sortOrder: String(prev.length + 1) }
              ])
            }
          >
            添加评分项
          </button>
          <span className={total === 100 ? 'tag tag-success' : 'tag tag-danger'}>
            分值上限合计：{total ?? '—'} / 100
          </span>
        </div>
      </Modal>
    </div>
  );
}

function validate(values: NewVersionValues, drafts: ItemDraft[]): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!values.effectiveFrom || !isDateString(values.effectiveFrom)) {
    errors.effectiveFrom = '生效日期格式应为 YYYY-MM-DD';
  }
  if (drafts.length === 0) {
    errors.items = '至少需要一个评分项';
    return errors;
  }
  const names = new Set<string>();
  const orders = new Set<number>();
  let sum = 0;
  for (const draft of drafts) {
    const name = draft.name.trim();
    if (!name) {
      errors.items = '评分项名称不能为空';
      return errors;
    }
    if (names.has(name)) {
      errors.items = `评分项名称不能重复：${name}`;
      return errors;
    }
    names.add(name);
    const order = Number(draft.sortOrder);
    if (!Number.isInteger(order) || order < 1) {
      errors.items = `评分项「${name}」的序号需为正整数`;
      return errors;
    }
    if (orders.has(order)) {
      errors.items = `评分项序号不能重复：${order}`;
      return errors;
    }
    orders.add(order);
    const score = Number(draft.maxScore);
    if (!Number.isInteger(score) || score < 1) {
      errors.items = `评分项「${name}」的分值上限需为正整数`;
      return errors;
    }
    sum += score;
  }
  if (sum !== 100) {
    errors.items = `各评分项分值上限合计必须为 100，当前为 ${sum}`;
  }
  return errors;
}
