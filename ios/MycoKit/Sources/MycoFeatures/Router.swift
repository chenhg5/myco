import SwiftUI
import Observation

public enum AppTab: String, Hashable, Sendable {
    case chats, projects, members
}

/// A page pushed inside a tab.
public enum Route: Hashable, Sendable {
    /// A project: the conversation with its coordinator.
    case project(String)
    /// An agent's conversation; `focus` is a task to land on.
    case agent(String, focus: String?)
    /// What two agents said to each other, to read: `agent` did the work
    /// (its steps show), `with` is the one that gave it. `focus` is a
    /// message to land on. Opened from a handoff card.
    case exchange(agent: String, with: String, focus: String?)
    case projectInfo(String)
    /// What is sent for me on a schedule, or when something calls.
    case automations
    /// 应用: ready-made pages to use with agents, and the ones made.
    case apps
}

/// Something that rises from the bottom, over any tab.
public enum SheetRoute: Hashable, Identifiable, Sendable {
    case workspace
    case memberCard(String)
    case addAgent
    /// A command that makes a folder on a computer one agents can use;
    /// `project`: the project it is added from, if any.
    case addComputer(project: String?)
    /// Naming my superagent, or starting one.
    case superagent
    /// Writing a conversation out as a file: with this agent, in this
    /// project's page or its own.
    case export(agent: String, project: String?)
    /// Another session takes this agent over; `back`: a handover to undo.
    case handover(agent: String, back: String?)
    /// A conversation's pictures and files on one page.
    case files(agent: String, project: String?)
    /// An address to pin above a conversation.
    case pinAddress(agent: String)
    /// A conversation's background (nil key: the default for all).
    case background(key: String?)

    public var id: String {
        switch self {
        case .workspace: return "workspace"
        case .memberCard(let name): return "member:" + name
        case .addAgent: return "add"
        case .addComputer(let project): return "computer:" + (project ?? "")
        case .export(let agent, let project): return "export:" + agent + ":" + (project ?? "")
        case .handover(let agent, let back): return "handover:" + agent + ":" + (back ?? "")
        case .files(let agent, let project): return "files:" + agent + ":" + (project ?? "")
        case .pinAddress(let agent): return "pin:" + agent
        case .background(let key): return "background:" + (key ?? "")
        }
    }
}

/// Where the app is. The only place navigation state lives: a tap, a
/// notification and a link all end up here, so they take the same road.
@MainActor
@Observable
public final class Router {
    public var tab: AppTab = .chats
    public var chatsPath: [Route] = []
    public var projectsPath: [Route] = []
    public var membersPath: [Route] = []
    public var sheet: SheetRoute?
    /// A web page an agent published, open over the whole screen.
    public var page: PageRoute?

    public init() {}

    /// Pushes a page in the tab on screen.
    public func push(_ route: Route) {
        switch tab {
        case .chats: chatsPath.append(route)
        case .projects: projectsPath.append(route)
        case .members: membersPath.append(route)
        }
    }

    /// Goes to a tab with exactly these pages on it (from a notification or
    /// a link): back then leads to the tab's list.
    public func show(_ routes: [Route], in tab: AppTab) {
        sheet = nil
        self.tab = tab
        switch tab {
        case .chats: chatsPath = routes
        case .projects: projectsPath = routes
        case .members: membersPath = routes
        }
    }

    /// Selecting the tab already on screen goes back to its list.
    public func select(_ tab: AppTab) {
        if self.tab == tab { show([], in: tab) } else { self.tab = tab }
    }

    /// Closes any sheet and opens the agent's conversation where the person is.
    /// The page in front, in the tab on screen; nil on a tab's own list.
    public var front: Route? {
        switch tab {
        case .chats: return chatsPath.last
        case .projects: return projectsPath.last
        case .members: return membersPath.last
        }
    }

    public func chat(with agent: String) {
        sheet = nil
        push(.agent(agent, focus: nil))
    }

    public func reset() {
        sheet = nil
        tab = .chats
        chatsPath = []
        projectsPath = []
        membersPath = []
    }

    /// Opens a place named in words, for UI tests and screenshots
    /// (`-myco-route project:myco-ios`): `tab:<name>`, `project:<name>`,
    /// `info:<project>`, `agent:<name>`, `member:<name>`, `sheet:workspace`,
    /// `sheet:add`, `sheet:computer`, `page:automations`, `page:apps`. Returns false for anything else.
    @discardableResult
    public func open(spec: String) -> Bool {
        let parts = spec.split(separator: ":", maxSplits: 1).map(String.init)
        guard parts.count == 2 else { return false }
        let (what, name) = (parts[0], parts[1])
        switch what {
        case "tab":
            guard let tab = AppTab(rawValue: name) else { return false }
            show([], in: tab)
        case "project": show([.project(name)], in: .projects)
        case "info": show([.project(name), .projectInfo(name)], in: .projects)
        case "agent": show([.agent(name, focus: nil)], in: .chats)
        case "member":
            show([], in: .members)
            sheet = .memberCard(name)
        case "page":
            switch name {
            case "automations": show([.automations], in: .members)
            case "apps": show([.apps], in: .members)
            default: return false
            }
        case "sheet":
            switch name {
            case "workspace": sheet = .workspace
            case "add": sheet = .addAgent
            case "computer": sheet = .addComputer(project: nil)
            default: return false
            }
        default: return false
        }
        return true
    }
}
