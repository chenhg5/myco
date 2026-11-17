import Foundation
import MycoAPI

/// The model and reasoning effort an agent runs with, as the message box
/// shows them (as Codex and Claude show theirs under their input): small
/// and quiet. For an agent a runner wakes, whoever may manage it switches
/// both there, from its next run. An agent in its own session (it receives
/// there, or runs in the cloud) decides itself: shown, and where it is
/// switched said. Nothing known, nothing shown. The desktop app's `RunPill`.
public struct RunChoice: Sendable, Hashable {
    public enum Kind: Sendable, Hashable {
        /// I may switch it here.
        case mine
        /// Its own session decides (switched there, with /model).
        case itsSession
        /// In its own session, and it has not said what it runs on.
        case unknown
        /// A runner's agent that someone else manages.
        case theirs
        /// Mine to switch, but this workspace keeps no run settings yet.
        case workspaceTooOld
    }

    public var kind: Kind
    /// "Opus 5.5 · 高", or "模型未知".
    public var label: String
    /// What can be picked, when it is mine: what its CLI is commonly
    /// started with, and whatever it runs with now.
    public var models: [String] = []
    public var efforts: [String] = []
    /// What was chosen for it; "" follows its computer's setting.
    public var chosenModel = ""
    public var chosenEffort = ""
    /// What it runs with now, as its runner or it itself says.
    public var modelInUse = ""
    public var effortInUse = ""
    /// A switch was made that is not in use yet: it applies from its next run.
    public var pending = false

    static let knownModels: [String: [String]] = [
        "claude": ["fable", "opus", "sonnet", "haiku"],
        "codex": ["gpt-6-sol", "gpt-6", "gpt-5-codex", "o3"],
    ]
    static let knownEfforts: [String: [String]] = [
        "claude": ["low", "medium", "high", "xhigh", "max"],
        "codex": ["minimal", "low", "medium", "high", "xhigh"],
    ]

    /// - Parameters:
    ///   - manages: I own the agent, or manage the workspace.
    ///   - tooOld: the workspace answered that it keeps no run settings.
    public static func of(_ agent: Agent, manages: Bool, tooOld: Bool = false) -> RunChoice? {
        guard Naming.kind(of: agent) == .agent else { return nil }
        let runtime = agent.runtime
        var cli = runtime?.cli ?? ""
        if cli.isEmpty { cli = agent.binding.harness ?? "" }
        let cloud = (agent.machineId ?? "").isEmpty && cli.hasSuffix("-cloud")
        let inbox = cli == "inbox"
        let runner = !cloud && !inbox && (cli == "claude" || cli == "codex")
        let editable = runner && manages && !tooOld
        let chosenModel = agent.runSettings?.model ?? "", chosenEffort = agent.runSettings?.effort ?? ""
        let inUseModel = runtime?.model ?? "", inUseEffort = runtime?.effort ?? ""
        let model = chosenModel.isEmpty ? inUseModel : chosenModel
        let effort = chosenEffort.isEmpty ? inUseEffort : chosenEffort
        let unknown = model.isEmpty && !editable && (inbox || cloud)
        guard !model.isEmpty || editable || unknown else { return nil }
        var choice = RunChoice(
            kind: editable ? .mine : unknown ? .unknown : (inbox || cloud) ? .itsSession
                : (runner && manages && tooOld) ? .workspaceTooOld : .theirs,
            label: unknown ? L("模型未知", "Model unknown") : words(model: model, effort: effort))
        choice.chosenModel = chosenModel
        choice.chosenEffort = chosenEffort
        choice.modelInUse = inUseModel
        choice.effortInUse = inUseEffort
        if editable {
            choice.models = with([inUseModel, chosenModel], before: knownModels[cli] ?? [])
            choice.efforts = with([inUseEffort, chosenEffort], before: knownEfforts[cli] ?? [])
            choice.pending = (!chosenModel.isEmpty && chosenModel != inUseModel) || (!chosenEffort.isEmpty && chosenEffort != inUseEffort)
        }
        return choice
    }

    /// The list with what is in use put first, when it is not in it.
    private static func with(_ extra: [String], before list: [String]) -> [String] {
        var list = list
        for value in extra where !value.isEmpty && !list.contains(value) { list.insert(value, at: 0) }
        return list
    }

    /// "Opus 5.5 · 高"; a model not known reads "默认模型".
    public static func words(model: String, effort: String) -> String {
        let name = short(model)
        return [name.isEmpty ? L("默认模型", "Default model") : name, effort.isEmpty ? "" : effortLabel(effort)]
            .filter { !$0.isEmpty }.joined(separator: " · ")
    }

    /// What it runs with now, for the line under the choices.
    public var inUse: String {
        let name = RunChoice.short(modelInUse)
        return (name.isEmpty ? "—" : name) + (effortInUse.isEmpty ? "" : " · " + RunChoice.effortLabel(effortInUse))
    }

    /// What is said when it is not mine to switch.
    public var note: String {
        switch kind {
        case .mine:
            // (short: a menu shows two lines of it)
            return L("现在用的是 \(inUse)。切换后下次唤醒时生效。", "Now: \(inUse). A switch applies from its next run.")
        case .unknown:
            return L("它在自己的会话里收消息，还没说用的哪个模型。它的 myco 升级到新版后，会在等消息时自动报上来；也可以让它运行 myco agent runtime set。",
                     "It receives in its own session and has not said which model it runs on. With a newer myco it says so itself while it waits for messages; or ask it to run myco agent runtime set.")
        case .itsSession:
            return L("这是它自己的会话在用的设置，在它的会话里用 /model 切换。", "Its own session's setting: switch it there with /model.")
        case .theirs:
            return L("只有管理它的人能切换。", "Only those who manage it can switch it.")
        case .workspaceTooOld:
            return L("这个工作区的版本还不能在这里切换，升级工作区后再试。", "This workspace's version cannot switch it here yet. Upgrade the workspace and try again.")
        }
    }

    /// A model's name, short, as a person says it: claude-opus-5-5 →
    /// Opus 5.5, gpt-6-sol → GPT-6 Sol, opus → Opus.
    public static func short(_ model: String) -> String {
        var name = model.trimmingCharacters(in: .whitespaces)
        guard !name.isEmpty else { return "" }
        if name.hasSuffix("]"), let open = name.lastIndex(of: "[") { name = String(name[..<open]) }
        if name.lowercased().hasPrefix("claude-") { name = String(name.dropFirst("claude-".count)) }
        // (a date at the end: -20260101)
        if let dash = name.lastIndex(of: "-") {
            let tail = name[name.index(after: dash)...]
            if tail.count == 8, tail.allSatisfy(\.isNumber) { name = String(name[..<dash]) }
        }
        if name.lowercased().hasPrefix("gpt-") {
            let rest = name.dropFirst("gpt-".count).split(separator: "-", omittingEmptySubsequences: false).map(String.init)
            guard let first = rest.first else { return "GPT" }
            let more = rest.dropFirst().map { part -> String in
                guard let letter = part.first, letter.isLetter, letter.isLowercase else { return "-" + part }
                return " " + letter.uppercased() + part.dropFirst()
            }
            return "GPT-" + first + more.joined()
        }
        // name-5-5 → Name 5.5; name-4 → Name 4
        var words: [String] = [], numbers: [String] = []
        for part in name.split(separator: "-").map(String.init) {
            if part.allSatisfy(\.isNumber) { numbers.append(part) } else { words.append(part) }
        }
        let said = words.map { $0.count <= 2 ? $0.uppercased() : $0.prefix(1).uppercased() + $0.dropFirst() }.joined(separator: " ")
        return numbers.isEmpty ? said : said + " " + numbers.joined(separator: ".")
    }

    public static func effortLabel(_ effort: String) -> String {
        switch effort {
        case "minimal": return L("最低", "Minimal")
        case "low": return L("低", "Low")
        case "medium": return L("中", "Medium")
        case "high": return L("高", "High")
        case "xhigh": return L("很高", "Very high")
        case "max": return L("最高", "Max")
        default: return effort
        }
    }
}
