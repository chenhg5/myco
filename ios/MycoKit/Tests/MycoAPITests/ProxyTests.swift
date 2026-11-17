import Foundation
import Testing
@testable import MycoAPI

/// The app's network proxy: the setting, and which session a request takes.
struct ProxyTests {
    @Test func aSettingSaysWhatIsWrongWithIt() {
        #expect(ProxySettings().problem == nil)
        #expect(ProxySettings(mode: .none).problem == nil)
        #expect(ProxySettings(mode: .manual, kind: .http, host: "192.168.1.10", port: 7890).problem == nil)
        #expect(ProxySettings(mode: .manual, kind: .socks5, host: "proxy.example.com", port: 1080, user: "me").problem == nil)
        // The address is a host name or an IP, nothing more.
        for host in ["", "  ", "http://127.0.0.1", "127.0.0.1/x", "user@host", "a b"] {
            #expect(ProxySettings(mode: .manual, host: host, port: 1080).problem == .host, "\(host)")
        }
        for port in [0, -1, 65_536] {
            #expect(ProxySettings(mode: .manual, host: "h", port: port).problem == .port)
        }
        // Not asked of a setting that sets no proxy.
        #expect(ProxySettings(mode: .system, host: "", port: 0).problem == nil)
    }

    @Test func whatWasSavedBeforeStillReads() throws {
        let old = try JSONDecoder().decode(ProxySettings.self, from: Data(#"{"mode":"later","host":"h"}"#.utf8))
        #expect(old.mode == .system)
        #expect(old.kind == .socks5)
        #expect(old.host == "h")
        let whole = ProxySettings(mode: .manual, kind: .https, host: "p.example.com", port: 8443, user: "me")
        #expect(try JSONDecoder().decode(ProxySettings.self, from: JSONEncoder().encode(whole)) == whole)
    }

    @Test func thisDeviceAndTheLocalNetworkAreLocal() {
        for host in [
            "localhost", "LOCALHOST.", "dev.localhost", "studio.local", "127.0.0.1", "127.8.9.1", "10.0.0.5", "172.16.0.1", "172.31.255.254",
            "192.168.1.10", "169.254.3.4", "0.0.0.0", "::1", "[::1]", "fe80::1", "fd12:3456::1", "::",
        ] {
            #expect(ProxySettings.isLocal(host), "\(host)")
        }
        for host in ["example.com", "8.8.8.8", "172.32.0.1", "172.15.0.1", "192.169.1.1", "11.0.0.1", "2001:db8::1", "local", "mylocal.com", ""] {
            #expect(!ProxySettings.isLocal(host), "\(host)")
        }
    }

    @Test func aRequestTakesTheSessionTheSettingSays() {
        let network = AppNetwork()
        let far = URL(string: "https://hub.example.com/v1/x")
        let near = URL(string: "http://192.168.1.20:8080/v1/x")
        // As the system has it: the shared session, for everything.
        #expect(network.session(for: far) === URLSession.shared)
        #expect(network.session(for: near) === URLSession.shared)

        // A proxy set here: what is far goes through it, what is on the
        // local network goes directly, through no proxy at all.
        network.apply(ProxySettings(mode: .manual, kind: .socks5, host: "10.0.0.2", port: 1080, user: "me"), password: "pw")
        let through = network.session(for: far)
        let directly = network.session(for: near)
        #expect(through !== URLSession.shared)
        #expect(directly !== through)
        #expect(through.configuration.proxyConfigurations.count == 1)
        // (An empty dictionary beside it would switch the proxy off too.)
        #expect(through.configuration.connectionProxyDictionary?.isEmpty != true)
        #expect(directly.configuration.proxyConfigurations.isEmpty)
        #expect(directly.configuration.connectionProxyDictionary?.isEmpty == true, "not the system's proxies either")
        #expect(network.session(for: URL(string: "http://localhost:8080/")) === directly)
        #expect(network.session(for: nil) === through)
        #expect(network.current.mode == .manual)

        // No proxy: one session, with the system's proxies off.
        network.apply(ProxySettings(mode: .none), password: "")
        let none = network.session(for: far)
        #expect(none !== URLSession.shared)
        #expect(none.configuration.proxyConfigurations.isEmpty)
        #expect(none.configuration.connectionProxyDictionary?.isEmpty == true)
        #expect(network.session(for: near) === none)

        // A setting that cannot be used counts as the system's.
        network.apply(ProxySettings(mode: .manual, host: "", port: 0), password: "")
        #expect(network.session(for: far) === URLSession.shared)
        #expect(network.current.mode == .system)
    }

    @Test func whyATryFailedIsSaidInAWord() {
        func failed(_ code: URLError.Code) -> ProxyTrial { ProxyTrial.of(URLError(code)) }
        #expect(failed(.timedOut) == .timeout)
        #expect(failed(.cannotConnectToHost) == .refused)
        #expect(failed(.cannotFindHost) == .notFound)
        #expect(failed(.dnsLookupFailed) == .notFound)
        #expect(failed(.userAuthenticationRequired) == .auth)
        // What the system says when a proxy wants a password: under the
        // error, or by itself.
        let refused = NSError(domain: NSPOSIXErrorDomain, code: Int(EAUTH))
        #expect(ProxyTrial.of(refused) == .auth)
        #expect(ProxyTrial.of(NSError(domain: NSURLErrorDomain, code: -1005, userInfo: [NSUnderlyingErrorKey: refused])) == .auth)
        guard case .other = failed(.networkConnectionLost) else {
            Issue.record("anything else is said in the system's words")
            return
        }
    }

    @Test func aFailedRequestThroughTheProxyGetsANewSession() {
        let network = AppNetwork()
        let away = URL(string: "https://hub.example.com/v1/overview")!
        let near = URL(string: "http://192.168.1.5:8080/v1/overview")!
        // As the system has it: nothing of the app's own to make again.
        network.failed(away, URLError(.timedOut))
        #expect(network.session(for: away) === URLSession.shared)
        network.apply(ProxySettings(mode: .manual, kind: .socks5, host: "192.168.1.10", port: 7891), password: "")
        let first = network.session(for: away)
        let direct = network.session(for: near)
        // A request to the local network went directly: the proxy's
        // session stays.
        network.failed(near, URLError(.timedOut))
        #expect(network.session(for: away) === first)
        // One through the proxy: the next opens a new tunnel; the direct
        // one is left alone.
        network.failed(away, URLError(.timedOut))
        let second = network.session(for: away)
        #expect(second !== first)
        #expect(network.session(for: near) === direct)
        #expect(second.configuration.proxyConfigurations.count == 1, "the same proxy, made again")
        // Not again at once: a burst of failures makes one new session.
        network.failed(away, URLError(.networkConnectionLost))
        #expect(network.session(for: away) === second)
    }
}
