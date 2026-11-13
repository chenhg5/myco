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
    case projectInfo(String)
}

/// Something that rises from the bottom, over any tab.
public enum SheetRoute: Hashable, Identifiable, Sendable {
    case workspace
    case memberCard(String)
    case addAgent

    public var id: String {
        switch self {
        case .workspace: return "workspace"
        case .memberCard(let name): return "member:" + name
        case .addAgent: return "add"
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
    /// `sheet:add`. Returns false for anything else.
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
        case "sheet":
            switch name {
            case "workspace": sheet = .workspace
            case "add": sheet = .addAgent
            default: return false
            }
        default: return false
        }
        return true
    }
}
