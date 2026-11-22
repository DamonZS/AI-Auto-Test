<script setup>
defineProps({ value: { type: String, default: '' } })

const LABELS = {
  draft: '草稿', in_review: '待审核', approved: '已定稿', deprecated: '已废弃',
  queued: '排队中', running: '执行中', passed: '通过', failed: '失败', error: '异常',
  new: '新建', reopened: '重新打开', confirmed: '已确认', fixing: '修复中',
  fixed: '已修复', verified: '已验证', closed: '已关闭', rejected: '已拒绝',
  pending: '待审批', drift: '脚本漂移',
}
const TONES = {
  passed: 'ok', approved: 'ok', closed: 'ok', verified: 'ok',
  failed: 'bad', error: 'bad', reopened: 'bad',
  running: 'run', in_review: 'wait', pending: 'wait', fixed: 'wait',
}
</script>

<template>
  <span class="tag" :class="TONES[value] || ''">{{ LABELS[value] || value || '—' }}</span>
</template>

<style scoped>
.tag {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 12px;
  background: #f1f1f4;
  color: var(--muted);
  white-space: nowrap;
}
.tag.ok { background: #e8f5ee; color: var(--ok); }
.tag.bad { background: #fdecea; color: var(--bad); }
.tag.run { background: #eef1fe; color: var(--accent); }
.tag.wait { background: #fff6e5; color: #8a5a00; }
</style>
