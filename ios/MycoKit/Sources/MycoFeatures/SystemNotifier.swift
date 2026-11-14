import Foundation
import UserNotifications
import MycoCore

/// The system's notifications: a banner, a sound, the number on the icon.
/// Shown while the app is in front too (what is on screen is never sent
/// here), and a tap on one leads to its conversation.
@MainActor
public final class SystemNotifier: NSObject, Notifier, UNUserNotificationCenterDelegate {
    /// A notification was tapped: which workspace, and which page of it
    /// (as `Router.open(spec:)` reads it: "agent:<name>", "project:<name>").
    public var onOpen: (@MainActor (_ workspace: String, _ page: String) -> Void)?

    public override init() {
        super.init()
        // Set at launch, so a tap that starts the app is not missed.
        UNUserNotificationCenter.current().delegate = self
    }

    public func authorized(asking: Bool) async -> Bool {
        let center = UNUserNotificationCenter.current()
        switch await center.notificationSettings().authorizationStatus {
        case .authorized, .provisional, .ephemeral:
            return true
        case .notDetermined:
            guard asking else { return false }
            return (try? await center.requestAuthorization(options: [.alert, .sound, .badge])) ?? false
        default:
            return false
        }
    }

    public func post(_ notice: Notice, workspaceName: String?) async {
        let content = UNMutableNotificationContent()
        content.title = notice.title
        if let workspaceName { content.subtitle = workspaceName }
        content.body = Self.words(for: notice)
        content.sound = .default
        // One conversation's notifications stack together.
        content.threadIdentifier = notice.workspace + "|" + notice.party
        content.userInfo = ["workspace": notice.workspace, "page": Self.page(of: notice.target)]
        try? await UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: notice.id, content: content, trigger: nil))
    }

    public func setBadge(_ count: Int) async {
        try? await UNUserNotificationCenter.current().setBadgeCount(count)
    }

    /// What a notification says: the message's first line, with what kind
    /// of news it is when that is not an answer.
    static func words(for notice: Notice) -> String {
        switch notice.kind {
        case .failed:
            return notice.body.isEmpty ? L("没做成", "Did not finish") : L("没做成：\(notice.body)", "Did not finish: \(notice.body)")
        case .asking:
            return L("在问你：\(notice.body)", "Asks you: \(notice.body)")
        case .answered, .said, .progress:
            return notice.body
        }
    }

    static func page(of target: ChatRow.Target) -> String {
        switch target {
        case .project(let name): return "project:" + name
        case .agent(let name): return "agent:" + name
        }
    }

    // The system calls these off the main actor.

    public nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter, willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        [.banner, .sound, .list]
    }

    public nonisolated func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse) async {
        let info = response.notification.request.content.userInfo
        guard let workspace = info["workspace"] as? String, let page = info["page"] as? String else { return }
        await MainActor.run { self.onOpen?(workspace, page) }
    }
}
