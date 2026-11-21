import SwiftUI
import MycoAPI
import MycoCore
import MycoUI

/// 应用: tables, documents, boards and games people and agents use
/// together (the desktop app's Templates page). Tap one to make a page
/// from it, which opens; the pages made before are on top, to open again.
struct AppsScreen: View {
    @Environment(WorkspaceSession.self) private var session
    @Environment(Router.self) private var router

    @State private var shelf = "all"
    @State private var query = ""
    @State private var allMade = false
    @State private var using: AppTemplate?

    var body: some View {
        let model = session.apps
        Group {
            if model.unsupported {
                EmptyState(
                    symbol: "square.grid.2x2", title: L("这个工作区还不能用应用", "No apps in this workspace yet"),
                    note: L("它的版本太旧。在服务器上运行 sudo myco upgrade，升级到 v0.20.5 或更新。", "It is too old: run sudo myco upgrade on the server (v0.20.5 or later)."))
                    .accessibilityIdentifier("apps.unsupported")
            } else if model.list != nil {
                list(model)
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .background(MycoColor.bg)
        .navigationTitle(L("应用", "Apps"))
        .navigationBarTitleDisplayMode(.inline)
        .solidTopBar()
        .searchable(text: $query, placement: .navigationBarDrawer(displayMode: .automatic), prompt: L("找应用", "Find an app"))
        .task { await model.load() }
        .refreshable { await model.load() }
        .sheet(item: $using) { app in
            UseAppSheet(app: app) { page in
                using = nil
                // (once the sheet has gone: one cover at a time)
                Task {
                    try? await Task.sleep(for: .milliseconds(500))
                    router.page = PageRoute(id: page.id)
                }
            }
        }
    }

    private func list(_ model: AppsModel) -> some View {
        let official = model.official.filter { AppsModel.matches($0, query) }
        let own = model.own.filter { AppsModel.matches($0, query) }
        let shelves = model.shelvesInUse
        let made = allMade ? model.made : Array(model.made.prefix(5))
        return List {
            Section {
                Text(L("你和 agent 一起用的表格、文档、白板和小游戏，两边的改动实时同步。",
                       "Tables, documents, boards and games you and agents use together, changes shared as they happen."))
                    .font(.footnote).foregroundStyle(MycoColor.muted)
                    .fixedSize(horizontal: false, vertical: true)
                    .listRowInsets(EdgeInsets(top: 4, leading: Space.page, bottom: 4, trailing: Space.page))
                    .listRowBackground(MycoColor.bg)
                    .listRowSeparator(.hidden)
                if let failure = model.failure {
                    FailureLine(failure: failure, onRetry: { Task { await model.load() } })
                        .listRowInsets(EdgeInsets(top: 4, leading: Space.page, bottom: 4, trailing: Space.page))
                        .listRowBackground(MycoColor.bg)
                        .listRowSeparator(.hidden)
                }
            }
            if !model.made.isEmpty, query.isEmpty {
                Section {
                    ForEach(made) { page in
                        Button {
                            router.page = PageRoute(id: page.id)
                        } label: {
                            MadeRow(page: page, app: model.app(of: page), project: projectTitle(page.project))
                        }
                        .buttonStyle(RowButtonStyle())
                        .modifier(AppRowLook())
                        .accessibilityIdentifier("apps.made")
                    }
                } header: {
                    HStack {
                        Text(L("最近建的", "Made recently"))
                        Spacer()
                        if model.made.count > 5 {
                            Button(allMade ? L("收起", "Fewer") : L("全部 \(model.made.count) 个", "All \(model.made.count)")) {
                                withAnimation(.snappy(duration: 0.2)) { allMade.toggle() }
                            }
                            .font(.footnote).tint(MycoColor.accent)
                        }
                    }
                    .font(.footnote).foregroundStyle(MycoColor.muted)
                }
            }
            Section {
                chips(shelves)
                    .listRowInsets(EdgeInsets(top: 6, leading: 0, bottom: 6, trailing: 0))
                    .listRowBackground(MycoColor.bg)
                    .listRowSeparator(.hidden)
                if official.isEmpty, own.isEmpty, !query.isEmpty {
                    Text(L("没有找到「\(query)」", "Nothing matches \"\(query)\""))
                        .font(.footnote).foregroundStyle(MycoColor.muted)
                        .modifier(AppRowLook())
                }
            }
            if shelf != "own" {
                ForEach(shelves.filter { shelf == "all" || $0.key == shelf }, id: \.key) { one in
                    let on = official.filter { AppsModel.shelf(of: $0) == one.key }
                    if !on.isEmpty {
                        Section {
                            ForEach(on) { app in card(app) }
                        } header: {
                            Text(L(one.zh, one.en)).font(.footnote).foregroundStyle(MycoColor.muted)
                        }
                    }
                }
            }
            if shelf == "all" || shelf == "own" {
                Section {
                    if own.isEmpty {
                        if query.isEmpty {
                            Text(L("还没有自己的应用。想要一个（比如周报、面试评估表），跟 agent 说：「用 myco template init 帮我做一个周报应用，做好发布到工作区」。",
                                   "None of your own yet. For one (a weekly report, an interview sheet), ask an agent: \"make a weekly report app with myco template init and publish it\"."))
                                .font(.footnote).foregroundStyle(MycoColor.muted)
                                .fixedSize(horizontal: false, vertical: true)
                                .modifier(AppRowLook())
                                .accessibilityIdentifier("apps.own.empty")
                        }
                    } else {
                        ForEach(own) { app in card(app) }
                    }
                } header: {
                    Text(L("这个工作区的应用", "This workspace's apps")).font(.footnote).foregroundStyle(MycoColor.muted)
                }
            }
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .accessibilityIdentifier("apps.list")
    }

    private func card(_ app: AppTemplate) -> some View {
        Button {
            using = app
        } label: {
            AppRow(app: app)
        }
        .buttonStyle(RowButtonStyle())
        .modifier(AppRowLook())
        .accessibilityIdentifier("app." + app.slug)
    }

    /// The shelves as a row of chips: all, myco's shelves, the workspace's own.
    private func chips(_ shelves: [(key: String, zh: String, en: String)]) -> some View {
        let all = [("all", L("全部", "All"))] + shelves.map { ($0.key, L($0.zh, $0.en)) } + [("own", L("这个工作区的", "This workspace's"))]
        return ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(all, id: \.0) { key, title in
                    Button {
                        withAnimation(.snappy(duration: 0.2)) { shelf = key }
                    } label: {
                        Text(title)
                            .font(.subheadline.weight(shelf == key ? .semibold : .regular))
                            .foregroundStyle(shelf == key ? MycoColor.buttonText : MycoColor.fg2)
                            .padding(.horizontal, 14).frame(minHeight: 34)
                            .background(shelf == key ? MycoColor.button : MycoColor.pill, in: Capsule())
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(shelf == key ? .isSelected : [])
                    .accessibilityIdentifier("apps.shelf." + key)
                }
            }
            .padding(.horizontal, Space.page)
        }
    }

    private func projectTitle(_ name: String?) -> String? {
        guard let name, !name.isEmpty else { return nil }
        return session.projects.first { $0.name == name }?.title ?? name
    }
}

private struct AppRowLook: ViewModifier {
    func body(content: Content) -> some View {
        content
            .listRowInsets(EdgeInsets(top: 8, leading: Space.page, bottom: 8, trailing: Space.page))
            .listRowBackground(MycoColor.bg)
            .listRowSeparator(.hidden)
    }
}

/// An app's icon: by what kind of page it makes, else by its shelf.
struct AppIcon: View {
    let app: AppTemplate?
    var size: CGFloat = 40

    static func symbol(of app: AppTemplate?) -> String {
        switch app?.kind ?? "" {
        case "doc": return "doc.text"
        case "records": return "rectangle.split.3x1"
        case "table": return "tablecells"
        case "slides": return "rectangle.on.rectangle"
        default: break
        }
        switch app.map(AppsModel.shelf(of:)) ?? "" {
        case "office": return "doc.richtext"
        case "dev": return "chevron.left.forwardslash.chevron.right"
        case "tools": return "wrench.and.screwdriver"
        case "life": return "leaf"
        case "creative": return "paintpalette"
        case "game": return "gamecontroller"
        default: return "square.grid.2x2"
        }
    }

    var body: some View {
        Image(systemName: Self.symbol(of: app))
            .font(.system(size: size * 0.42, weight: .medium))
            .foregroundStyle(MycoColor.fg2)
            .frame(width: size, height: size)
            .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: size * 0.26, style: .continuous))
            .accessibilityHidden(true)
    }
}

/// One app: what it is, and "新建".
private struct AppRow: View {
    let app: AppTemplate

    var body: some View {
        HStack(spacing: 12) {
            AppIcon(app: app)
            VStack(alignment: .leading, spacing: 2) {
                Text(app.name).font(.body.weight(.medium)).foregroundStyle(MycoColor.fg)
                if let description = app.description, !description.isEmpty {
                    Text(description).font(.footnote).foregroundStyle(MycoColor.muted).lineLimit(2)
                }
                if app.official != true {
                    Text([app.slug, app.version, app.publisher ?? ""].filter { !$0.isEmpty }.joined(separator: " · "))
                        .font(.caption).foregroundStyle(MycoColor.faint).lineLimit(1)
                }
            }
            Spacer(minLength: 0)
            Label(L("新建", "New"), systemImage: "plus")
                .labelStyle(.titleAndIcon)
                .font(.footnote.weight(.medium)).foregroundStyle(MycoColor.accent)
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
        .accessibilityHint(L("新建一份\(app.name)", "Make a new \(app.name)"))
    }
}

/// A page made from an app: its name, the app, where, when.
private struct MadeRow: View {
    let page: Page
    let app: AppTemplate?
    let project: String?

    var body: some View {
        HStack(spacing: 12) {
            AppIcon(app: app, size: 34)
            VStack(alignment: .leading, spacing: 2) {
                Text(page.name).font(.body).foregroundStyle(MycoColor.fg).lineLimit(1)
                Text(meta).font(.footnote).foregroundStyle(MycoColor.muted).lineLimit(1)
            }
            Spacer(minLength: 0)
            if page.template?.latest?.isEmpty == false {
                Circle().fill(MycoColor.unread).frame(width: 7, height: 7)
                    .accessibilityLabel(L("有新版本", "A new version"))
            }
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }

    private var meta: String {
        var parts = [page.template?.name ?? app?.name ?? ""]
        if let project { parts.append(project) }
        if let at = page.updatedAt ?? page.createdAt { parts.append(RelativeTime.short(at)) }
        return parts.filter { !$0.isEmpty }.joined(separator: " · ")
    }
}

/// Making a page from an app: its name, where it goes, sample data.
struct UseAppSheet: View {
    let app: AppTemplate
    let onMade: (Page) -> Void

    @Environment(WorkspaceSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var project = ""
    @State private var sample = true
    @FocusState private var naming: Bool

    var body: some View {
        let model = session.apps
        NavigationStack {
            Form {
                Section {
                    HStack(spacing: 12) {
                        AppIcon(app: app, size: 34)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(app.name).font(.body.weight(.medium)).foregroundStyle(MycoColor.fg)
                            if let description = app.description, !description.isEmpty {
                                Text(description).font(.footnote).foregroundStyle(MycoColor.muted)
                            }
                        }
                    }
                }
                Section {
                    TextField(L("叫什么", "Name"), text: $name, prompt: Text(app.name))
                        .focused($naming)
                        .submitLabel(.done)
                        .accessibilityIdentifier("app.use.name")
                } header: {
                    Text(L("叫什么", "Name"))
                }
                Section {
                    Picker(L("放在哪", "Where"), selection: $project) {
                        Text(L("不放进项目", "No project")).tag("")
                        ForEach(session.projects) { one in
                            Text(one.title).tag(one.name)
                        }
                    }
                    .accessibilityIdentifier("app.use.project")
                } footer: {
                    Text(project.isEmpty
                         ? L("只有你和你的 agent 能看到。", "Only you and your agents can see it.")
                         : L("项目里的人和 agent 都能用。", "Everyone in the project can use it."))
                }
                Section {
                    Toggle(L("先放一些示例数据，看看怎么用", "Start with some sample data"), isOn: $sample)
                        .tint(MycoColor.accent)
                        .accessibilityIdentifier("app.use.sample")
                }
                if let failure = model.useFailure {
                    Section {
                        FailureLine(failure: failure)
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .background(MycoColor.bg)
            .navigationTitle(L("新建一份\(app.name)", "New \(app.name)"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(L("取消", "Cancel")) { dismiss() }.disabled(model.making)
                }
                ToolbarItem(placement: .confirmationAction) {
                    if model.making {
                        ProgressView()
                    } else {
                        Button(L("新建", "Make it")) {
                            Task {
                                if let page = await model.use(app, name: name.isEmpty ? app.name : name, project: project, sample: sample) {
                                    onMade(page)
                                }
                            }
                        }
                        .fontWeight(.semibold)
                        .accessibilityIdentifier("app.use.make")
                    }
                }
            }
        }
        .presentationDetents([.medium, .large])
        .interactiveDismissDisabled(model.making)
    }
}
