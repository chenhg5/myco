import SwiftUI
import MycoCore
import MycoUI

/// The model and reasoning effort beside the send button: a quiet label.
/// For an agent I may manage it opens a menu to switch both (from its next
/// run; a dot while a switch is not in use yet); for any other it says who
/// decides. The desktop app's `RunPill`.
struct RunChoiceButton: View {
    @Environment(WorkspaceSession.self) private var session
    let agent: String
    let choice: RunChoice

    @State private var explaining = false
    @State private var saving = false
    @State private var problem: String?

    var body: some View {
        Group {
            if choice.kind == .mine {
                Menu {
                    // Two rows that open, each saying what is chosen: the
                    // lists are too long to lay out one under the other.
                    Picker(selection: model) {
                        Text(L("跟随电脑设置", "Follow this computer")).tag("")
                        ForEach(choice.models, id: \.self) { Text(choice.name(of: $0)).tag($0) }
                    } label: {
                        Label(L("模型", "Model"), systemImage: "cpu")
                    }
                    .pickerStyle(.menu)
                    .accessibilityIdentifier("composer.run.model")
                    Picker(selection: effort) {
                        Text(L("默认", "Default")).tag("")
                        ForEach(choice.efforts, id: \.self) { Text(RunChoice.effortLabel($0)).tag($0) }
                    } label: {
                        Label(L("推理强度", "Effort"), systemImage: "gauge.with.dots.needle.33percent")
                    }
                    .pickerStyle(.menu)
                    .accessibilityIdentifier("composer.run.effort")
                    // What it runs with now, and when a switch applies.
                    Section {
                        Text(choice.note)
                    }
                    // On another vendor: where the models came from, and
                    // its runner asked to list them again.
                    if let source = choice.source {
                        Section {
                            Text(source)
                            if choice.canListAgain {
                                Button(action: listAgain) {
                                    Label(L("重新读取模型列表", "List models again"), systemImage: "arrow.clockwise")
                                }
                                .accessibilityIdentifier("composer.run.listAgain")
                            }
                        }
                    }
                } label: {
                    label
                }
                .menuOrder(.fixed)
                .disabled(saving)
            } else {
                Button { explaining = true } label: { label }
                    .buttonStyle(.plain)
                    .popover(isPresented: $explaining) {
                        VStack(alignment: .leading, spacing: 6) {
                            if choice.kind != .unknown {
                                Text(RunChoice.words(model: choice.modelInUse, effort: choice.effortInUse))
                                    .font(.subheadline.weight(.semibold)).foregroundStyle(MycoColor.fg)
                            }
                            Text(choice.note).font(.footnote).foregroundStyle(MycoColor.fg2)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        .padding(14)
                        .frame(width: 270, alignment: .leading)
                        .presentationCompactAdaptation(.popover)
                        .accessibilityIdentifier("composer.run.note")
                    }
            }
        }
        .accessibilityLabel(L("模型和推理强度：\(choice.label)", "Model and effort: \(choice.label)"))
        .accessibilityIdentifier("composer.run")
        .alert(L("没切换成", "Not switched"), isPresented: SwiftUI.Binding(get: { problem != nil }, set: { if !$0 { problem = nil } })) {
            Button(L("好", "OK")) { problem = nil }
        } message: {
            Text(problem ?? "")
        }
    }

    private var label: some View {
        HStack(spacing: 4) {
            if choice.pending {
                Circle().fill(MycoColor.amber).frame(width: 5, height: 5)
            }
            Text(choice.label)
                .font(.footnote)
                .foregroundStyle(choice.kind == .unknown ? MycoColor.faint : MycoColor.muted)
                .lineLimit(1)
            if choice.kind == .mine {
                Image(systemName: "chevron.down").font(.system(size: 9, weight: .semibold)).foregroundStyle(MycoColor.faint)
            }
        }
        .padding(.horizontal, 8)
        .frame(height: 44)
        .contentShape(Rectangle())
    }

    private var model: SwiftUI.Binding<String> {
        SwiftUI.Binding(get: { choice.chosenModel }, set: { picked in save(model: picked, effort: choice.chosenEffort) })
    }

    private var effort: SwiftUI.Binding<String> {
        SwiftUI.Binding(get: { choice.chosenEffort }, set: { picked in save(model: choice.chosenModel, effort: picked) })
    }

    private func listAgain() {
        guard !saving else { return }
        saving = true
        Task {
            problem = await session.listModelsAgain(of: agent)
            saving = false
        }
    }

    private func save(model: String, effort: String) {
        guard !saving, model != choice.chosenModel || effort != choice.chosenEffort else { return }
        saving = true
        Task {
            problem = await session.setRun(of: agent, model: model, effort: effort)
            saving = false
        }
    }
}

/// What an agent may touch when it runs, beside its model (the desktop
/// app's access pill): its owner switches it, from its next run, up to what
/// its computer allows; full access asks first. Others are told why not.
struct AccessButton: View {
    @Environment(WorkspaceSession.self) private var session
    let agent: String
    let choice: AccessChoice

    @State private var explaining = false
    @State private var saving = false
    @State private var problem: String?
    @State private var askingFull = false

    var body: some View {
        Group {
            if choice.editable {
                Menu {
                    Picker(selection: SwiftUI.Binding(get: { choice.chosen }, set: { pick($0) })) {
                        Text(L("跟随电脑设置", "Follow this computer")).tag("")
                        ForEach(choice.allowed, id: \.self) { level in
                            Text(AccessChoice.label(level) + " · " + AccessChoice.says(level)).tag(level)
                        }
                    } label: {
                        Text(L("它运行时能动什么", "What it may touch when it runs"))
                    }
                    .pickerStyle(.inline)
                    Section {
                        Text(L("现在是：\(AccessChoice.label(choice.inUse))。切换后下次唤醒时生效。", "In use: \(AccessChoice.label(choice.inUse)). A switch applies from its next run."))
                        if choice.cap != "full" {
                            Text(L("那台电脑最多允许「\(AccessChoice.label(choice.cap))」。", "That computer allows at most \(AccessChoice.label(choice.cap))."))
                        }
                    }
                } label: {
                    label
                }
                .menuOrder(.fixed)
                .disabled(saving)
            } else {
                Button { explaining = true } label: { label }
                    .buttonStyle(.plain)
                    .popover(isPresented: $explaining) {
                        VStack(alignment: .leading, spacing: 6) {
                            Text(AccessChoice.label(choice.shown)).font(.subheadline.weight(.semibold)).foregroundStyle(MycoColor.fg)
                            Text(AccessChoice.says(choice.shown) + L("。", ". ") + (choice.why ?? ""))
                                .font(.footnote).foregroundStyle(MycoColor.fg2)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        .padding(14)
                        .frame(width: 270, alignment: .leading)
                        .presentationCompactAdaptation(.popover)
                        .accessibilityIdentifier("composer.access.note")
                    }
            }
        }
        .accessibilityLabel(L("权限：\(AccessChoice.label(choice.shown))", "Access: \(AccessChoice.label(choice.shown))"))
        .accessibilityIdentifier("composer.access")
        .alert(L("使用完全权限？", "Use full access?"), isPresented: $askingFull) {
            Button(L("使用完全权限", "Use full access"), role: .destructive) { save("full") }
            Button(L("取消", "Cancel"), role: .cancel) {}
        } message: {
            Text(L("完全权限下，它运行时没有沙箱，可以读写那台电脑上的任何文件。", "With full access, it runs without a sandbox and can read and write any file on that computer."))
        }
        .alert(L("没切换成", "Not switched"), isPresented: SwiftUI.Binding(get: { problem != nil }, set: { if !$0 { problem = nil } })) {
            Button(L("好", "OK")) { problem = nil }
        } message: {
            Text(problem ?? "")
        }
    }

    private var label: some View {
        HStack(spacing: 3) {
            if choice.pending {
                Circle().fill(MycoColor.amber).frame(width: 5, height: 5)
            }
            Image(systemName: choice.shown == "full" ? "exclamationmark.shield" : choice.shown == "readonly" ? "eye" : "folder")
                .font(.system(size: 11, weight: .medium))
                .foregroundStyle(choice.shown == "full" ? MycoColor.amber : MycoColor.muted)
            if choice.editable {
                Image(systemName: "chevron.down").font(.system(size: 9, weight: .semibold)).foregroundStyle(MycoColor.faint)
            }
        }
        .padding(.horizontal, 6)
        .frame(height: 44)
        .contentShape(Rectangle())
    }

    private func pick(_ level: String) {
        guard level != choice.chosen else { return }
        if level == "full" { askingFull = true } else { save(level) }
    }

    private func save(_ level: String) {
        guard !saving else { return }
        saving = true
        let settings = session.agent(agent)?.runSettings
        Task {
            problem = await session.setRun(of: agent, model: settings?.model ?? "", effort: settings?.effort ?? "", access: level)
            saving = false
        }
    }
}
