# 会议纪要整理与行动项生成台 规格共创样本 规格包导出

- team_id: `meeting-notes-spec1`
- profile: `spec-package`
- generated_at: `2026-04-13T07:24:13Z`
- document_count: `7`
- supporting_count: `4`

## 正文规格

### README Spec

- artifact_id: `readme-spec`
- section: `readme`
- channel_id: `artifacts`
- kind: `markdown`

规格包入口与边界

# 会议纪要整理与行动项生成台

这是一套独立于 Team 的本地会议纪要整理与行动项生成系统规格包。

目标是让任何大模型都能根据这组规格独立实现本地可运行程序。

### Product Spec

- artifact_id: `product-spec`
- section: `product`
- channel_id: `artifacts`
- kind: `markdown`

产品目标与非目标

# Product

目标：导入会议文本或转写稿，生成结构化纪要、决议和行动项。

非目标：不做复杂企业权限、不做通用知识库、不追求完全免人工校对。

核心承诺：本地优先、可编辑、可回溯、可导出。

### Workflow Spec

- artifact_id: `workflow-spec`
- section: `workflows`
- channel_id: `artifacts`
- kind: `markdown`

核心工作流

# Workflows

1. 新建会议并导入会议文本或转写稿。
2. 系统生成纪要初稿、议题分段、决议草稿和行动项草稿。
3. 用户逐条校对并补全责任人、截止时间、优先级。
4. 发布最终会议纪要。
5. 将 ActionItem 进入跟踪列表。
6. 导出 Markdown 和 JSON，并在归档中回看原文与终稿。

### Data Model Spec

- artifact_id: `data-model-spec`
- section: `data-model`
- channel_id: `artifacts`
- kind: `markdown`

数据模型

# Data Model

- Meeting: 标题、时间、参与人、来源、状态
- Topic: 议题标题、摘要、来源片段
- Decision: 决议内容、结论、来源片段
- ActionItem: 内容、负责人、截止时间、优先级、状态、来源片段
- Revision: 修改人、修改时间、差异摘要

ActionItem 状态机：open -> confirmed -> done / dropped。

### Screens And Interactions Spec

- artifact_id: `screens-spec`
- section: `screens-and-interactions`
- channel_id: `artifacts`
- kind: `markdown`

页面与交互

# Screens And Interactions

- 会议列表页
- 会议详情页
- 纪要校对页
- 行动项跟踪页
- 导出与归档页

关键交互：导入文本、重新抽取、逐条编辑、确认发布、导出 Markdown/JSON、按责任人筛选行动项。

### API And Runtime Spec

- artifact_id: `api-runtime-spec`
- section: `api-and-runtime`
- channel_id: `artifacts`
- kind: `markdown`

运行时与接口

# API And Runtime

- 单机本地 Web 程序
- 文件持久化
- 本地 JSON API
- Markdown + JSON 双导出
- 默认人工校对后才能发布终稿

最低运行单元：一个可启动进程 + 一个状态目录。

### Verification Spec

- artifact_id: `verification-spec`
- section: `verification`
- channel_id: `artifacts`
- kind: `markdown`

验证与验收

# Verification

最小验收集：
1. 导入 1 份会议文本
2. 生成 1 份可编辑纪要
3. 至少产出 3 条行动项，包含责任人、截止时间、状态
4. 修改后保留版本痕迹
5. 重启后数据不丢
6. 可按会议名/责任人检索
7. 可导出 Markdown，并回看原文与终稿

## 支撑产物

### 会议纪要系统规格包结构与交付要求

- artifact_id: `meeting-notes-spec1:agent://spec/editor:2026-04-13T07:24:13.247503Z:会议纪要系统规格包结构与交付要求`
- section: `supporting`
- channel_id: `main`
- kind: `skill-doc`

规格包结构与交付要求

{
  "kind": "skill",
  "notes": [
    "目标：面向本地会议，把录音/文字纪要快速整理成结构化摘要、决策点、行动项和待办追踪，支持会后即时分发与回收确认，降低人工整理成本。非目标：不做通用知识库、不替代完整项目管理系统、不追求跨组织复杂权限与深度协作，也不承诺对所有口语噪声、多人插话做完全准确的语义还原。\n\n核心工作流：用户新建会议后导入录音或粘贴纪要，系统先做转写/清洗，再生成议程回顾、主题分段、关键结论与行动项草稿；用户可逐条校对、补充负责人、截止时间和优先级；确认后生成可分享的会议总结页，并把行动项进入跟踪列表，支持后续按人、按会议、按状态检索与回看。\n\n数据对象：Meeting、Transcript、Summary、ActionItem、Review；所有对象都要保留来源引用，保证回溯与纠错。\n\n风险与边界：优先保证本地可用、隐私可控、离线容错，再考虑多端同步、复杂权限和深度日程联动。"
  ],
  "steps": [
    "冻结 product",
    "冻结 workflows",
    "冻结 data model",
    "冻结 api/runtime",
    "冻结 verification",
    "导出 spec package"
  ],
  "summary": "README + 6 份主规格 + verification，并可直接导出给下游实现。",
  "title": "会议纪要系统规格包结构与交付要求"
}

### 会议纪要边界评审结论

- artifact_id: `meeting-notes-spec1:agent://spec/editor:2026-04-13T07:24:13.250017Z:会议纪要边界评审结论`
- section: `supporting`
- channel_id: `reviews`
- kind: `review-summary`

接受 reviewer 建议，先冻结输入边界、行动项状态机和导出格式。

{
  "decision": "进入 decision-room 冻结运行边界。",
  "kind": "decision",
  "next_steps": [
    "冻结输入范围",
    "冻结数据模型",
    "进入 artifacts 正文编写"
  ],
  "summary": "先冻结输入边界、ActionItem 状态机和 Markdown/JSON 导出。",
  "title": "会议纪要边界评审结论"
}

### 会议纪要运行边界冻结

- artifact_id: `meeting-notes-spec1:agent://spec/editor:2026-04-13T07:24:13.253183Z:会议纪要运行边界冻结`
- section: `supporting`
- channel_id: `decisions`
- kind: `decision-note`

决定采用四层建模、本地单机优先、所有结论必须保留来源片段。

{
  "followups": [
    "写 Product Spec",
    "写 Data Model Spec",
    "写 Verification Spec"
  ],
  "kind": "decision",
  "outcome": "Meeting/Topic/Decision/ActionItem 四层对象 + Markdown/JSON 双导出。",
  "summary": "四层建模、本地优先、人工校对后发布。",
  "title": "会议纪要运行边界冻结"
}

### 会议纪要规格包正文已发布

- artifact_id: `meeting-notes-spec1:agent://spec/editor:2026-04-13T07:24:13.25633Z:会议纪要规格包正文已发布`
- section: `supporting`
- channel_id: `artifacts`
- kind: `artifact-brief`

第一版规格正文已经可以直接交给下游模型实现。

{
  "artifact_kind": "spec-package",
  "followups": [
    "导出规格包",
    "走最小验收",
    "冻结 spec-package-ready"
  ],
  "kind": "publish",
  "result": "进入 export 阶段",
  "summary": "正文规格已齐，可导出 Markdown/JSON。",
  "title": "会议纪要规格包正文已发布"
}
