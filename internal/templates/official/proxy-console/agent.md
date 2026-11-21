# Proxy console

A local controller for an already-running Mihomo or sing-box core with Clash API enabled. It uses `myco.os.call("proxy.profile", ...)` and `proxy.control`; older Apps show an upgrade message. Open the explicit demo preview for a harmless tour.

There are no shared collections. Do not put controller credentials, subscriptions, connections or proxy configurations in page records, messages, exports or screenshots. Credentials use the App's local keychain store, scoped by workspace and page. Linux follows the App's protected-file fallback when configured. Forgetting a connection does not stop its core.

This app does not install a core, change the system proxy or DNS, start services, or enable TUN. Mode switching changes the existing core's mode. Closing a connection interrupts that connection. Only Selector groups can be manually changed. Other group types are managed by the core. Unsupported controller endpoints are reported as warnings.
