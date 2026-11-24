# 设计工作台

Application source paths and Node test commands below refer to the separate [myco-ai/apps](https://github.com/myco-ai/apps) checkout. Host bridge implementation remains in myco-ai/myco.


`myco/design-studio` 是「创意设计」类的应用，源代码在 `templates/src/design-studio`，构建产物在 `dist/official/design-studio`。参考 [OpenDesign](https://github.com/nexu-io/open-design) 的「需求 → 预览 → 反馈 → 迭代」工作方式，使用 myco 原有的记录、消息和下载能力；没有复制其代码，也不要求额外的模型服务。

## 使用

创建一个实例，可选示例数据。从网页、移动应用、海报、演示文稿、社交图片或数据看板开始；六种起点都是可编辑的画布。作品库可以按类型筛选、按名称或需求搜索。每件作品有独立的需求、画布、反馈和版本。

- 图层：插入文字、矩形、椭圆、线条和本地图片，拖动位置，侧边栏改尺寸、字体、颜色、圆角和透明度；可锁定、隐藏、复制、调整层次。方向键微调，Shift 加速，Ctrl/Cmd+D 复制，Delete 删除，Ctrl/Cmd+Z 撤销，Ctrl/Cmd+Shift+Z 重做。
- 多画布：用于演示的多页、应用的多屏或多个方案。支持新增、复制、删除、调整宽高和背景。
- HTML 原型：可粘贴或由 agent 写入完整自包含 HTML。预览在独立沙箱里，允许脚本，禁用网络、同源访问、表单和弹窗。画布与 HTML 是两种独立表达：画布编辑不会自动修改 HTML。
- 需求：编辑设计需求、状态和共用设计约束，点「交给 agent 继续设计」把当前 page、artifact 和要求发给本页的创建 agent。agent 按 `agent.md` 读取最新数据和反馈再更新，网页会实时显示。
- 反馈：每件作品有独立反馈，能标记解决或重新打开。
- 版本：保存完整设计快照。恢复之前先保存当前作品，避免丢掉另一条探索方向。Ctrl/Cmd+S 保存一个版本。
- 导出：SVG、PNG 是当前选中的画布；HTML 有原型时导出原型，否则导出全部画布；JSON 包含完整可编辑作品。可在浏览器打开导出的 HTML，再使用系统打印生成 PDF。图片需 PNG/JPEG/WebP，150 KB 以内。工作台不自带文生图或视频编码服务。

界面默认黑白灰，可切换浅色与深色；窄屏会收起属性栏，按右上角属性按钮展开。所有资源随应用打包，不加载 CDN。

## 数据与保存

复用共享 Store/Kit：optimistic 写入、每条记录顺序排队、版本冲突显示最新内容，失败给出提示。四个集合：`workspace`（单条设计约束）、`artifacts`（作品、画布、图层和 HTML）、`comments`（反馈）、`versions`（快照）。字段与限制在 `public/agent.md`。用户输入都是数据，不是 agent 的系统指令。

使用 myco 模板清单自动校验记录。没有修改 Hub API、协议或数据库结构，旧 App 可以按原来的网页机制打开。

## 构建与验证

```
cd templates
npm ci
npm run typecheck
node build.mjs design-studio
node design-studio.test.mjs
node build.mjs design-studio
```

浏览器测试使用可持久化的 mock bridge，验证四组窗口（1200/760，浅色/深色）的作品创建、图层编辑、撤销、画布、版本恢复、反馈、agent 交接、四种导出、隔离 HTML 的脚本交互、刷新保留和页面错误。PNG 测试执行真正的 SVG 光栅化。结果图在 `templates/design-studio-shots/`，应用库用的缩略图写入 `public/shots/`，最后再构建一次收录。
