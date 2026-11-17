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
                    Picker(L("模型", "Model"), selection: model) {
                        Text(L("跟随电脑设置", "Follow this computer")).tag("")
                        ForEach(choice.models, id: \.self) { Text(RunChoice.short($0)).tag($0) }
                    }
                    .pickerStyle(.inline)
                    Picker(L("推理强度", "Effort"), selection: effort) {
                        Text(L("默认", "Default")).tag("")
                        ForEach(choice.efforts, id: \.self) { Text(RunChoice.effortLabel($0)).tag($0) }
                    }
                    .pickerStyle(.inline)
                    Section(choice.note) {}
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

    private func save(model: String, effort: String) {
        guard !saving, model != choice.chosenModel || effort != choice.chosenEffort else { return }
        saving = true
        Task {
            problem = await session.setRun(of: agent, model: model, effort: effort)
            saving = false
        }
    }
}
