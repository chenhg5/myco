import Foundation
import MycoAPI

/// 待处理 (the desktop's notices): what waits for me — an agent asking, a
/// task of mine that failed, a run that failed or got stuck. Decisions and
/// approvals are shown with their workflows and the superagent, where they
/// are answered.
@MainActor
@Observable
public final class NoticesModel {
    private let hub: any HubAPI

    /// What waits for me, newest first, that is not shown elsewhere.
    public private(set) var waiting: [HubNotice] = []
    /// Open needs-you notices, as the hub counts them (the app's badge).
    public private(set) var badge = 0
    /// The workspace is from before notices (v0.21.21).
    public private(set) var unsupported = false
    public private(set) var loaded = false

    /// Kinds answered where they are shown (a workflow's decision, an
    /// approval): not listed again.
    static let shownElsewhere: Set<String> = ["decision", "approval"]

    public init(hub: any HubAPI) {
        self.hub = hub
    }

    public func load() async {
        do {
            // The newest 200 are enough: they are kept 30 days.
            var all: [HubNotice] = []
            var after: Int64? = nil
            for _ in 0..<5 {
                let page = try await hub.notices(after: after, limit: 200)
                all += page.notices
                badge = page.badge ?? 0
                guard page.notices.count == 200, let last = page.notices.last?.seq else { break }
                after = last
            }
            // One per place: the newest of what leads there.
            var seen: Set<String> = []
            waiting = all.filter { $0.open && !Self.shownElsewhere.contains($0.kind) }
                .sorted { $0.createdAt > $1.createdAt }
                .filter { seen.insert($0.target ?? $0.id).inserted }
            unsupported = false
            loaded = true
        } catch let error as HubError where error.status == 404 || Failure.isTooOld(error) {
            unsupported = true
            waiting = []
            loaded = true
        } catch {
            loaded = true
        }
    }

    /// Opened: what leads there is read (and settled, if nothing more is
    /// asked of me).
    public func read(_ notice: HubNotice) async {
        try? await hub.readNotices(target: notice.target)
        await load()
    }

    /// What a notice says, in a line: who, and what.
    public static func words(_ notice: HubNotice, name: (String) -> String) -> String {
        let who = notice.actor.map(name) ?? ""
        let what: String
        switch notice.kind {
        case "asking": what = L("在问你", "is asking you")
        case "failed": what = L("的任务失败了", "had a task fail")
        case "run_failed": what = L("工作流失败了", "workflow failed")
        case "stuck": what = L("可能卡住了", "may be stuck")
        case "mention": what = L("提到了你", "mentioned you")
        case "answered": what = L("回复了", "answered")
        default: what = notice.kind
        }
        return who.isEmpty ? what : who + " " + what
    }
}

/// 用量 (the desktop's Usage page): where the workspace's tokens go, for
/// the last 7, 30 or 90 days: totals, a bar a day, and who used most.
@MainActor
@Observable
public final class UsageModel {
    private let hub: any HubAPI

    public struct Totals: Sendable, Hashable {
        public var total: Int64 = 0
        public var input: Int64 = 0
        public var output: Int64 = 0
        public var calls: Int64 = 0
        public var cost: Double = 0

        mutating func add(_ row: UsageRow) {
            total += row.total
            input += row.inputTokens ?? 0
            output += row.outputTokens ?? 0
            calls += row.calls ?? 0
            cost += row.costUsd ?? 0
        }
    }

    public private(set) var days = 7
    /// Every day of the range, oldest first, with its tokens.
    public private(set) var byDay: [(day: String, total: Int64)] = []
    public private(set) var total = Totals()
    /// Members, models and projects, most tokens first.
    public private(set) var agents: [(name: String, total: Int64)] = []
    public private(set) var models: [(name: String, total: Int64)] = []
    public private(set) var projects: [(name: String, total: Int64)] = []
    /// The workspace is from before usage (v0.21.34).
    public private(set) var unsupported = false
    public private(set) var loaded = false

    public init(hub: any HubAPI) {
        self.hub = hub
    }

    /// The range's first and last UTC days, ending today.
    static func range(days: Int, now: Date) -> (from: String, to: String, all: [String]) {
        var utc = Calendar(identifier: .gregorian)
        utc.timeZone = TimeZone(identifier: "UTC")!
        let format = DateFormatter()
        format.calendar = utc
        format.timeZone = utc.timeZone
        format.locale = Locale(identifier: "en_US_POSIX")
        format.dateFormat = "yyyy-MM-dd"
        let all = (0..<days).reversed().compactMap { utc.date(byAdding: .day, value: -$0, to: now).map(format.string(from:)) }
        return (all.first ?? "", all.last ?? "", all)
    }

    public func load(days: Int? = nil, now: Date = Date()) async {
        if let days { self.days = days }
        let range = Self.range(days: self.days, now: now)
        do {
            let report = try await hub.usage(from: range.from, to: range.to, project: nil)
            summarize(report.rows, days: range.all)
            unsupported = false
            loaded = true
        } catch let error as HubError where error.status == 404 || Failure.isTooOld(error) {
            unsupported = true
            loaded = true
        } catch {
            loaded = true
        }
    }

    func summarize(_ rows: [UsageRow], days: [String]) {
        var total = Totals()
        var perDay: [String: Int64] = [:]
        var perAgent: [String: Int64] = [:]
        var perModel: [String: Int64] = [:]
        var perProject: [String: Int64] = [:]
        for row in rows {
            total.add(row)
            perDay[row.day, default: 0] += row.total
            perAgent[row.agent, default: 0] += row.total
            perModel[row.model ?? "", default: 0] += row.total
            perProject[row.project ?? "", default: 0] += row.total
        }
        func ranked(_ map: [String: Int64]) -> [(name: String, total: Int64)] {
            map.map { (name: $0.key, total: $0.value) }.sorted { $0.total != $1.total ? $0.total > $1.total : $0.name < $1.name }
        }
        self.total = total
        byDay = days.map { ($0, perDay[$0] ?? 0) }
        agents = ranked(perAgent)
        models = ranked(perModel)
        projects = ranked(perProject)
    }

    /// 1.2M, 340K, 12: as the desktop writes token counts.
    public static func compact(_ n: Int64) -> String {
        for (size, suffix) in [(1e9, "B"), (1e6, "M"), (1e3, "K")] where Double(n) >= size {
            let value = (Double(n) / size * 10).rounded() / 10
            return (value == value.rounded() ? String(Int(value)) : String(value)) + suffix
        }
        return String(n)
    }
}
