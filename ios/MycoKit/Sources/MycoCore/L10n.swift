import Foundation

/// The app's two languages. Strings stay next to their use as
/// `L("中文", "English")`, as in the desktop app (docs/design/app-ui.md §9).
public enum AppLanguage: String, Sendable, CaseIterable {
    case zh, en

    /// The key the choice is stored under ("system", "zh" or "en").
    public static let defaultsKey = "myco-lang"

    /// A language for the current task only (tests): nothing global changes.
    @TaskLocal public static var forced: AppLanguage?

    /// The language in use: the one chosen in Settings, else the system's.
    public static var current: AppLanguage {
        if let forced { return forced }
        if let chosen = UserDefaults.standard.string(forKey: defaultsKey), let lang = AppLanguage(rawValue: chosen) {
            return lang
        }
        return Locale.preferredLanguages.contains { $0.lowercased().hasPrefix("zh") } ? .zh : .en
    }
}

public func L(_ zh: String, _ en: String) -> String {
    AppLanguage.current == .zh ? zh : en
}
