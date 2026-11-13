import Foundation

/// Times as the app writes them: relative and short ("5 分钟前").
public enum RelativeTime {
    public static func short(_ date: Date, now: Date = Date()) -> String {
        let seconds = max(0, now.timeIntervalSince(date))
        if seconds < 60 { return L("刚刚", "now") }
        let minutes = Int(seconds / 60)
        if minutes < 60 { return L("\(minutes) 分钟前", "\(minutes)m ago") }
        let hours = minutes / 60
        if hours < 24 { return L("\(hours) 小时前", "\(hours)h ago") }
        let days = hours / 24
        if days < 7 { return L("\(days) 天前", "\(days)d ago") }
        let locale = Locale(identifier: AppLanguage.current == .zh ? "zh-Hans" : "en")
        return date.formatted(.dateTime.month().day().locale(locale))
    }

    /// How long a task has been worked on, to the second: "40 秒",
    /// "20 分 37 秒", "2 小时 5 分" (as the desktop app counts it).
    public static func working(since date: Date, now: Date = Date()) -> String {
        let s = max(0, Int(now.timeIntervalSince(date)))
        if s < 60 { return L("\(s) 秒", "\(s)s") }
        let m = s / 60
        if m < 60 { return L("\(m) 分 \(s % 60) 秒", "\(m)m \(s % 60)s") }
        return L("\(m / 60) 小时 \(m % 60) 分", "\(m / 60)h \(m % 60)m")
    }

    /// What an agent at work is doing, when it has not said: one of these,
    /// a different one every few seconds.
    public static func workingVerb(now: Date = Date()) -> String {
        let verbs: [(String, String)] = [
            ("埋头干活", "Heads down"), ("琢磨中", "Mulling it over"), ("理一理头绪", "Untangling threads"),
            ("长菌丝中", "Growing mycelium"), ("推敲细节", "Weighing details"), ("捣鼓中", "Tinkering"),
            ("翻一翻资料", "Leafing through notes"), ("酝酿中", "Brewing"), ("一点点拼起来", "Piecing it together"),
            ("啃硬骨头", "Chewing on it"), ("散播孢子", "Spreading spores"), ("盘一盘", "Sorting it out"),
        ]
        // A pick that holds for a few seconds, then moves on.
        let turn = Int(now.timeIntervalSince1970 / 3.6)
        let pick = verbs[(turn &* 7 + turn / 5) % verbs.count]
        return L(pick.0, pick.1)
    }

    /// How long something has been going: "40 秒", "12 分钟", "2 小时".
    public static func elapsed(since date: Date, now: Date = Date()) -> String {
        let seconds = max(0, Int(now.timeIntervalSince(date)))
        if seconds < 60 { return L("\(seconds) 秒", "\(seconds)s") }
        let minutes = seconds / 60
        if minutes < 60 { return L("\(minutes) 分钟", "\(minutes)m") }
        return L("\(minutes / 60) 小时", "\(minutes / 60)h")
    }
}
