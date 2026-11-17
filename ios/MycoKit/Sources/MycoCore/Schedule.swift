import Foundation

/// Schedules as people say them, and the cron the workspace keeps. The
/// editor offers the common ones (every day, weekdays, one day a week,
/// every few hours) and a cron of your own; a cron that is one of them
/// reads in words. The desktop app's `schedule.ts`.
public enum Every: Sendable, Hashable {
    case daily(hour: Int, minute: Int)
    case weekdays(hour: Int, minute: Int)
    /// `day`: 0 is Sunday.
    case weekly(day: Int, hour: Int, minute: Int)
    /// Every N hours, at that minute.
    case hours(every: Int, minute: Int)
    case cron(String)

    public var cron: String {
        switch self {
        case .daily(let hour, let minute): return "\(minute) \(hour) * * *"
        case .weekdays(let hour, let minute): return "\(minute) \(hour) * * 1-5"
        case .weekly(let day, let hour, let minute): return "\(minute) \(hour) * * \(day)"
        case .hours(let every, let minute): return "\(minute) \(every == 1 ? "*" : "*/\(every)") * * *"
        case .cron(let cron): return cron.split(whereSeparator: \.isWhitespace).joined(separator: " ")
        }
    }

    private static func number(_ text: Substring, _ low: Int, _ high: Int) -> Int? {
        guard !text.isEmpty, text.allSatisfy({ $0.isASCII && $0.isNumber }), let value = Int(text), value >= low, value <= high else { return nil }
        return value
    }

    /// The preset a cron is, if it is one (else a cron of its own).
    public init(cron: String) {
        let fields = cron.split(whereSeparator: \.isWhitespace)
        if fields.count == 5, let minute = Self.number(fields[0], 0, 59), fields[2] == "*", fields[3] == "*" {
            if let hour = Self.number(fields[1], 0, 23) {
                if fields[4] == "*" {
                    self = .daily(hour: hour, minute: minute)
                    return
                }
                if fields[4] == "1-5" {
                    self = .weekdays(hour: hour, minute: minute)
                    return
                }
                if let day = Self.number(fields[4], 0, 7) {
                    self = .weekly(day: day % 7, hour: hour, minute: minute)
                    return
                }
            }
            if fields[4] == "*" {
                var every: Int?
                if fields[1] == "*" { every = 1 } else if fields[1].hasPrefix("*/") { every = Self.number(fields[1].dropFirst(2), 1, 12) }
                if let every {
                    self = .hours(every: every, minute: minute)
                    return
                }
            }
        }
        self = .cron(cron.trimmingCharacters(in: .whitespaces))
    }

    static let daysZh = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"]
    static let daysEn = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"]

    static func clock(_ hour: Int, _ minute: Int) -> String {
        String(format: "%02d:%02d", hour, minute)
    }

    /// In words: "每天 09:00", "工作日 18:30", "每 2 小时（第 15 分）", or the cron.
    public var words: String {
        switch self {
        case .daily(let hour, let minute):
            return L("每天 \(Self.clock(hour, minute))", "Every day at \(Self.clock(hour, minute))")
        case .weekdays(let hour, let minute):
            return L("工作日 \(Self.clock(hour, minute))", "Weekdays at \(Self.clock(hour, minute))")
        case .weekly(let day, let hour, let minute):
            return L("每\(Self.daysZh[day % 7]) \(Self.clock(hour, minute))", "Every \(Self.daysEn[day % 7]) at \(Self.clock(hour, minute))")
        case .hours(let every, let minute):
            let at = String(format: "%02d", minute)
            return every == 1
                ? L("每小时（第 \(minute) 分）", "Every hour at :\(at)")
                : L("每 \(every) 小时（第 \(minute) 分）", "Every \(every) hours at :\(at)")
        case .cron(let cron):
            return "cron " + cron
        }
    }

    /// A cron of one's own needs its five fields.
    public var isComplete: Bool {
        if case .cron(let cron) = self { return cron.split(whereSeparator: \.isWhitespace).count == 5 }
        return true
    }

    /// When it next sends, by a clock in `zone` (nil for a cron of its own).
    public func next(after from: Date, in zone: TimeZone = .current) -> Date? {
        if case .cron = self { return nil }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = zone
        guard var time = calendar.date(from: calendar.dateComponents([.year, .month, .day, .hour, .minute], from: from)) else { return nil }
        for _ in 0..<(8 * 24 * 60) {
            time = time.addingTimeInterval(60)
            let parts = calendar.dateComponents([.hour, .minute, .weekday], from: time)
            let hour = parts.hour ?? 0, minute = parts.minute ?? 0, day = (parts.weekday ?? 1) - 1
            switch self {
            case .daily(let h, let m): if hour == h, minute == m { return time }
            case .weekdays(let h, let m): if (1...5).contains(day), hour == h, minute == m { return time }
            case .weekly(let d, let h, let m): if day == d, hour == h, minute == m { return time }
            case .hours(let every, let m): if minute == m, hour % every == 0 { return time }
            case .cron: return nil
            }
        }
        return nil
    }

    /// A moment soon, as people say it: 今天 16:40, 明天 09:00, 周五 09:00, 12月3日 09:00.
    public static func soon(_ at: Date, now: Date, in zone: TimeZone = .current) -> String {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = zone
        let parts = calendar.dateComponents([.month, .day, .hour, .minute, .weekday], from: at)
        let time = clock(parts.hour ?? 0, parts.minute ?? 0)
        let days = calendar.dateComponents([.day], from: calendar.startOfDay(for: now), to: calendar.startOfDay(for: at)).day ?? 0
        let weekday = (parts.weekday ?? 1) - 1
        switch days {
        case 0: return L("今天 \(time)", "today \(time)")
        case 1: return L("明天 \(time)", "tomorrow \(time)")
        case -1: return L("昨天 \(time)", "yesterday \(time)")
        case 2..<7: return L("\(daysZh[weekday]) \(time)", "\(daysEn[weekday].prefix(3)) \(time)")
        default:
            let months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]
            let month = parts.month ?? 1, day = parts.day ?? 1
            return L("\(month)月\(day)日 \(time)", "\(months[(month - 1) % 12]) \(day) \(time)")
        }
    }

    /// The days for a row of chips, Monday first: (day, short name).
    public static var weekChips: [(day: Int, label: String)] {
        [1, 2, 3, 4, 5, 6, 0].map { ($0, L(String(daysZh[$0].dropFirst()), String(daysEn[$0].prefix(3)))) }
    }
}
