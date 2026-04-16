# 夜间快讯值班系统2 数据模型

## 1. Source

```json
{
  "id": "src-001",
  "title": "深夜政策快讯",
  "summary": "核心来源摘要",
  "source_url": "",
  "source_name": "某主流来源",
  "credibility": "high",
  "status": "needs_review",
  "tags": ["policy", "urgent"],
  "created_at": "2026-04-13T22:10:00Z",
  "updated_at": "2026-04-13T22:15:00Z"
}
```

### 约束

- `credibility` 只能取：
  - `high`
  - `medium`
  - `low`

## 2. Review

```json
{
  "id": "review-001",
  "source_id": "src-001",
  "kind": "risk",
  "status": "open",
  "actor": "night-reviewer",
  "title": "来源仍需补核",
  "body": "二级来源已转述，但一手确认还不够。",
  "created_at": "2026-04-13T22:20:00Z"
}
```

## 3. Decision

```json
{
  "id": "decision-001",
  "source_id": "src-001",
  "outcome": "publish_now",
  "title": "先发已核实部分",
  "body": "先发已核实结论，争议点留到早班补核。",
  "created_at": "2026-04-13T22:30:00Z"
}
```

## 4. Brief

```json
{
  "id": "brief-001",
  "title": "夜间快讯简报",
  "status": "published",
  "items": ["src-001"],
  "summary": "夜间已发内容和待确认内容汇总",
  "created_at": "2026-04-13T23:10:00Z"
}
```

### `Brief.status`

- `draft`
- `published`

## 5. Incident

```json
{
  "id": "incident-001",
  "stage": "incident",
  "severity": "high",
  "title": "发布接口超时",
  "body": "主推送接口 5 分钟内连续超时",
  "created_at": "2026-04-13T23:20:00Z"
}
```

## 6. Handoff

```json
{
  "id": "handoff-001",
  "stage": "handoff",
  "title": "交接给早班",
  "body": "待确认来源 2 条，故障已恢复待复盘。",
  "created_at": "2026-04-14T06:40:00Z"
}
```

## 7. Task

```json
{
  "id": "task-001",
  "title": "补核深夜政策快讯",
  "status": "doing",
  "source_id": "src-001",
  "decision_id": "",
  "incident_id": "",
  "handoff_id": "",
  "owner": "night-editor",
  "created_at": "2026-04-13T22:16:00Z",
  "updated_at": "2026-04-13T22:25:00Z"
}
```

## 8. ArchiveItem

```json
{
  "id": "archive-001",
  "kind": "decision-note",
  "title": "夜间终审口径",
  "body": "终审结论正文",
  "related_ids": ["src-001", "decision-001"],
  "created_at": "2026-04-13T22:35:00Z"
}
```

### `ArchiveItem.kind`

- `decision-note`
- `brief`
- `incident-summary`
- `handoff-summary`

## 9. 最近动态

需要一个统一事件流：

```json
{
  "id": "event-001",
  "scope": "incident",
  "action": "recovery",
  "title": "发布链路恢复",
  "detail": "恢复后重新补发 2 条快讯",
  "created_at": "2026-04-13T23:40:00Z"
}
```

## 10. 持久化要求

默认允许两种实现：

- 单个 JSON 文件
- SQLite

但无论使用哪种方式，最终必须能稳定保留：

- 所有来源
- 所有复核
- 所有决策
- 所有事故
- 所有交接
- 所有任务
- 所有归档
- 所有历史事件

