import Foundation

/// Any JSON value: a workflow's inputs and a step's output are what its
/// definition says they are.
public enum JSONValue: Codable, Sendable, Hashable {
    case string(String)
    case number(Double)
    case bool(Bool)
    case array([JSONValue])
    case object([String: JSONValue])
    case null

    public init(from decoder: any Decoder) throws {
        let one = try decoder.singleValueContainer()
        if one.decodeNil() {
            self = .null
        } else if let value = try? one.decode(Bool.self) {
            self = .bool(value)
        } else if let value = try? one.decode(Double.self) {
            self = .number(value)
        } else if let value = try? one.decode(String.self) {
            self = .string(value)
        } else if let value = try? one.decode([JSONValue].self) {
            self = .array(value)
        } else {
            self = .object(try one.decode([String: JSONValue].self))
        }
    }

    public func encode(to encoder: any Encoder) throws {
        var one = encoder.singleValueContainer()
        switch self {
        case .string(let value): try one.encode(value)
        case .number(let value): try one.encode(value)
        case .bool(let value): try one.encode(value)
        case .array(let value): try one.encode(value)
        case .object(let value): try one.encode(value)
        case .null: try one.encodeNil()
        }
    }

    /// As a person reads it: text as it is, the rest as short JSON.
    public var text: String {
        switch self {
        case .string(let value): return value
        case .number(let value): return value == value.rounded() && abs(value) < 1e15 ? String(Int64(value)) : String(value)
        case .bool(let value): return value ? "true" : "false"
        case .null: return ""
        case .array, .object:
            let encoder = JSONEncoder()
            encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
            return (try? encoder.encode(self)).flatMap { String(data: $0, encoding: .utf8) } ?? ""
        }
    }
}

/// A workflow: a graph the workspace walks, handing steps to agents,
/// computers and people (hubs from v0.21).
public struct Workflow: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var name: String
    public var title: String?
    public var description: String?
    public var project: String?
    public var owner: String?
    public var version: Int
    public var enabled: Bool
    /// How many steps it has.
    public var steps: Int?
    /// Runs going on.
    public var active: Int?
    public var lastRun: WorkflowRun?
    /// With `GET /v1/workflows/{ref}` only.
    public var definition: WorkflowDefinition?
    public var createdAt: Date
    public var updatedAt: Date

    public init(
        id: String, name: String, title: String? = nil, description: String? = nil, project: String? = nil, owner: String? = nil,
        version: Int = 1, enabled: Bool = true, steps: Int? = nil, active: Int? = nil, lastRun: WorkflowRun? = nil,
        definition: WorkflowDefinition? = nil, createdAt: Date = Date(), updatedAt: Date = Date()
    ) {
        self.id = id
        self.name = name
        self.title = title
        self.description = description
        self.project = project
        self.owner = owner
        self.version = version
        self.enabled = enabled
        self.steps = steps
        self.active = active
        self.lastRun = lastRun
        self.definition = definition
        self.createdAt = createdAt
        self.updatedAt = updatedAt
    }

    /// What people call it.
    public var shownName: String { (title?.isEmpty == false ? title : nil) ?? name }
}

/// What a workflow is made of, as far as the phone reads it: what a run
/// asks for, and its steps (who does each).
public struct WorkflowDefinition: Codable, Sendable, Hashable {
    public struct Inputs: Codable, Sendable, Hashable {
        public var required: [String]?
        public var properties: [String: Field]?

        public init(required: [String]? = nil, properties: [String: Field]? = nil) {
            self.required = required
            self.properties = properties
        }
    }

    /// One input of a run.
    public struct Field: Codable, Sendable, Hashable {
        /// string, number, integer, boolean, array
        public var type: String?
        public var title: String?
        public var description: String?
        /// textarea: more than a line.
        public var format: String?
        public var `enum`: [JSONValue]?
        public var `default`: JSONValue?

        enum CodingKeys: String, CodingKey { case type, title, description, format, `enum`, `default` }

        /// (A schema may say more than this reads, in other shapes: what
        /// cannot be read is left out, not the workflow.)
        public init(from decoder: any Decoder) throws {
            let c = try decoder.container(keyedBy: CodingKeys.self)
            type = (try? c.decodeIfPresent(String.self, forKey: .type))
                ?? (try? c.decodeIfPresent([String].self, forKey: .type))?.first { $0 != "null" }
            title = try? c.decodeIfPresent(String.self, forKey: .title)
            description = try? c.decodeIfPresent(String.self, forKey: .description)
            format = try? c.decodeIfPresent(String.self, forKey: .format)
            self.enum = try? c.decodeIfPresent([JSONValue].self, forKey: .enum)
            self.default = try? c.decodeIfPresent(JSONValue.self, forKey: .default)
        }

        public init(type: String? = nil, title: String? = nil, description: String? = nil, format: String? = nil, enum: [JSONValue]? = nil, default: JSONValue? = nil) {
            self.type = type
            self.title = title
            self.description = description
            self.format = format
            self.enum = `enum`
            self.default = `default`
        }
    }

    public struct Step: Codable, Sendable, Hashable, Identifiable {
        public var id: String
        public var title: String?
        public var agent: String?
        public var role: String?
        public var computer: String?
        public var human: String?

        public init(id: String, title: String? = nil, agent: String? = nil, role: String? = nil, computer: String? = nil, human: String? = nil) {
            self.id = id
            self.title = title
            self.agent = agent
            self.role = role
            self.computer = computer
            self.human = human
        }
    }

    public var inputs: Inputs?
    public var steps: [Step]?

    enum CodingKeys: String, CodingKey { case inputs, steps }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        inputs = try? c.decodeIfPresent(Inputs.self, forKey: .inputs)
        steps = try? c.decodeIfPresent([Step].self, forKey: .steps)
    }

    public init(inputs: Inputs? = nil, steps: [Step]? = nil) {
        self.inputs = inputs
        self.steps = steps
    }
}

/// A run of a workflow.
public struct WorkflowRun: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    public var workflowId: String
    public var name: String
    public var version: Int?
    public var project: String?
    /// running, waiting (only decisions are open), succeeded, failed, cancelled
    public var status: String
    public var inputs: [String: JSONValue]?
    public var owner: String?
    public var by: String?
    /// manual, agent, automation
    public var trigger: String?
    public var error: String?
    public var current: [String]?
    public var steps: [RunStep]?
    public var durationMs: Int64?
    public var createdAt: Date
    public var updatedAt: Date
    public var endedAt: Date?

    public init(
        id: String, workflowId: String, name: String, version: Int? = 1, project: String? = nil, status: String,
        inputs: [String: JSONValue]? = nil, owner: String? = nil, by: String? = nil, trigger: String? = "manual", error: String? = nil,
        current: [String]? = nil, steps: [RunStep]? = nil, durationMs: Int64? = nil, createdAt: Date = Date(), updatedAt: Date = Date(),
        endedAt: Date? = nil
    ) {
        self.id = id
        self.workflowId = workflowId
        self.name = name
        self.version = version
        self.project = project
        self.status = status
        self.inputs = inputs
        self.owner = owner
        self.by = by
        self.trigger = trigger
        self.error = error
        self.current = current
        self.steps = steps
        self.durationMs = durationMs
        self.createdAt = createdAt
        self.updatedAt = updatedAt
        self.endedAt = endedAt
    }

    /// Still going (or waiting for a decision).
    public var isGoing: Bool { status == "running" || status == "waiting" }
}

/// One start of a step in a run.
public struct RunStep: Codable, Sendable, Hashable, Identifiable {
    public var id: String
    /// The step's id in the definition.
    public var step: String
    public var title: String?
    /// agent, computer, human
    public var kind: String
    public var attempt: Int?
    /// running, waiting (a decision), done, failed, cancelled
    public var status: String
    /// Who does it: an agent, a computer, a person.
    public var target: String?
    public var messageId: String?
    /// What was sent (a decision: what is asked).
    public var input: String?
    public var summary: String?
    public var error: String?
    /// A decision's choices.
    public var options: [String]?
    public var durationMs: Int64?
    public var startedAt: Date?
    public var endedAt: Date?
    /// When a decision expires.
    public var dueAt: Date?

    public init(
        id: String, step: String, title: String? = nil, kind: String, attempt: Int? = 1, status: String, target: String? = nil,
        messageId: String? = nil, input: String? = nil, summary: String? = nil, error: String? = nil, options: [String]? = nil,
        durationMs: Int64? = nil, startedAt: Date? = Date(), endedAt: Date? = nil, dueAt: Date? = nil
    ) {
        self.id = id
        self.step = step
        self.title = title
        self.kind = kind
        self.attempt = attempt
        self.status = status
        self.target = target
        self.messageId = messageId
        self.input = input
        self.summary = summary
        self.error = error
        self.options = options
        self.durationMs = durationMs
        self.startedAt = startedAt
        self.endedAt = endedAt
        self.dueAt = dueAt
    }

    /// What people call it.
    public var shownName: String { (title?.isEmpty == false ? title : nil) ?? step }
}

/// A decision waiting for me: the run, and the step that asks.
public struct WorkflowDecision: Codable, Sendable, Hashable, Identifiable {
    public var run: WorkflowRun
    public var step: RunStep

    public init(run: WorkflowRun, step: RunStep) {
        self.run = run
        self.step = step
    }

    public var id: String { run.id + "/" + step.id }
}
