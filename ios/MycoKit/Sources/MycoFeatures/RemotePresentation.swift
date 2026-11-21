import UIKit

/// The rest of the phone App stays portrait. Only an authorized remote
/// connection temporarily requests landscape, and every revocation restores it.
@MainActor
public enum RemotePresentation {
    public private(set) static var orientations: UIInterfaceOrientationMask = .portrait

    static func setActive(_ active: Bool) {
        let next: UIInterfaceOrientationMask = active ? .landscape : .portrait
        guard orientations != next else { return }
        orientations = next
        for case let scene as UIWindowScene in UIApplication.shared.connectedScenes {
            guard scene.activationState == .foregroundActive else { continue }
            let root = scene.windows.first(where: { $0.isKeyWindow })?.rootViewController
            var visible = root
            while let presented = visible?.presentedViewController { visible = presented }
            root?.setNeedsUpdateOfSupportedInterfaceOrientations()
            visible?.setNeedsUpdateOfSupportedInterfaceOrientations()
            scene.requestGeometryUpdate(.iOS(interfaceOrientations: orientations)) { _ in
                // The system can postpone rotation while backgrounded; the
                // mask remains correct and the next activation retries it.
            }
        }
    }
}
