import Foundation

/// Optional Remote Desktop signaling. Pixels and input travel over WebRTC,
/// never in these requests; the phone is a controller, never a capture host.
public struct RemoteComputer: Codable, Sendable, Hashable {
    public var machine: String
    public var name: String
    public var os: String
    public var control: Bool

    public init(machine: String, name: String, os: String, control: Bool) {
        self.machine = machine; self.name = name; self.os = os; self.control = control
    }
}

public struct RemoteDescription: Codable, Sendable, Hashable {
    public var type: String
    public var sdp: String
    public init(type: String, sdp: String) { self.type = type; self.sdp = sdp }
}

public struct RemoteOffer: Codable, Sendable, Hashable {
    public var description: RemoteDescription
    public var pairingCode: String
    public var control: Bool
    public init(description: RemoteDescription, pairingCode: String, control: Bool) {
        self.description = description; self.pairingCode = pairingCode; self.control = control
    }
}

public struct RemoteConnection: Codable, Sendable, Hashable {
    public var id: String
    public var machine: String
    public var state: String
    public var answer: RemoteDescription?
    public var error: String?
    public init(id: String, machine: String, state: String, answer: RemoteDescription? = nil, error: String? = nil) {
        self.id = id; self.machine = machine; self.state = state; self.answer = answer; self.error = error
    }
}
