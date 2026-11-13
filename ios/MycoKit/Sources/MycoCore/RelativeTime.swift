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

    /// How long something has been going: "40 秒", "12 分钟", "2 小时".
    public static func elapsed(since date: Date, now: Date = Date()) -> String {
        let seconds = max(0, Int(now.timeIntervalSince(date)))
        if seconds < 60 { return L("\(seconds) 秒", "\(seconds)s") }
        let minutes = seconds / 60
        if minutes < 60 { return L("\(minutes) 分钟", "\(minutes)m") }
        return L("\(minutes / 60) 小时", "\(minutes / 60)h")
    }
}
