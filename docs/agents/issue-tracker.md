# 任务跟踪器：GitHub Issues

本项目的任务、缺陷与需求规格统一记录在 GitHub Issues 中，所有操作均通过 `gh` CLI 执行。

## 操作约定

- **创建 Issue**：`gh issue create --title "..." --body "..."`。多行内容建议使用标准输入（heredoc）。
- **查看 Issue**：`gh issue view <number> --comments`，可通过 `jq` 过滤评论并获取标签。
- **列出 Issues**：`gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'`，按需配合 `--label` 与 `--state` 过滤。
- **发表评论**：`gh issue comment <number> --body "..."`
- **添加/移除标签**：`gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **关闭 Issue**：`gh issue close <number> --comment "..."`

仓库上下文自动根据本地 `git remote -v` 推断（`gh` 在本地克隆仓库中运行时会自动识别）。

## Pull Request 作为分类面 (Triage Surface)

**是否将 PR 视作需求请求面：否 (PRs as a request surface: no)** _（若本项目将外部 PR 视作特性需求请求，可改为 `yes`；`/triage` 技能会读取此标识）_

当设置为 `yes` 时，PR 将使用与 Issue 相同的标签体系与状态流转，对应采用 `gh pr` 命令：
- **查看 PR**：`gh pr view <number> --comments`，并通过 `gh pr diff <number>` 查看差异。
- **列出待分类外部 PR**：`gh pr list --state open --json number,title,body,labels,author,authorAssociation,comments`，仅保留 `authorAssociation` 为 `CONTRIBUTOR`、`FIRST_TIME_CONTRIBUTOR` 或 `NONE` 的条目（排除 `OWNER` / `MEMBER` / `COLLABORATOR`）。
- **评论 / 标签 / 关闭**：`gh pr comment`、`gh pr edit --add-label`/`--remove-label`、`gh pr close`。

由于 GitHub 在 Issue 与 PR 之间共享同一编号空间，单独的 `#42` 可能是 Issue 或 PR —— 优先使用 `gh pr view 42` 查询，若失败则降级使用 `gh issue view 42`。

## 当技能提示“发布至任务跟踪器 (publish to the issue tracker)”

创建对应的 GitHub Issue。

## 当技能提示“获取关联工单 (fetch the relevant ticket)”

执行 `gh issue view <number> --comments`。

## 寻路导航操作 (Wayfinding operations)

供 `/wayfinder` 技能使用。**导航图谱 (Map)** 为一个主 Issue，其下级关联任务为 **子工单 (Child issues)**。

- **导航图谱 (Map)**：带有 `wayfinder:map` 标签的独立 Issue，内容包含备注（Notes）、既定决策（Decisions-so-far）与未知迷雾区（Fog）。命令：`gh issue create --label wayfinder:map`。
- **子工单 (Child ticket)**：通过 GitHub 子任务（sub-issue）机制关联到主图谱（通过 `gh api` 调用 sub-issues 端点）。若未开启子任务特性，则在主图谱正文的任务列表中列出该子工单，并在子工单顶部标注 `Part of #<map>`。标签格式为 `wayfinder:<type>`（可选值：`research` / `prototype` / `grilling` / `task`）。领取后分配给对应负责人。
- **依赖阻塞 (Blocking)**：采用 GitHub 原生 Issue 依赖机制。通过 `gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>` 添加阻塞边，其中 `<blocker-db-id>` 为阻塞者的数字数据库 ID（通过 `gh api repos/<owner>/<repo>/issues/<n> --jq .id` 获取，而非编号 `#number`）。GitHub 返回 `issue_dependencies_summary.blocked_by`。若原生依赖不可用，降级在子工单正文顶部标注 `Blocked by: #<n>, #<n>`。当所有前置阻塞 Issue 均关闭时，该工单自动解除阻塞。
- **前沿就绪查询 (Frontier query)**：列出主图谱的所有开放子工单，剔除存在未解阻塞或已分配负责人的工单，按图谱顺序优先取第一个。
- **认领任务 (Claim)**：`gh issue edit <n> --add-assignee @me`。
- **任务办结 (Resolve)**：`gh issue comment <n> --body "<answer>"`，然后执行 `gh issue close <n>`，并在主图谱的既定决策（Decisions-so-far）中追加上下文说明指针。
