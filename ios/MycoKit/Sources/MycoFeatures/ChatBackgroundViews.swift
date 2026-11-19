import SwiftUI
import UIKit
import PhotosUI
import MycoCore
import MycoUI

/// What a conversation draws behind its words: its background, or the page.
struct ChatBackdrop: View {
    let key: String
    private var backgrounds: ChatBackgrounds { .shared }

    var body: some View {
        if let background = backgrounds.background(for: key) {
            ChatBackgroundLayer(background: background).ignoresSafeArea()
        } else {
            MycoColor.bg.ignoresSafeArea()
        }
    }
}

/// One background: its preset or picture, filled and centred, with the
/// veil over a picture.
struct ChatBackgroundLayer: View {
    let background: ChatBackground

    var body: some View {
        ZStack {
            MycoColor.bg
            if let image = background.image, let picture = UIImage(contentsOfFile: ChatBackgrounds.shared.url(of: image).path) {
                GeometryReader { proxy in
                    Image(uiImage: picture).resizable().scaledToFill()
                        .frame(width: proxy.size.width, height: proxy.size.height).clipped()
                }
                MycoColor.bg.opacity(background.veil)
            } else if let preset = background.preset {
                Self.preset(preset)
            }
        }
    }

    /// The presets, from the theme's own colours (light and dark alike).
    @ViewBuilder static func preset(_ id: String) -> some View {
        switch id {
        case "mist": LinearGradient(colors: [MycoColor.tinted(0x6B9BE0, 0.16), MycoColor.tinted(0xE0A96B, 0.13)], startPoint: .topLeading, endPoint: .bottomTrailing)
        case "mint": LinearGradient(colors: [MycoColor.tinted(0x3FAE8A, 0.15), MycoColor.tinted(0x6BB7E0, 0.12)], startPoint: .topTrailing, endPoint: .bottomLeading)
        case "lilac": LinearGradient(colors: [MycoColor.tinted(0xA07BE0, 0.14), MycoColor.tinted(0xE07BA9, 0.11)], startPoint: .topLeading, endPoint: .bottomTrailing)
        case "sand": LinearGradient(colors: [MycoColor.tinted(0xD9B98A, 0.18), MycoColor.tinted(0xC9A27A, 0.10)], startPoint: .top, endPoint: .bottom)
        case "dots": Canvas { context, size in
            let dot = GraphicsContext.Shading.color(MycoColor.fg.opacity(0.13))
            for x in stride(from: 9.0, to: size.width, by: 18) {
                for y in stride(from: 9.0, to: size.height, by: 18) {
                    context.fill(Path(ellipseIn: CGRect(x: x - 1.2, y: y - 1.2, width: 2.4, height: 2.4)), with: dot)
                }
            }
        }
        case "grid": Canvas { context, size in
            var path = Path()
            for x in stride(from: 0.0, to: size.width, by: 24) { path.addRect(CGRect(x: x, y: 0, width: 0.6, height: size.height)) }
            for y in stride(from: 0.0, to: size.height, by: 24) { path.addRect(CGRect(x: 0, y: y, width: size.width, height: 0.6)) }
            context.fill(path, with: .color(MycoColor.fg.opacity(0.07)))
        }
        default: EmptyView()
        }
    }
}

/// Choosing a conversation's background, or the default for the rest: a
/// preview with two lines of talk on it, the presets, a picture from this
/// phone, and for a picture how strong the veil is.
struct ChatBackgroundSheet: View {
    /// The conversation's key; nil chooses the default.
    let key: String?
    @Environment(\.dismiss) private var dismiss
    @State private var choice = ChatBackground()
    @State private var follows = true
    @State private var picked: PhotosPickerItem?
    @State private var problem: String?

    private var backgrounds: ChatBackgrounds { .shared }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: Space.m) {
                    preview
                    if key != nil {
                        Toggle(L("跟随默认背景", "Follow the default"), isOn: $follows)
                            .tint(MycoColor.green)
                            .accessibilityIdentifier("background.follow")
                    }
                    if !follows || key == nil {
                        LazyVGrid(columns: [GridItem(.adaptive(minimum: 92), spacing: 10)], spacing: 12) {
                            tile(L("无", "None"), on: choice.isEmpty) { choice = ChatBackground(veil: choice.veil) } swatch: {
                                MycoColor.bg
                            }
                            ForEach(ChatBackground.presets, id: \.self) { preset in
                                tile(ChatBackground.name(of: preset), on: choice.preset == preset && choice.image == nil) {
                                    choice = ChatBackground(preset: preset, veil: choice.veil)
                                } swatch: {
                                    ChatBackgroundLayer(background: ChatBackground(preset: preset))
                                }
                            }
                            PhotosPicker(selection: $picked, matching: .images) {
                                VStack(spacing: 6) {
                                    ZStack {
                                        MycoColor.pill
                                        Image(systemName: "photo").foregroundStyle(MycoColor.muted)
                                    }
                                    .frame(height: 56).clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                    .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).strokeBorder(choice.image != nil ? MycoColor.fg : MycoColor.line, lineWidth: choice.image != nil ? 2 : 1))
                                    Text(L("用一张照片", "A photo")).font(.caption).foregroundStyle(MycoColor.fg2)
                                }
                            }
                            .accessibilityIdentifier("background.photo")
                        }
                        if choice.image != nil {
                            HStack {
                                Text(L("淡化", "Veil")).font(.subheadline).foregroundStyle(MycoColor.fg2)
                                Slider(value: $choice.veil, in: 0...0.9)
                                Text("\(Int(choice.veil * 100))%").font(.footnote.monospacedDigit()).foregroundStyle(MycoColor.muted).frame(width: 40, alignment: .trailing)
                            }
                        }
                        if let problem {
                            Text(problem).font(.footnote).foregroundStyle(MycoColor.red)
                        }
                    }
                    Text(L("只存在这台手机上。", "Kept on this phone only.")).font(.footnote).foregroundStyle(MycoColor.muted)
                }
                .padding(Space.page)
            }
            .background(MycoColor.bg)
            .navigationTitle(key == nil ? L("默认聊天背景", "Default chat background") : L("聊天背景", "Chat background"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button(L("取消", "Cancel")) { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button(L("完成", "Done")) {
                        backgrounds.set(follows && key != nil ? nil : choice, for: key)
                        dismiss()
                    }
                    .fontWeight(.semibold)
                    .accessibilityIdentifier("background.done")
                }
            }
            .onAppear {
                if let key {
                    follows = backgrounds.own(key) == nil
                    choice = backgrounds.own(key) ?? backgrounds.fallback ?? ChatBackground()
                } else {
                    choice = backgrounds.fallback ?? ChatBackground()
                }
            }
            .onChange(of: picked) { _, item in
                guard let item else { return }
                Task { await take(item) }
            }
        }
    }

    /// Two lines of talk on what is chosen.
    private var preview: some View {
        ZStack(alignment: .bottom) {
            if follows, let key, backgrounds.own(key) == nil, let fallback = backgrounds.fallback {
                ChatBackgroundLayer(background: fallback)
            } else if follows && key != nil {
                MycoColor.bg
            } else {
                ChatBackgroundLayer(background: choice)
            }
            VStack(alignment: .leading, spacing: 10) {
                Text(L("改好了，测试也过了。", "Done, and the tests pass.")).font(.subheadline).foregroundStyle(MycoColor.fg)
                Text(L("好，推上去吧", "Good, push it")).font(.subheadline).foregroundStyle(MycoColor.fg)
                    .padding(.horizontal, 12).padding(.vertical, 7)
                    .background(MycoColor.pill, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
                    .frame(maxWidth: .infinity, alignment: .trailing)
            }
            .padding(16)
        }
        .frame(height: 150)
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).strokeBorder(MycoColor.line, lineWidth: 1))
        .accessibilityIdentifier("background.preview")
    }

    private func tile<Swatch: View>(_ name: String, on: Bool, action: @escaping () -> Void, @ViewBuilder swatch: () -> Swatch) -> some View {
        Button(action: action) {
            VStack(spacing: 6) {
                swatch()
                    .frame(height: 56).clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                    .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).strokeBorder(on ? MycoColor.fg : MycoColor.line, lineWidth: on ? 2 : 1))
                Text(name).font(.caption).foregroundStyle(MycoColor.fg2)
            }
        }
        .buttonStyle(.plain)
        .accessibilityLabel(name)
        .accessibilityAddTraits(on ? .isSelected : [])
        .accessibilityIdentifier("background.tile")
    }

    /// A photo, made at most 2560 points on its long side, kept here.
    private func take(_ item: PhotosPickerItem) async {
        problem = nil
        guard let data = try? await item.loadTransferable(type: Data.self), let image = UIImage(data: data) else {
            problem = L("这张照片读不出来。", "This photo could not be read.")
            return
        }
        let side = max(image.size.width, image.size.height)
        let scale = side > 2560 ? 2560 / side : 1
        let size = CGSize(width: image.size.width * scale, height: image.size.height * scale)
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        let small = UIGraphicsImageRenderer(size: size, format: format).image { _ in image.draw(in: CGRect(origin: .zero, size: size)) }
        guard let jpeg = small.jpegData(compressionQuality: 0.82), let name = try? backgrounds.keep(jpeg) else {
            problem = L("没能存下这张照片。", "The photo could not be kept.")
            return
        }
        choice = ChatBackground(image: name, veil: choice.veil)
        follows = false
    }
}
