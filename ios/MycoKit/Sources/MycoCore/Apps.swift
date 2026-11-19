import Foundation
import MycoAPI

/// 应用: ready-made pages people and agents use together (the desktop
/// app's `TemplatesPage`). myco's own are on shelves; the workspace's own
/// come after them; the pages made from them are listed to open again.
@MainActor
@Observable
public final class AppsModel {
    private let hub: any HubAPI

    /// nil until the workspace has answered once.
    public private(set) var list: [AppTemplate]?
    /// The pages made from apps, the latest first.
    public private(set) var made: [Page] = []
    /// The workspace is from before apps (v0.20.5).
    public private(set) var unsupported = false
    public private(set) var failure: Failure?
    /// Why the last page could not be made.
    public private(set) var useFailure: Failure?
    /// A page is being made.
    public private(set) var making = false

    public init(hub: any HubAPI) {
        self.hub = hub
    }

    /// The shelves myco's apps are on, in order: key, Chinese, English.
    public static let shelves: [(key: String, zh: String, en: String)] = [
        ("office", "办公套件", "Office"), ("dev", "编程开发", "Development"), ("tools", "效率工具", "Tools"), ("life", "生活", "Life"),
        ("creative", "创意设计", "Creative"), ("game", "游戏", "Games"), ("other", "其他", "Other"),
    ]

    /// An app's shelf (hubs before v0.20.6 say none: the office kinds go on the first).
    public static func shelf(of app: AppTemplate) -> String {
        if let category = app.category, !category.isEmpty { return category }
        return ["table", "records", "doc", "slides"].contains(app.kind ?? "") ? "office" : "other"
    }

    /// Whether an app is what is looked for (its name, what it is for, its slug).
    public static func matches(_ app: AppTemplate, _ query: String) -> Bool {
        let wanted = query.trimmingCharacters(in: .whitespaces)
        guard !wanted.isEmpty else { return true }
        return "\(app.name) \(app.description ?? "") \(app.slug)".localizedCaseInsensitiveContains(wanted)
    }

    public var official: [AppTemplate] { (list ?? []).filter { $0.official == true } }
    public var own: [AppTemplate] { (list ?? []).filter { $0.official != true } }

    /// The shelves that have one of myco's apps on them.
    public var shelvesInUse: [(key: String, zh: String, en: String)] {
        Self.shelves.filter { shelf in official.contains { Self.shelf(of: $0) == shelf.key } }
    }

    /// The app a page was made from, when it is still here.
    public func app(of page: Page) -> AppTemplate? {
        guard let id = page.template?.id else { return nil }
        return list?.first { $0.id == id }
    }

    public func load() async {
        do {
            list = try await hub.apps()
            unsupported = false
            failure = nil
        } catch let error as HubError where Failure.isTooOld(error) || error.status == 404 {
            unsupported = true
            list = []
        } catch is CancellationError {
            return
        } catch {
            failure = Failure.of(error, canRetry: true)
            if list == nil { list = [] }
        }
        if !unsupported, let pages = try? await hub.pages() {
            made = pages.filter { $0.template != nil }.sorted { ($0.updatedAt ?? .distantPast) > ($1.updatedAt ?? .distantPast) }
        }
    }

    /// Makes a page from an app: its name, the project it is in (none:
    /// only me and my agents), with some sample data or none. The page, or
    /// nil and `failure` says why.
    public func use(_ app: AppTemplate, name: String, project: String?, sample: Bool) async -> Page? {
        guard !making else { return nil }
        useFailure = nil
        making = true
        defer { making = false }
        do {
            let page = try await hub.useApp(
                app.id, name: name.trimmingCharacters(in: .whitespacesAndNewlines), project: project?.isEmpty == true ? nil : project,
                sample: sample)
            made.removeAll { $0.id == page.id }
            made.insert(page, at: 0)
            return page
        } catch {
            useFailure = Failure.of(error, canRetry: false)
            return nil
        }
    }
}
