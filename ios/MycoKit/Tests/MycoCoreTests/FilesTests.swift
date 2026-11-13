import Foundation
import Testing
import MycoAPI
import MycoFake
@testable import MycoCore

/// Sending files with a message, and reading the ones messages carry.
@MainActor
struct FilesTests {
    static func folder() -> URL {
        FileManager.default.temporaryDirectory.appending(path: "myco-tests-" + UUID().uuidString)
    }

    func open(_ agent: String = "studio-claude-c41f") async -> (ConversationModel, FakeHub, WorkspaceSession) {
        let hub = FakeHub()
        let session = WorkspaceSession(id: "files", name: "Demo", hub: hub, files: Self.folder())
        await session.refresh()
        let model = session.conversation(with: agent)
        await model.reload()
        return (model, hub, session)
    }

    @Test func aFileGoesUpWhenPickedAndOutWithTheMessage() async throws {
        let (model, hub, _) = await open()
        #expect(model.canSubmit == false)
        await model.attach(SamplePicture.png(hue: 0.3, width: 300, height: 200), name: "照片.png", contentType: "image/png")
        await model.attach(Data("hello".utf8), name: "notes.md", contentType: "text/markdown")
        #expect(model.pending.map(\.name) == ["照片.png", "notes.md"])
        #expect(model.pending.allSatisfy { if case .ready = $0.state { return true } else { return false } })
        // A picture has something to show before it is sent; a text file has not.
        #expect(model.pending[0].preview != nil)
        #expect(model.pending[1].preview == nil)

        model.draft = "看看这两个"
        #expect(model.canSubmit)
        await model.submit()
        #expect(model.pending.isEmpty)
        #expect(model.draft.isEmpty)
        let sent = try #require(await hub.allMessages().last { $0.from == FakeHub.me && $0.to == "studio-claude-c41f" })
        #expect(sent.body == "看看这两个")
        #expect(sent.attachments.map(\.name) == ["照片.png", "notes.md"])
        // And the conversation shows them with what was said.
        let mine = try #require(model.items.last { $0.id == sent.id })
        #expect(mine.files.map(\.name) == ["照片.png", "notes.md"])
        #expect(mine.files[0].isPicture)
    }

    @Test func filesCanGoWithoutWords() async throws {
        let (model, hub, _) = await open()
        await model.attach(Data("a".utf8), name: "a.txt", contentType: "text/plain")
        #expect(model.canSubmit)
        #expect(model.action == .send)
        await model.submit()
        #expect(await hub.allMessages().last?.body == "发来一个文件：a.txt")

        // Several: all their names.
        await model.attach(Data("b".utf8), name: "b.txt", contentType: "text/plain")
        await model.attach(Data("c".utf8), name: "c.txt", contentType: "text/plain")
        #expect(ConversationModel.body("", files: model.pending) == "发来 2 个文件：b.txt、c.txt")
        #expect(ConversationModel.body("说明", files: model.pending) == "说明")
    }

    @Test func filesAddToATaskBeingWorkedOn() async throws {
        // Ops took a task two hours ago: what I send now adds to it.
        let (model, hub, _) = await open("vps-codex-90aa")
        #expect(model.openTask != nil)
        #expect(model.action == .stop)
        await model.attach(Data("log".utf8), name: "disk.log", contentType: "text/plain")
        // Something to send: the button adds, it does not stop.
        #expect(model.action == .steer)
        await model.submit()
        let steer = try #require(await hub.allMessages().last)
        #expect(steer.type == "steer")
        #expect(steer.attachments.map(\.name) == ["disk.log"])
        #expect(model.items.last { $0.id == steer.id }?.files.count == 1)
    }

    @Test func aFileBehindTheOpenTaskWaitsWithItsMessage() async throws {
        let (model, hub, _) = await open("vps-codex-90aa")
        let task = try #require(model.openTask)
        await model.attach(Data("x".utf8), name: "next.txt", contentType: "text/plain")
        model.draft = "下一件事"
        await model.queueNext()
        let queued = try #require(await hub.allMessages().last)
        #expect(queued.after == task.id)
        #expect(queued.attachments.map(\.name) == ["next.txt"])
        #expect(model.pending.isEmpty)
        #expect(model.waiting.map(\.body) == ["下一件事"])
    }

    @Test func aWorkspaceTooOldSaysSoAndSendsNothing() async {
        let (model, hub, _) = await open()
        await hub.setTooOldForFiles(true)
        await model.attach(Data("a".utf8), name: "a.txt", contentType: "text/plain")
        guard case .failed(let reason) = model.pending.first?.state else {
            Issue.record("the upload should have failed")
            return
        }
        #expect(reason.contains("升级"))
        // Nothing went up: with no words there is nothing to send.
        #expect(model.canSubmit == false)

        // The workspace is updated: trying again sends it.
        await hub.setTooOldForFiles(false)
        await model.retryPending(model.pending[0].id)
        #expect(model.canSubmit)

        model.removePending(model.pending[0].id)
        #expect(model.pending.isEmpty)
        #expect(model.canSubmit == false)
    }

    @Test func aFileTooLargeIsRefusedHere() async {
        let (model, _, _) = await open()
        await model.attach(Data(count: HubFile.maxSize + 1), name: "big.bin", contentType: "application/octet-stream")
        guard case .failed = model.pending.first?.state else {
            Issue.record("a file over the limit should not go up")
            return
        }
    }

    @Test func aFailedSendGivesTheFilesBack() async {
        let (model, hub, _) = await open()
        await model.attach(Data("a".utf8), name: "a.txt", contentType: "text/plain")
        model.draft = "带个文件"
        await hub.setUnreachable(true)
        await model.submit()
        #expect(model.problem != nil)
        #expect(model.draft == "带个文件")
        #expect(model.pending.map(\.name) == ["a.txt"])
        await hub.setUnreachable(false)
        await model.submit()
        #expect(model.problem == nil)
        #expect(model.pending.isEmpty)
    }

    @Test func aSealedMessageShowsNoFiles() {
        var sealed = Message(
            id: "m1", from: "@console:glenn", to: "coder", body: "myco-e2e:v1:abc", status: "done",
            attachments: [Attachment(fileId: "f1", name: "encrypted", size: 10)])
        sealed.e2e = true
        #expect(ConversationBuilder.files(of: sealed).isEmpty)
        let plain = Message(
            id: "m2", from: "@console:glenn", to: "coder", body: "看图", status: "done",
            attachments: [Attachment(fileId: "f2", name: "a.png", size: 10, contentType: "image/png")])
        #expect(ConversationBuilder.files(of: plain).count == 1)
    }

    @Test func aMessagesFileIsFetchedOnceAndKept() async throws {
        actor Counter {
            var calls = 0
            func hit() { calls += 1 }
        }
        let counter = Counter()
        let folder = Self.folder()
        let shelf = FileShelf(folder: folder) { id in
            await counter.hit()
            try await Task.sleep(for: .milliseconds(30))
            return Data(id.utf8)
        }
        let file = Attachment(fileId: "f_1", name: "../../etc/报告: v2.pdf", size: 3)
        // Asked for twice at once: one download.
        async let first = shelf.url(of: file)
        async let second = shelf.url(of: file)
        let (a, b) = try await (first, second)
        #expect(a == b)
        #expect(await counter.calls == 1)
        #expect(try Data(contentsOf: a) == Data("f_1".utf8))
        // Its own name, made safe: inside its folder, whatever it was called.
        #expect(a.lastPathComponent == "_.._etc_报告_ v2.pdf")
        #expect(a.deletingLastPathComponent().deletingLastPathComponent().path == folder.path)
        // Later: from this device.
        _ = try await shelf.url(of: file)
        #expect(await counter.calls == 1)

        // One that cannot be fetched fails, and can be asked for again.
        let broken = FileShelf(folder: folder) { _ in throw HubError(error: "not_found", message: "gone") }
        await #expect(throws: HubError.self) { _ = try await broken.url(of: Attachment(fileId: "f_2", name: "x", size: 1)) }
        // Leaving the workspace forgets them.
        FileShelf.forget("w", in: folder.deletingLastPathComponent())
    }

    @Test func whatISentIsAlreadyHere() async throws {
        let (model, hub, session) = await open()
        let bytes = SamplePicture.png(hue: 0.3, width: 200, height: 200)
        await model.attach(bytes, name: "a.png", contentType: "image/png")
        await model.submit()
        let sent = try #require(await hub.allMessages().last?.attachments.first)
        // The workspace is gone, the picture I just sent still shows.
        await hub.setUnreachable(true)
        let url = try await session.shelf.url(of: sent)
        #expect(try Data(contentsOf: url) == bytes)
    }

    @Test func aPictureGoesAsSomethingAgentsCanRead() throws {
        // A screenshot stays the PNG it is.
        let png = SamplePicture.png(hue: 0.5, width: 600, height: 1300)
        let kept = try #require(Pictures.prepared(png))
        #expect(kept.data == png)
        #expect(kept.fileExtension == "png")
        #expect(kept.contentType == "image/png")
        // A very large one is made a JPEG of sensible size.
        let huge = SamplePicture.png(hue: 0.5, width: 4800, height: 3600)
        let smaller = try #require(Pictures.prepared(huge))
        #expect(smaller.contentType == "image/jpeg")
        #expect(smaller.fileExtension == "jpg")
        let size = try #require(Pictures.pixelSize(of: smaller.data))
        #expect(max(size.width, size.height) == Pictures.longestSide)
        #expect(size.width > size.height)
        // What is not a picture is not made one.
        #expect(Pictures.prepared(Data("not a picture".utf8)) == nil)
        #expect(Pictures.thumbnail(Data("nope".utf8)) == nil)
        let thumb = try #require(Pictures.thumbnail(png))
        let small = try #require(Pictures.pixelSize(of: thumb))
        #expect(max(small.width, small.height) == 240)
    }

    @Test func sizesReadAsPeopleWriteThem() {
        #expect(byteCount(812) == "812 B")
        #expect(byteCount(3_500) == "3.4 KB")
        #expect(byteCount(250_000) == "244 KB")
        #expect(byteCount(12 * 1024 * 1024) == "12.0 MB")
    }
}
