import Foundation
import MycoAPI

/// An account's usage limits in words: the desktop app's web/src/app/quota.ts
/// (the two say the same).
public enum QuotaWords {
    static let hour = 3_600
    static let day = 86_400

    /// A window's length for people: "5 小时", "每周", "每天", "每月".
    static func duration(_ seconds: Int) -> String {
        if seconds == 5 * hour { return L("5 小时", "5 hours") }
        if seconds == day { return L("每天", "Daily") }
        if seconds == 7 * day { return L("每周", "Weekly") }
        if seconds >= 28 * day, seconds <= 31 * day { return L("每月", "Monthly") }
        if seconds < 2 * day {
            let h = Int((Double(seconds) / Double(hour)).rounded())
            return L("\(h) 小时", "\(h) hours")
        }
        let d = Int((Double(seconds) / Double(day)).rounded())
        return L("\(d) 天", "\(d) days")
    }

    /// A window's name for people: by its length when the source says it,
    /// else by the CLI's own window names; a model's bucket adds the model.
    public static func label(_ w: QuotaWindow) -> String {
        let base: String
        if let seconds = w.durationSec, seconds > 0 {
            base = duration(seconds)
        } else {
            switch w.name {
            case "five_hour": base = L("5 小时", "5 hours")
            case "seven_day", "seven_day_opus", "seven_day_sonnet", "weekly": base = L("每周", "Weekly")
            case "primary": base = w.label ?? L("主要窗口", "Primary window")
            case "secondary": base = w.label ?? L("次要窗口", "Secondary window")
            case "monthly": base = L("每月", "Monthly")
            case "daily": base = L("每天", "Daily")
            case "rolling": base = L("滚动窗口", "Rolling window")
            default: base = w.label ?? w.name
            }
        }
        let scope = w.modelScope ?? (w.name == "seven_day_opus" ? "opus" : w.name == "seven_day_sonnet" ? "sonnet" : nil)
        guard let scope, !scope.isEmpty else { return base }
        let name = scope.prefix(1).uppercased() + scope.dropFirst()
        return L("\(base)（\(name)）", "\(base) (\(name))")
    }

    /// Whole percent left, 0...100.
    public static func left(_ w: QuotaWindow) -> Int {
        100 - max(0, min(100, Int((w.used * 100).rounded())))
    }

    /// No room left: its status says so, or it is all used.
    public static func spent(_ w: QuotaWindow) -> Bool {
        (w.status.map { !$0.isEmpty && $0 != "allowed" } ?? false) || w.used >= 1
    }

    /// The tightest window: the one with the least left (spent first).
    public static func worst(_ q: Quota) -> QuotaWindow? {
        q.windows.reduce(nil) { (best: QuotaWindow?, w: QuotaWindow) -> QuotaWindow? in
            guard let best else { return w }
            if spent(w) != spent(best) { return spent(w) ? w : best }
            return w.used > best.used ? w : best
        }
    }

    /// A successful read is there: numbers to show (a failure never
    /// stands for a full quota).
    public static func hasNumbers(_ q: Quota) -> Bool {
        !q.windows.isEmpty && (q.at.map { $0.timeIntervalSince1970 > 946_684_800 } ?? false)
    }

    /// "5h", "7d", "月": a window's name in a few characters.
    public static func short(_ w: QuotaWindow) -> String {
        let seconds = w.durationSec ?? (w.name == "five_hour" ? 5 * hour : w.name.hasPrefix("seven_day") || w.name == "weekly" ? 7 * day : w.name == "daily" ? day : 0)
        if w.name == "monthly" || (seconds >= 28 * day && seconds <= 31 * day) { return L("月", "mo") }
        if seconds == 0 { return w.label ?? "" }
        if seconds < 2 * day { return "\(Int((Double(seconds) / Double(hour)).rounded()))h" }
        return "\(Int((Double(seconds) / Double(day)).rounded()))d"
    }

    public enum Level: Sendable { case ok, low, out }

    /// The composer's colour: grey, amber under 20% left, red under 5% or spent.
    public static func level(_ w: QuotaWindow) -> Level {
        if spent(w) || left(w) < 5 { return .out }
        return left(w) < 20 ? .low : .ok
    }

    /// The composer's pill: "5h 剩 38%" or "5h 用完".
    public static func pill(_ w: QuotaWindow) -> String {
        let name = short(w)
        let what = spent(w) ? L("用完", "out") : L("剩 \(left(w))%", "\(left(w))% left")
        return name.isEmpty ? what : name + " " + what
    }

    /// "3:40", "周五" or "11 月 20 日": when a window resets.
    public static func resetLabel(_ date: Date?, now: Date = Date()) -> String {
        guard let date else { return "—" }
        let diff = date.timeIntervalSince(now)
        if diff <= 0 { return L("马上", "now") }
        let calendar = Calendar.current
        if diff < 86_400 {
            let c = calendar.dateComponents([.hour, .minute], from: date)
            return String(format: "%d:%02d", c.hour ?? 0, c.minute ?? 0)
        }
        if diff < 7 * 86_400 {
            let weekday = calendar.component(.weekday, from: date) - 1
            return L(["周日", "周一", "周二", "周三", "周四", "周五", "周六"][weekday], ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"][weekday])
        }
        let c = calendar.dateComponents([.month, .day], from: date)
        return L("\(c.month ?? 0) 月 \(c.day ?? 0) 日", date.formatted(.dateTime.month(.abbreviated).day().locale(Locale(identifier: "en_US"))))
    }

    /// "3 小时后重置": how long until a window resets; "" when unknown.
    public static func resetIn(_ date: Date?, now: Date = Date()) -> String {
        guard let date else { return "" }
        let s = date.timeIntervalSince(now)
        if s <= 60 { return L("马上重置", "resets now") }
        if s < 3_600 { let m = Int((s / 60).rounded(.up)); return L("\(m) 分钟后重置", "resets in \(m) min") }
        if s < 2 * 86_400 { let h = Int((s / 3_600).rounded()); return L("\(h) 小时后重置", "resets in \(h) h") }
        let d = Int((s / 86_400).rounded())
        return L("\(d) 天后重置", "resets in \(d) days")
    }

    /// A window's row: "剩 38% · 3 小时后重置", or "额度用完 · ...".
    public static func row(_ w: QuotaWindow, now: Date = Date()) -> String {
        let reset = resetIn(w.resetsAt, now: now)
        let what = spent(w) ? L("额度用完", "Out of quota") : L("剩 \(left(w))%", "\(left(w))% left")
        return [what, reset].filter { !$0.isEmpty }.joined(separator: " · ")
    }

    /// "Codex · ChatGPT 订阅": whose usage, on which plan.
    public static func source(_ q: Quota) -> String {
        let names = ["codex": "Codex", "claude": "Claude", "opencode_go": "OpenCode Go", "minimax": "MiniMax"]
        let provider = q.provider == "unknown" ? "" : (q.provider ?? q.source ?? "")
        let plans = [
            "chatgpt_subscription": L("ChatGPT 订阅", "ChatGPT plan"), "claude_subscription": L("Claude 订阅", "Claude plan"),
            "go": L("Go 套餐", "Go plan"), "coding_plan": "Coding Plan",
        ]
        let plan = q.plan.map { plans[$0] ?? $0 }
        let from = q.sourceKind == "cloud_session" ? L("云端会话", "cloud session") : nil
        return [names[provider] ?? provider, plan, from].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · ")
    }

    /// Why there are no fresh numbers, in a few words.
    public static func status(_ q: Quota) -> String? {
        switch q.status ?? "ok" {
        case "ok", "": return nil
        case "no_credentials": return L("需要配置 key", "Needs an API key")
        case "auth_required": return L("需要在 CLI 重新登录", "Sign in again in the CLI")
        case "unsupported": return L("这个账号不报额度", "This account doesn't report usage")
        case "throttled": return L("查得太勤，稍后再取", "Checked too often; tries again later")
        case "schema_changed": return L("额度格式变了，需要更新 myco", "The usage format changed; update myco")
        default: return L("获取失败，稍后再试", "Couldn't fetch; tries again later")
        }
    }

    /// "5 小时额度已用完，3:40 重置；…": what the composer says while the
    /// account is out (it does not stop the send).
    public static func outNote(_ q: Quota, now: Date = Date()) -> String? {
        guard hasNumbers(q), let w = worst(q), spent(w) else { return nil }
        let at = resetLabel(w.resetsAt, now: now)
        return w.resetsAt != nil && at != "—"
            ? L("\(label(w))额度已用完，\(at) 重置；现在发的消息可能要等到那时才会处理。", "\(label(w)) quota is used up; resets \(at). What you send now may wait until then.")
            : L("\(label(w))额度已用完；现在发的消息可能要等重置后才会处理。", "\(label(w)) quota is used up. What you send now may wait until it resets.")
    }
}
