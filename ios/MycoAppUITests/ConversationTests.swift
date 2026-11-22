import XCTest

/// The risky parts of a conversation, in a simulator: where a long one
/// opens and how it scrolls, the keyboard coming and going, going back
/// with a swipe, and a task's steps.
final class ConversationTests: XCTestCase {
    static let lastReply = "这是最后一条回复。"

    @MainActor
    private func launch(_ arguments: [String]) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-myco-lang", "en", "-myco-appearance", "light"] + (arguments.contains("-myco-start") ? arguments : ["-myco-start", "demo"] + arguments)
        app.launch()
        return app
    }

    /// Saves what is on screen to $MYCO_SHOTS_DIR, when set (to look at it).
    @MainActor
    private func snap(_ name: String) {
        guard let dir = ProcessInfo.processInfo.environment["MYCO_SHOTS_DIR"], !dir.isEmpty else { return }
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: URL(fileURLWithPath: dir).appendingPathComponent(name + ".png"))
    }

    /// Swipes from the left edge to the right: the system's "back".
    @MainActor
    private func swipeBack(_ app: XCUIApplication) {
        let from = app.coordinate(withNormalizedOffset: CGVector(dx: 0.01, dy: 0.5))
        from.press(forDuration: 0.05, thenDragTo: app.coordinate(withNormalizedOffset: CGVector(dx: 0.85, dy: 0.5)))
    }

    @MainActor
    func testMessagesShowTheirPicturesAndFiles() {
        let app = launch(["-myco-route", "agent:studio-claude-c41f"])
        // What I sent came with a picture; the answer with a file.
        let picture = app.buttons["attachment.picture"].firstMatch
        XCTAssertTrue(picture.waitForExistence(timeout: 20), "a picture shows as a thumbnail")
        let file = app.buttons["attachment.file"].firstMatch
        XCTAssertTrue(file.exists, "another file shows as a row")
        XCTAssertTrue(file.label.contains("会议结论.md"), "with its name")
        snap("files-in-messages")

        // The plus button offers where a file can come from.
        app.descendants(matching: .any)["composer.attach"].tap()
        XCTAssertTrue(app.buttons["Photo Library"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["Choose Files"].exists)
        snap("files-menu")
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.25)).tap()

        // A tap on a file opens it in the system's preview, under its own name.
        XCTAssertTrue(file.waitForExistence(timeout: 5))
        file.tap()
        XCTAssertTrue(
            app.staticTexts["会议结论.md"].waitForExistence(timeout: 10) || app.navigationBars["会议结论.md"].exists
                || app.staticTexts["会议结论"].exists,
            "the file opens")
        // Markdown is laid out, not shown as plain text; its source is a tap away.
        XCTAssertTrue(app.descendants(matching: .any)["markdown.rendered"].waitForExistence(timeout: 5), "laid out")
        snap("files-preview")
        app.segmentedControls["markdown.mode"].buttons["Source"].tap()
        XCTAssertTrue(app.descendants(matching: .any)["markdown.source"].waitForExistence(timeout: 5))
        app.buttons["markdown.close"].tap()
        XCTAssertFalse(app.descendants(matching: .any)["markdown.viewer"].waitForExistence(timeout: 2))
    }

    @MainActor
    func testPickedFilesWaitAboveTheInputAndGoWithTheMessage() {
        let app = launch(["-myco-route", "agent:studio-claude-c41f", "-myco-attach", "studio-claude-c41f"])
        let tiles = app.descendants(matching: .any).matching(identifier: "composer.file")
        let submit = app.descendants(matching: .any)["composer.submit"]
        XCTAssertTrue(tiles.firstMatch.waitForExistence(timeout: 20), "what was picked shows above the input")
        expectation(for: NSPredicate(format: "count == 2"), evaluatedWith: tiles)
        // Once they are in the workspace they can go, with no words at all.
        expectation(for: NSPredicate(format: "enabled == true"), evaluatedWith: submit)
        waitForExpectations(timeout: 10)
        snap("files-tray")

        // One is taken off again.
        app.buttons.matching(identifier: "composer.file.remove").element(boundBy: 1).tap()
        expectation(for: NSPredicate(format: "count == 1"), evaluatedWith: tiles)
        waitForExpectations(timeout: 5)

        let before = app.buttons.matching(identifier: "attachment.picture").count
        submit.tap()
        expectation(for: NSPredicate(format: "count == 0"), evaluatedWith: tiles)
        expectation(for: NSPredicate(format: "count == %d", before + 1), evaluatedWith: app.buttons.matching(identifier: "attachment.picture"))
        waitForExpectations(timeout: 10)
        snap("files-sent")
    }

    @MainActor
    func testSpeakingWritesIntoTheInput() {
        // The microphone hears a script here: nobody speaks to a simulator.
        let app = launch(["-myco-route", "agent:studio-claude-c41f", "-myco-dictation", "scripted"])
        let mic = app.buttons["composer.mic"]
        XCTAssertTrue(mic.waitForExistence(timeout: 20), "the microphone is in the input")
        let input = app.descendants(matching: .any)["composer.input"]
        // The controls sit under the words: what adds to a message on the
        // left, the button that sends it alone on the right, far enough
        // from the microphone that one is never hit for the other, and
        // each large enough to hit without aiming.
        let send = app.descendants(matching: .any)["composer.submit"]
        let attach = app.descendants(matching: .any)["composer.attach"]
        XCTAssertLessThan(mic.frame.midX, app.frame.midX, "the microphone is on the left")
        XCTAssertGreaterThan(send.frame.midX, app.frame.width * 0.8, "the send button is at the right edge")
        XCTAssertGreaterThan(send.frame.minX - mic.frame.maxX, 150, "a whole card apart")
        for control in [mic, send, attach] {
            XCTAssertGreaterThanOrEqual(control.frame.width, 44, "wide enough to hit")
            XCTAssertGreaterThanOrEqual(control.frame.height, 44, "tall enough to hit")
        }
        XCTAssertGreaterThanOrEqual(input.frame.width, app.frame.width * 0.8, "the words get the whole width")
        XCTAssertGreaterThan(send.frame.minY, input.frame.maxY - 1, "the controls are under the words")
        snap("composer-empty")
        mic.tap()
        // The words grow into the box, a new sentence after the one before.
        expectation(for: NSPredicate(format: "value CONTAINS %@", "帮我看一下下载页然后改一下标题"), evaluatedWith: input)
        waitForExpectations(timeout: 10)
        XCTAssertTrue(mic.isSelected, "it shows that it is listening")
        snap("dictation-listening")
        XCTAssertTrue(mic.isHittable, "the microphone stays there while there are words: speaking can go on")
        mic.tap()
        expectation(for: NSPredicate(format: "selected == false"), evaluatedWith: mic)
        waitForExpectations(timeout: 5)
        XCTAssertTrue((input.value as? String ?? "").contains("然后改一下标题"), "what was heard stays")
        XCTAssertTrue(app.descendants(matching: .any)["composer.submit"].isEnabled, "and can be sent")
    }

    @MainActor
    func testAMicrophoneThatMayNotListenSaysWhereToAllowIt() {
        let app = launch(["-myco-route", "agent:studio-claude-c41f", "-myco-dictation", "denied"])
        let mic = app.buttons["composer.mic"]
        XCTAssertTrue(mic.waitForExistence(timeout: 20))
        mic.tap()
        let note = app.descendants(matching: .any)["dictation.note"]
        XCTAssertTrue(note.waitForExistence(timeout: 5), "it says why it does not listen")
        XCTAssertTrue(app.buttons["Settings"].exists, "and where to allow it")
        snap("dictation-denied")
    }

    @MainActor
    func testAVoiceChatHearsAnswersAndPassesOn() {
        // Nobody speaks to a simulator and there is no key: the microphone
        // hears a script, a scripted model answers, the voice is silent.
        let app = launch(["-myco-route", "agent:studio-codex-e07b", "-myco-voice", "scripted"])
        let submit = app.descendants(matching: .any)["composer.submit"]
        XCTAssertTrue(submit.waitForExistence(timeout: 20))
        XCTAssertEqual(submit.label, "Voice chat", "nothing typed: the round button starts a voice chat")
        snap("voice-entry")
        submit.tap()

        // The call is a bar docked above the input.
        let bar = app.descendants(matching: .any)["voice.bar"]
        XCTAssertTrue(bar.waitForExistence(timeout: 5), "the call shows above the input")
        let input = app.descendants(matching: .any)["composer.input"]
        XCTAssertLessThan(bar.frame.maxY, input.frame.minY + 1, "above the words")
        XCTAssertFalse(app.buttons["composer.mic"].exists, "the microphone is the call's while it is on")
        XCTAssertNotEqual(submit.label, "Voice chat", "one call at a time")
        for control in [app.buttons["voice.mute"], app.buttons["voice.hangup"]] {
            XCTAssertGreaterThanOrEqual(control.frame.width, 44, "wide enough to hit")
            XCTAssertGreaterThanOrEqual(control.frame.height, 44, "tall enough to hit")
        }

        // A question about progress is answered by the voice itself.
        let answer = app.descendants(matching: .any).matching(identifier: "voice.line.voice").firstMatch
        XCTAssertTrue(answer.waitForExistence(timeout: 10), "the voice answers")
        expectation(for: NSPredicate(format: "label CONTAINS %@", "checking the links"), evaluatedWith: answer)
        waitForExpectations(timeout: 10)
        snap("voice-answer")

        // What is wanted goes to the agent as if typed, marked as said.
        let action = app.descendants(matching: .any).matching(identifier: "voice.line.action").firstMatch
        XCTAssertTrue(action.waitForExistence(timeout: 25), "the bar says what it passed on")
        let mark = app.buttons["voice.mark"].firstMatch
        XCTAssertTrue(mark.waitForExistence(timeout: 10), "the message in the conversation carries the voice mark")
        XCTAssertTrue(app.staticTexts["顺便把标题也改短一点"].exists, "and reads as what was asked, without the note under it")
        snap("voice-passed-on")
        mark.tap()
        XCTAssertTrue(app.staticTexts["As said: 顺便把标题也改短一点"].waitForExistence(timeout: 3), "a tap shows the words as heard")

        // Muting, then hanging up: the bar goes and the microphone is back.
        // (Once it has finished what it was saying, the bar says so.)
        app.buttons["voice.mute"].tap()
        expectation(for: NSPredicate(format: "label == %@", "Muted"), evaluatedWith: app.staticTexts["voice.state"])
        waitForExpectations(timeout: 25)
        snap("voice-muted")
        app.buttons["voice.hangup"].tap()
        expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: bar)
        waitForExpectations(timeout: 5)
        XCTAssertTrue(app.buttons["composer.mic"].waitForExistence(timeout: 3))
    }

    @MainActor
    func testAVoiceChatGoesOnWhenItsChatIsLeft() {
        let app = launch(["-myco-route", "agent:studio-codex-e07b", "-myco-voice", "scripted"])
        let submit = app.descendants(matching: .any)["composer.submit"]
        XCTAssertTrue(submit.waitForExistence(timeout: 20))
        submit.tap()
        let bar = app.descendants(matching: .any)["voice.bar"]
        XCTAssertTrue(bar.waitForExistence(timeout: 5))
        let strip = app.buttons["call.strip"]
        XCTAssertFalse(strip.exists, "in its own chat the call is the bar above the input")

        // Leaving the chat does not hang up: a strip at the top says how
        // the call goes, and with whom.
        swipeBack(app)
        XCTAssertTrue(strip.waitForExistence(timeout: 5), "the call goes on, in a strip at the top")
        XCTAssertTrue(strip.label.contains("翻译"), strip.label)
        XCTAssertFalse(bar.exists)
        XCTAssertTrue(app.buttons["call.strip.mute"].exists)
        XCTAssertTrue(app.buttons["call.strip.hangup"].exists)
        snap("call-strip")

        // In another chat the round button would take the call's place,
        // and says so; the microphone is the call's meanwhile.
        app.buttons["chat.agent:studio-claude-c41f"].tap()
        XCTAssertTrue(submit.waitForExistence(timeout: 10))
        XCTAssertTrue(submit.label.hasPrefix("Hang up with 翻译"), submit.label)
        XCTAssertFalse(app.buttons["composer.mic"].exists)
        XCTAssertTrue(strip.exists, "still on")
        snap("call-strip-elsewhere")

        // A tap on the strip goes back to the call's chat: the bar again.
        strip.tap()
        XCTAssertTrue(bar.waitForExistence(timeout: 10))
        expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: strip)
        waitForExpectations(timeout: 5)

        // Hung up from the strip, elsewhere: it goes.
        swipeBack(app)
        XCTAssertTrue(strip.waitForExistence(timeout: 5))
        app.buttons["call.strip.hangup"].tap()
        expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: strip)
        waitForExpectations(timeout: 5)
    }

    @MainActor
    func testWithoutVoiceSetUpTheButtonOnlySends() {
        // No key on this simulator: the round button is Send, as always.
        let app = launch(["-myco-route", "agent:studio-codex-e07b"])
        let submit = app.descendants(matching: .any)["composer.submit"]
        XCTAssertTrue(submit.waitForExistence(timeout: 20))
        XCTAssertEqual(submit.label, "Send")
        XCTAssertFalse(submit.isEnabled, "nothing typed: nothing to send, and no voice chat to start")
    }

    @MainActor
    func testAVoiceChatWithoutTheMicrophoneSaysWhereToAllowIt() {
        let app = launch(["-myco-route", "agent:studio-codex-e07b", "-myco-voice", "denied"])
        let submit = app.descendants(matching: .any)["composer.submit"]
        XCTAssertTrue(submit.waitForExistence(timeout: 20))
        submit.tap()
        let note = app.descendants(matching: .any)["dictation.note"]
        XCTAssertTrue(note.waitForExistence(timeout: 5), "it says why the call ended")
        XCTAssertTrue(app.buttons["Settings"].exists, "and where to allow it")
        XCTAssertFalse(app.descendants(matching: .any)["voice.bar"].exists)
    }

    @MainActor
    func testATaskThatWaitsReadsAsAMessageNotAState() {
        // The notes helper set my task aside: a few words under what I
        // asked say it answers later, and no line of its own says so.
        var app = launch(["-myco-start", "waiting", "-myco-route", "agent:studio-claude-c41f"])
        let receipt = app.staticTexts["message.receipt"]
        XCTAssertTrue(receipt.waitForExistence(timeout: 20), "a task not started has a few words under it")
        XCTAssertEqual(receipt.label, "Will answer later")
        XCTAssertFalse(app.staticTexts["Set aside; it answers later"].exists, "no line of state hangs under the task")
        // It is free meanwhile: what I type is a new message, not an addition.
        let submit = app.descendants(matching: .any)["composer.submit"]
        XCTAssertEqual(submit.label, "Send")
        snap("waiting-held")
        app.terminate()

        // The coordinator handed work out and its turn ended: what it said
        // last stands; nothing says "waiting for".
        app = launch(["-myco-start", "waiting", "-myco-route", "project:myco-ios"])
        XCTAssertTrue(app.descendants(matching: .any)["composer.input"].waitForExistence(timeout: 20))
        XCTAssertTrue(app.buttons["steps.summary"].firstMatch.waitForExistence(timeout: 20), "its run is on the page")
        XCTAssertFalse(app.staticTexts["Waiting for 测试"].exists)
        XCTAssertFalse(app.descendants(matching: .any)["message.receipt"].exists, "it began: nothing under what I asked")
        // Its turn is over: the input is a plain message box again, with
        // nothing to stop.
        XCTAssertEqual(app.descendants(matching: .any)["composer.submit"].label, "Send")
        snap("waiting-for")
    }

    @MainActor
    func testAMessageCanBeTakenOutOfMyViewAndBack() {
        let app = launch(["-myco-route", "agent:studio-claude-c41f"])
        let mine = app.staticTexts["帮我把昨天的会议记录整理成三条结论。"]
        XCTAssertTrue(mine.waitForExistence(timeout: 20))
        mine.press(forDuration: 1.0)
        let delete = app.buttons["Delete (from your view only)"]
        XCTAssertTrue(delete.waitForExistence(timeout: 5), "a long press offers to delete it")
        snap("delete-menu")
        delete.tap()
        expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: mine)
        waitForExpectations(timeout: 5)
        // For a moment it can be taken back.
        let undo = app.buttons["delete.undo"]
        XCTAssertTrue(undo.waitForExistence(timeout: 3))
        snap("delete-undo")
        undo.tap()
        XCTAssertTrue(mine.waitForExistence(timeout: 5), "undo brings it back")
    }

    @MainActor
    func testASlowConversationShowsItsShapeThenFadesIn() {
        // Every answer of the demo takes a while, as on a bad connection.
        let app = launch(["-myco-route", "agent:studio-claude-c41f", "-myco-latency", "1500"])
        let skeleton = app.descendants(matching: .any)["conversation.skeleton"]
        XCTAssertTrue(skeleton.waitForExistence(timeout: 20), "while it loads, the page shows the shape of a conversation")
        snap("loading-skeleton")
        let answer = app.buttons["attachment.file"].firstMatch
        XCTAssertTrue(answer.waitForExistence(timeout: 30), "then the conversation is there")
        expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: skeleton)
        waitForExpectations(timeout: 5)
        // Whole at once: how the task was worked on came with it.
        XCTAssertTrue(app.descendants(matching: .any)["plan.card"].firstMatch.exists, "with its plan and steps, not filled in later")
        snap("loading-done")
    }

    @MainActor
    func testWhatIsSentShowsAtOnceMarkedAsOnItsWay() {
        // (Every answer takes a while here, as on a slow connection.)
        let app = launch(["-myco-route", "agent:studio-codex-e07b", "-myco-latency", "2000"])
        let input = app.descendants(matching: .any)["composer.input"]
        XCTAssertTrue(input.waitForExistence(timeout: 30))
        // (Once the conversation itself has come: nothing was said in it yet.)
        XCTAssertTrue(app.staticTexts["What should 翻译 do for you?"].waitForExistence(timeout: 40))
        input.tap()
        input.typeText("Translate the readme")
        app.buttons["composer.submit"].tap()
        // On the page before the workspace has answered, and said to be on its way.
        let sending = app.descendants(matching: .any)["message.sending"]
        XCTAssertTrue(sending.waitForExistence(timeout: 1.5), "what I sent is marked as being sent")
        XCTAssertTrue(app.staticTexts["Translate the readme"].exists, "and shows at once")
        XCTAssertFalse((input.value as? String ?? "").contains("Translate"), "the input is empty again")
        XCTAssertEqual(app.keyboards.count, 0, "the keyboard is out of the way")
        snap("sending")
        // Then it is simply there.
        expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: sending)
        waitForExpectations(timeout: 20)
        XCTAssertTrue(app.staticTexts["Translate the readme"].exists, "it stays when the workspace has it")
    }

    @MainActor
    func testWhatTheAgentOffersToSayNextIsATapAway() {
        let app = launch(["-myco-route", "agent:studio-claude-c41f"])
        let input = app.descendants(matching: .any)["composer.input"]
        XCTAssertTrue(input.waitForExistence(timeout: 30))
        // The notes helper answered, and offers what I may want to say next.
        let offer = app.buttons["suggestion.row"]
        XCTAssertTrue(offer.waitForExistence(timeout: 20), "the agent's suggestion shows above the empty input")
        XCTAssertTrue(offer.label.contains("把这三条结论发到项目里"))
        snap("suggestion")
        offer.tap()
        // It is in the input now, to edit or send; the offer is gone.
        XCTAssertTrue((input.value as? String ?? "").contains("把这三条结论发到项目里"), "a tap writes it into the input")
        XCTAssertFalse(offer.exists, "with something in the input, nothing is offered")
        app.buttons["composer.submit"].tap()
        XCTAssertTrue(app.staticTexts["把这三条结论发到项目里"].waitForExistence(timeout: 10), "and it sends like anything typed")
        XCTAssertFalse(offer.waitForExistence(timeout: 2), "what was offered was for the task before")
    }

    @MainActor
    func testAFileAboutToGoCanBeLookedAtFirst() {
        let app = launch(["-myco-route", "agent:studio-claude-c41f", "-myco-attach", "studio-claude-c41f"])
        let open = app.buttons.matching(identifier: "composer.file.open")
        XCTAssertTrue(open.firstMatch.waitForExistence(timeout: 20), "what was picked shows above the input")
        // A tap on the picture opens it full size, to see it is the right one.
        let submit = app.descendants(matching: .any)["composer.submit"]
        XCTAssertTrue(submit.isHittable)
        open.firstMatch.tap()
        // The picture viewer covers the page.
        expectation(for: NSPredicate(format: "hittable == false"), evaluatedWith: submit)
        waitForExpectations(timeout: 10)
        let viewer = app.descendants(matching: .any)["picture.viewer"]
        XCTAssertTrue(viewer.waitForExistence(timeout: 5), "it is the picture viewer")
        XCTAssertTrue(app.buttons["picture.share"].exists, "it can be shared or saved")
        snap("files-look")
        // Its close button closes it.
        app.buttons["picture.close"].tap()
        // It is still waiting to go.
        expectation(for: NSPredicate(format: "hittable == true"), evaluatedWith: submit)
        waitForExpectations(timeout: 10)
        XCTAssertTrue(open.firstMatch.exists)
        XCTAssertTrue(submit.isEnabled)
        // A drag down closes it too, as in chat apps.
        open.firstMatch.tap()
        XCTAssertTrue(viewer.waitForExistence(timeout: 5))
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.35))
            .press(forDuration: 0.05, thenDragTo: app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.85)), withVelocity: .fast, thenHoldForDuration: 0)
        expectation(for: NSPredicate(format: "hittable == true"), evaluatedWith: submit)
        waitForExpectations(timeout: 10)
        XCTAssertFalse(viewer.exists)
    }

    @MainActor
    func testAMessagesPicturesOpenOverTheScreenAndCloseWithADragDown() {
        // The coordinator's answer has three sketches.
        let app = launch(["-myco-route", "agent:cloud-lead"])
        let picture = app.buttons["attachment.picture"].firstMatch
        XCTAssertTrue(picture.waitForExistence(timeout: 30))
        picture.tap()
        let viewer = app.descendants(matching: .any)["picture.viewer"]
        XCTAssertTrue(viewer.waitForExistence(timeout: 10))
        let count = app.staticTexts["picture.count"]
        XCTAssertTrue(count.waitForExistence(timeout: 5))
        XCTAssertEqual(count.label, "1 / 3")
        snap("picture-viewer")
        // Sideways: the next one.
        app.swipeLeft()
        expectation(for: NSPredicate(format: "label == %@", "2 / 3"), evaluatedWith: count)
        waitForExpectations(timeout: 5)
        // Down: back in the conversation.
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.35))
            .press(forDuration: 0.05, thenDragTo: app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.85)), withVelocity: .fast, thenHoldForDuration: 0)
        expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: viewer)
        waitForExpectations(timeout: 10)
        XCTAssertTrue(picture.isHittable)
    }

    @MainActor
    func testSearchFindsMessagesAndWordsInAConversation() {
        // The chats page: what is typed is looked for in messages too.
        let app = launch(["-myco-route", "tab:chats"])
        let field = app.searchFields.firstMatch
        XCTAssertTrue(field.waitForExistence(timeout: 20))
        field.tap()
        field.typeText("Tab")
        let hit = app.buttons.matching(identifier: "search.message").firstMatch
        XCTAssertTrue(hit.waitForExistence(timeout: 10), "a message that says it")
        snap("search")
        hit.tap()
        // Its conversation opens on it, finding those words.
        let count = app.staticTexts["find.count"]
        XCTAssertTrue(count.waitForExistence(timeout: 15))
        XCTAssertTrue(count.label.contains("/"), count.label)
        snap("search-in-conversation")
        app.buttons["find.done"].tap()
        XCTAssertFalse(count.exists)
        // From the conversation's menu.
        app.buttons["conversation.more"].tap()
        app.buttons["Search this conversation"].tap()
        let text = app.textFields["find.text"]
        XCTAssertTrue(text.waitForExistence(timeout: 5))
        text.typeText("底部")
        XCTAssertTrue(count.waitForExistence(timeout: 5))
    }

    @MainActor
    func testPullingUpAtTheEndReadsTheConversationAgain() {
        // (Every answer takes a while here, as on a slow connection: the
        // refresh is still going when the finger has let go.)
        let app = launch(["-myco-route", "agent:studio-claude-c41f", "-myco-latency", "1500"])
        XCTAssertTrue(app.descendants(matching: .any)["composer.input"].waitForExistence(timeout: 30))
        XCTAssertTrue(app.buttons["attachment.picture"].firstMatch.waitForExistence(timeout: 40), "the conversation is open")
        let spinner = app.descendants(matching: .any)["conversation.refreshing"]
        XCTAssertFalse(spinner.exists)
        // A finger pulls the end of the conversation up, past where it stops.
        let from = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.62))
        from.press(forDuration: 0.05, thenDragTo: app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.2)))
        XCTAssertTrue(spinner.waitForExistence(timeout: 3), "pulling up past the end refreshes, and shows that it does")
        snap("refresh-end")
        expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: spinner)
        waitForExpectations(timeout: 30)
        XCTAssertTrue(app.buttons["attachment.picture"].firstMatch.exists, "the conversation is still there")
    }

    @MainActor
    func testALongConversationOpensAtItsEndAndScrolls() {
        let app = launch(["-myco-start", "long", "-myco-route", "agent:studio-claude-c41f"])
        let last = app.staticTexts[Self.lastReply]
        XCTAssertTrue(last.waitForExistence(timeout: 20), "it opens at the newest message")
        XCTAssertTrue(last.isHittable, "which is on screen, above the input")
        let input = app.descendants(matching: .any)["composer.input"]
        XCTAssertLessThanOrEqual(last.frame.maxY, input.frame.minY, "not under the input")
        snap("long-bottom")

        // Up through the history: older messages come, the newest leaves.
        let list = app.scrollViews.firstMatch
        for _ in 0..<4 { list.swipeDown(velocity: .fast) }
        XCTAssertFalse(last.exists && last.isHittable, "scrolling up leaves the end behind")
        snap("long-scrolled-up")

        // Away from the end, a button leads back to it.
        let toEnd = app.buttons["conversation.toEnd"]
        XCTAssertTrue(toEnd.waitForExistence(timeout: 3), "a button offers the way back")
        toEnd.tap()
        expectation(for: NSPredicate(format: "hittable == true"), evaluatedWith: last)
        waitForExpectations(timeout: 5)
        XCTAssertFalse(toEnd.exists, "and goes away at the end")
        // The newest answer carries its row of actions.
        XCTAssertTrue(app.buttons["reply.copy"].exists, "the newest answer can be copied with one tap")
        for _ in 0..<3 { list.swipeDown(velocity: .fast) }

        // Saying something new brings the end back into view.
        input.tap()
        input.typeText("One more question")
        app.buttons["composer.submit"].tap()
        let mine = app.staticTexts["One more question"]
        XCTAssertTrue(mine.waitForExistence(timeout: 5), "what I said shows")
        XCTAssertTrue(mine.isHittable, "and the conversation is back at its end")
    }

    @MainActor
    func testScrollingToTheTopBringsOlderMessages() {
        let app = launch(["-myco-start", "long", "-myco-route", "agent:studio-claude-c41f"])
        XCTAssertTrue(app.staticTexts[Self.lastReply].waitForExistence(timeout: 20))
        // The newest page holds the last thirty questions (171 to 200).
        let older = app.staticTexts["第 150 个问题：这件事接下来怎么做？"]
        let list = app.scrollViews.firstMatch
        var swipes = 0
        while !older.exists, swipes < 40 {
            list.swipeDown(velocity: .fast)
            swipes += 1
        }
        XCTAssertTrue(older.exists, "scrolling past the first page loads the one before it")
    }

    @MainActor
    func testTheKeyboardComesAndGoesWithoutHidingTheEnd() throws {
        let app = launch(["-myco-start", "long", "-myco-route", "agent:studio-claude-c41f"])
        let last = app.staticTexts[Self.lastReply]
        XCTAssertTrue(last.waitForExistence(timeout: 20))
        let input = app.descendants(matching: .any)["composer.input"]
        let restingY = input.frame.minY
        input.tap()
        let keyboard = app.keyboards.firstMatch
        // With a hardware keyboard connected (the Simulator window open on a
        // Mac), the software keyboard stays below the screen: nothing to check.
        let onScreen = keyboard.waitForExistence(timeout: 5) && keyboard.frame.minY < app.frame.maxY - 20
        try XCTSkipUnless(onScreen, "this simulator shows no software keyboard (a hardware keyboard is connected)")

        XCTAssertLessThan(input.frame.minY, restingY, "the input rides up with the keyboard")
        XCTAssertLessThanOrEqual(input.frame.maxY, keyboard.frame.minY + 1, "and sits on top of it")
        XCTAssertTrue(last.isHittable, "the newest message is still on screen")
        XCTAssertLessThanOrEqual(last.frame.maxY, input.frame.minY, "above the input")
        snap("keyboard-up")

        // Dragging from the conversation down into the keyboard takes the
        // keyboard with it.
        let from = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.3))
        from.press(forDuration: 0.1, thenDragTo: app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.99)), withVelocity: .slow, thenHoldForDuration: 0.1)
        let gone = NSPredicate(format: "exists == false")
        expectation(for: gone, evaluatedWith: keyboard)
        waitForExpectations(timeout: 5)
        XCTAssertEqual(input.frame.minY, restingY, accuracy: 2, "the input is back where it rests")
        snap("keyboard-down")
    }

    @MainActor
    func testSwipingFromTheEdgeGoesBackOneLevelAtATime() {
        let app = launch(["-myco-route", "project:myco-ios"])
        let member = app.buttons["strip.studio-codex-7f3a"]
        XCTAssertTrue(member.waitForExistence(timeout: 20))
        member.tap()
        let title = app.buttons["conversation.title"]
        XCTAssertTrue(title.waitForExistence(timeout: 5), "the member's page is on top")

        swipeBack(app)
        XCTAssertTrue(member.waitForExistence(timeout: 5), "one swipe: back on the project")
        XCTAssertFalse(title.exists, "the member's page is gone")

        swipeBack(app)
        XCTAssertTrue(app.buttons["project.myco-ios"].waitForExistence(timeout: 5), "another: back on the list of projects")
        XCTAssertTrue(app.buttons["tab.projects"].isHittable, "with the tab bar")
    }

    @MainActor
    func testATaskShowsItsPlanAndFoldsItsSteps() {
        let app = launch(["-myco-route", "project:myco-ios"])
        XCTAssertTrue(app.descendants(matching: .any)["plan.card"].waitForExistence(timeout: 20), "the plan is a card")
        XCTAssertTrue(app.staticTexts["1 of 4 done"].exists, "that counts what is done")
        let summaries = app.buttons.matching(identifier: "steps.summary")
        XCTAssertTrue(summaries.firstMatch.exists, "steps fold into one line")
        XCTAssertFalse(app.staticTexts["git status --short"].exists, "and never open by themselves")
        // What it thought is part of the steps, not prose on the page.
        let thought = app.staticTexts["先看现在的代码怎么组织的，再决定改哪里。"]
        XCTAssertFalse(thought.exists, "thinking is folded away too")
        snap("process-folded")
        summaries.element(boundBy: 0).tap()
        XCTAssertTrue(thought.waitForExistence(timeout: 3), "a tap shows what it thought")
        summaries.element(boundBy: 1).tap()
        XCTAssertTrue(app.staticTexts["git status --short"].waitForExistence(timeout: 3), "and a tap opens the commands it ran")
        snap("process-open")
    }

    @MainActor
    func testAMessageThatWasNotDeliveredSaysSoAndSendsAgain() {
        // Ops took my message several times and never began on it.
        let app = launch(["-myco-start", "session", "-myco-route", "agent:vps-codex-90aa"])
        let asked = app.staticTexts.matching(NSPredicate(format: "label == %@", "把 Hub 昨天的日志打包发我。"))
        XCTAssertTrue(asked.firstMatch.waitForExistence(timeout: 20))
        let why = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "Not delivered: it picked this up several times")).firstMatch
        XCTAssertTrue(why.waitForExistence(timeout: 10), "what happened is said in words")
        XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "exhausted")).firstMatch.exists, "not as the workspace put it")
        snap("not-delivered")
        // One tap sends the same message again; the failed one stays.
        let again = app.buttons["Send again"]
        XCTAssertTrue(again.exists)
        XCTAssertEqual(asked.count, 1)
        again.tap()
        expectation(for: NSPredicate(format: "count == 2"), evaluatedWith: asked)
        waitForExpectations(timeout: 10)
        XCTAssertTrue(why.exists, "the one that failed still says so")
        snap("not-delivered-sent-again")
    }

    @MainActor
    func testALongSessionShowsWhatItDoesUnderMyNewerMessage() {
        // The notes helper took my second message and goes on in the same
        // session: its steps and its update are still filed under the first.
        let app = launch(["-myco-start", "session", "-myco-route", "agent:studio-claude-c41f"])
        let second = app.staticTexts["第二场也一起整理，放进同一份里。"]
        XCTAssertTrue(second.waitForExistence(timeout: 20))
        let update = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "第二场的录音也拿到了")).firstMatch
        XCTAssertTrue(update.waitForExistence(timeout: 20), "what it reported since is on the page")
        XCTAssertGreaterThan(update.frame.minY, second.frame.maxY, "under my newer message, not above it")
        let summaries = app.buttons.matching(identifier: "steps.summary").allElementsBoundByIndex
        XCTAssertTrue(summaries.contains { $0.frame.minY > second.frame.maxY }, "and so are the steps it took since")
        XCTAssertTrue(summaries.contains { $0.frame.maxY < second.frame.minY }, "what it did before stays under the first")
        snap("long-session")
    }

    @MainActor
    func testTheModelItRunsWithIsBesideTheSendButtonAndSwitchesThere() {
        // The backend engineer runs Codex on my computer: mine to switch.
        var app = launch(["-myco-route", "agent:studio-codex-7f3a"])
        let run = app.descendants(matching: .any)["composer.run"].firstMatch
        XCTAssertTrue(run.waitForExistence(timeout: 20), "the model is beside the send button")
        XCTAssertTrue(run.label.contains("GPT-6 Sol · High"), run.label)
        snap("run-choice")
        run.tap()
        // Two rows that open: the model, and the effort.
        let models = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Model")).firstMatch
        XCTAssertTrue(models.waitForExistence(timeout: 5), "a tap offers to switch the model")
        XCTAssertTrue(app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Effort")).firstMatch.exists, "and the effort")
        XCTAssertTrue(
            app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", "Now: GPT-6 Sol · High")).firstMatch.exists,
            "with what it runs on now, and when a switch applies")
        snap("run-choice-menu")
        models.tap()
        let other = app.buttons["GPT-6"]
        XCTAssertTrue(other.waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["Follow this computer"].exists)
        snap("run-choice-models")
        other.tap()
        expectation(for: NSPredicate(format: "label CONTAINS %@", "GPT-6 · High"), evaluatedWith: run)
        waitForExpectations(timeout: 10)
        snap("run-choice-switched")
        app.terminate()

        // The coordinator runs in the cloud: its own session decides.
        app = launch(["-myco-route", "agent:cloud-lead"])
        let theirs = app.descendants(matching: .any)["composer.run"].firstMatch
        XCTAssertTrue(theirs.waitForExistence(timeout: 20))
        XCTAssertTrue(theirs.label.contains("Opus 5.5"), theirs.label)
        theirs.tap()
        let note = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "/model")).firstMatch
        XCTAssertTrue(note.waitForExistence(timeout: 5), "a tap says where it is switched")
        snap("run-choice-note")
    }

    @MainActor
    func testAPageAnAgentPublishedIsACardThatOpensInTheApp() {
        // The tester made its checklist a web page and says where it is.
        let app = launch(["-myco-start", "session", "-myco-route", "agent:studio-claude-55d0"])
        let card = app.buttons["page.card"]
        XCTAssertTrue(card.waitForExistence(timeout: 20), "the link has a card under its message")
        XCTAssertTrue(card.label.contains("真机验证清单"), card.label)
        XCTAssertTrue(card.label.contains("Version 2"), card.label)
        XCTAssertTrue(card.label.contains("Published by 测试"), card.label)
        snap("page-card")
        card.tap()
        // Over the whole screen: its name and version, and the page itself.
        let title = app.descendants(matching: .any)["page.title"]
        XCTAssertTrue(title.waitForExistence(timeout: 10))
        XCTAssertTrue(title.label.contains("真机验证清单"), title.label)
        let web = app.webViews.firstMatch
        XCTAssertTrue(web.waitForExistence(timeout: 10))
        // Its own files came: the words, the style, and the script's count.
        XCTAssertTrue(web.staticTexts["第 2 版"].waitForExistence(timeout: 15), "the page's first file is shown")
        XCTAssertTrue(web.staticTexts["2 / 4"].waitForExistence(timeout: 10), "its script ran")
        snap("page-open")
        app.buttons["page.close"].tap()
        XCTAssertTrue(card.waitForExistence(timeout: 5), "closing it is back in the conversation")
    }

    @MainActor
    func testAConversationsPicturesAndFilesAreOnOnePage() {
        // The coordinator's conversation has three sketches in an answer.
        let app = launch(["-myco-route", "agent:cloud-lead"])
        let more = app.buttons["conversation.more"]
        XCTAssertTrue(more.waitForExistence(timeout: 20))
        more.tap()
        app.buttons["Files and apps"].tap()
        let message = app.descendants(matching: .any).matching(identifier: "files.message").firstMatch
        XCTAssertTrue(message.waitForExistence(timeout: 10), "each message with files, with who sent it")
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "协调者 · ")).firstMatch.exists)
        snap("chat-files")
        app.buttons["Done"].tap()
        XCTAssertTrue(more.waitForExistence(timeout: 5))
    }

    @MainActor
    func testWhatAConversationKeepsAtHandIsAboveIt() {
        // The coordinator's conversation keeps the repository and the
        // first decision pinned.
        let app = launch(["-myco-route", "agent:cloud-lead"])
        let message = app.buttons["pin.message"]
        XCTAssertTrue(message.waitForExistence(timeout: 20), "the pins are above the conversation")
        XCTAssertTrue(app.buttons["pin.link"].exists)
        snap("pins")
        message.tap()
        XCTAssertTrue(app.staticTexts["定了：底部三个 Tab（**聊天、项目、成员**）。".replacingOccurrences(of: "**", with: "")].exists
            || app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "定了")).firstMatch.exists, "a tap goes to the message")
        // Pinning an address from the menu.
        app.buttons["conversation.more"].tap()
        app.buttons["Pin an address"].tap()
        let address = app.textFields["pin.address"]
        XCTAssertTrue(address.waitForExistence(timeout: 5))
        address.tap()
        address.typeText("https://example.com/plan")
        app.textFields["pin.title"].tap()
        app.textFields["pin.title"].typeText("Plan")
        app.buttons["pin.save"].tap()
        let added = app.buttons.matching(NSPredicate(format: "label == %@", "Pinned: Plan")).firstMatch
        XCTAssertTrue(added.waitForExistence(timeout: 10), "it joins the strip")
        // A long press takes it away.
        added.press(forDuration: 1.0)
        app.buttons["Unpin"].tap()
        expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: added)
        waitForExpectations(timeout: 10)
    }

    @MainActor
    func testAConversationHasABackgroundOfItsOwn() {
        let app = launch(["-myco-route", "agent:studio-claude-c41f"])
        let more = app.buttons["conversation.more"]
        XCTAssertTrue(more.waitForExistence(timeout: 20))
        more.tap()
        app.buttons["Chat background"].tap()
        let follow = app.switches["background.follow"]
        XCTAssertTrue(follow.waitForExistence(timeout: 5))
        if (follow.value as? String) == "1" { follow.switches.firstMatch.exists ? follow.switches.firstMatch.tap() : follow.tap() }
        app.buttons["Mint"].tap()
        snap("background-sheet")
        app.buttons["background.done"].tap()
        XCTAssertTrue(more.waitForExistence(timeout: 5))
        snap("background-on")
        // Back to the default.
        more.tap()
        app.buttons["Chat background"].tap()
        XCTAssertTrue(follow.waitForExistence(timeout: 5))
        XCTAssertEqual(follow.value as? String, "0", "it has its own")
        follow.switches.firstMatch.exists ? follow.switches.firstMatch.tap() : follow.tap()
        app.buttons["background.done"].tap()
    }

    @MainActor
    func testAConversationIsExportedAsAFile() {
        let app = launch(["-myco-route", "agent:studio-claude-c41f"])
        let more = app.buttons["conversation.more"]
        XCTAssertTrue(more.waitForExistence(timeout: 20))
        more.tap()
        let entry = app.buttons["Export the conversation"]
        XCTAssertTrue(entry.waitForExistence(timeout: 5))
        entry.tap()
        let submit = app.buttons["export.submit"]
        XCTAssertTrue(submit.waitForExistence(timeout: 5))
        XCTAssertTrue(app.switches["export.steps"].exists, "the agents' steps can go along")
        snap("export-sheet")
        submit.tap()
        // Written, and handed to the system's share sheet, which covers
        // the page until it is closed.
        sleep(3)
        snap("export-share")
        let close = app.buttons.matching(NSPredicate(format: "identifier == %@ OR label == %@", "header.closeButton", "Close")).firstMatch
        if close.waitForExistence(timeout: 5) { close.tap() }
        let done = app.staticTexts["export.done"]
        XCTAssertTrue(done.waitForExistence(timeout: 10), "what was written is said")
        XCTAssertTrue(done.label.contains("Exported 2 messages"), done.label)
        XCTAssertTrue(done.label.hasSuffix(".md"), done.label)
    }

    @MainActor
    func testWhatAgentsHandEachOtherIsACardThatOpensTheirExchange() {
        let app = launch(["-myco-route", "agent:studio-codex-7f3a"])
        // The coordinator's task is not a message in my conversation with
        // the member: it is a card that says who gave what and how it stands.
        let card = app.buttons["handoff.d1"]
        XCTAssertTrue(card.waitForExistence(timeout: 20), "the coordinator's task is a handoff card")
        XCTAssertTrue(card.label.contains("设计 Hub 的推送接口"), card.label)
        XCTAssertTrue(card.label.contains("Working"), card.label)
        XCTAssertFalse(app.buttons["steps.summary"].firstMatch.exists, "how it is worked on is not part of my conversation with it")
        XCTAssertTrue(app.descendants(matching: .any)["composer.input"].exists, "my conversation with it has its input")
        snap("member-handoff")
        card.tap()

        // Their whole exchange, to read: what was asked, and how it is
        // being worked on. What it reported sits in its run where it came:
        // after the step made before it, never under what it did later.
        XCTAssertTrue(app.descendants(matching: .any)["exchange.title"].waitForExistence(timeout: 10), "the card opens their exchange")
        let first = app.staticTexts["看了现有的唤醒方式：webhook、GitHub、Claude 三种。"]
        let newest = app.staticTexts["正在写中转服务的设计。"]
        XCTAssertTrue(first.waitForExistence(timeout: 15), "its updates show while it works")
        XCTAssertTrue(newest.exists)
        let summaries = app.buttons.matching(identifier: "steps.summary")
        XCTAssertGreaterThanOrEqual(summaries.count, 2, "its steps are in more than one run of steps")
        XCTAssertLessThan(summaries.element(boundBy: 0).frame.minY, first.frame.minY, "the first steps are above its first update")
        XCTAssertGreaterThan(summaries.element(boundBy: summaries.count - 1).frame.minY, first.frame.minY, "and later steps are below it")
        XCTAssertFalse(app.descendants(matching: .any)["composer.input"].exists, "nothing is written into their exchange")
        snap("exchange")
        swipeBack(app)
        XCTAssertTrue(card.waitForExistence(timeout: 5), "back leads to my conversation with the member")
    }

    @MainActor
    func testAProjectShowsWhatWasHandedOutAndWhatCameBack() {
        let app = launch(["-myco-route", "project:myco-ios"])
        // Work the coordinator gave the tester, handed back: the card says
        // so, with the first line of what came back.
        let done = app.buttons["handoff.d3"]
        XCTAssertTrue(done.waitForExistence(timeout: 20), "what was handed out is a card")
        XCTAssertTrue(done.label.contains("Handed back"), done.label)
        XCTAssertTrue(app.buttons["handoff.d1"].exists)
        snap("project-handoffs")
    }
    @MainActor
    func testAPageLinkOpensWhereItPointsAndMovesWithinThePage() {
        // Opened from a link with a place in it: at that place, Chinese
        // and all, and the bridge gives the page its public address.
        var app = launch(["-myco-start", "links", "-myco-route", "page:pg_cinema#watch?source=src_1&movie=m42&line=线路一&episode=第3集"])
        var web = app.webViews.firstMatch
        XCTAssertTrue(web.staticTexts["Episode: 第3集 / 线路一"].waitForExistence(timeout: 20), web.debugDescription)
        XCTAssertTrue(web.staticTexts["URL: https://demo.myco.example/pages/pg_cinema"].waitForExistence(timeout: 10), "the public address, not the app's own")
        app.terminate()

        // From the chat: the card opens where its link points.
        app = launch(["-myco-start", "links", "-myco-route", "agent:vps-codex-90aa"])
        let card = app.buttons.matching(identifier: "page.card").firstMatch
        XCTAssertTrue(card.waitForExistence(timeout: 20))
        snap("page-link-card")
        card.tap()
        web = app.webViews.firstMatch
        XCTAssertTrue(web.staticTexts["Episode: 第1集 / 线路一"].waitForExistence(timeout: 20), web.debugDescription)
        XCTAssertTrue(web.staticTexts["Loads: 1"].exists)
        // A link in the page to another place in itself: the same document,
        // told by a hashchange.
        web.links["Next episode"].tap()
        XCTAssertTrue(web.staticTexts["Episode: 第2集 / 线路一"].waitForExistence(timeout: 10), web.debugDescription)
        XCTAssertTrue(web.staticTexts["Changes: 1"].exists)
        // The same link again: the page hears it again, and is not reloaded.
        web.links["Next episode"].tap()
        XCTAssertTrue(web.staticTexts["Changes: 2"].waitForExistence(timeout: 10), web.debugDescription)
        XCTAssertTrue(web.staticTexts["Loads: 1"].exists, "not reloaded")
        snap("page-link-episode")
        app.buttons["page.close"].tap()
        // The second link in the words: that episode.
        let links = app.links.matching(NSPredicate(format: "label CONTAINS %@", "pg_cinema"))
        XCTAssertTrue(links.firstMatch.waitForExistence(timeout: 10), app.debugDescription)
        XCTAssertEqual(links.count, 2)
        links.element(boundBy: 1).tap()
        XCTAssertTrue(web.staticTexts["Episode: 第2集 / 线路一"].waitForExistence(timeout: 20), web.debugDescription)
    }

    @MainActor
    func testRemoteControllerHasWebRTCNativeConsentAndDisconnectsInBackground() {
        let app = launch(["-myco-start", "remote", "-myco-route", "page:pg_remote"])
        let web = app.webViews.firstMatch
        XCTAssertTrue(web.buttons["Probe WebRTC"].waitForExistence(timeout: 20))
        web.buttons["Probe WebRTC"].tap()
        XCTAssertTrue(web.staticTexts["RTC ready"].waitForExistence(timeout: 20), "receive-only H.264/WebRTC must work in the actual App page origin")
        web.buttons["Connect test computer"].tap()
        let alert = app.alerts.firstMatch
        XCTAssertTrue(alert.waitForExistence(timeout: 10), web.debugDescription)
        XCTAssertTrue(alert.label.contains("Allow this remote connection?") || alert.staticTexts["Allow this remote connection?"].exists)
        XCTAssertTrue(alert.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Fixture Mac")).firstMatch.exists)
        alert.buttons["Don't allow"].tap()
        XCTAssertTrue(web.staticTexts["Denied"].waitForExistence(timeout: 10))
        web.buttons["Connect test computer"].tap()
        XCTAssertTrue(alert.waitForExistence(timeout: 10))
        alert.buttons["Connect"].tap()
        XCTAssertTrue(web.staticTexts["Connected test session"].waitForExistence(timeout: 10))
        XCTAssertTrue(app.buttons["page.close"].waitForNonExistence(timeout: 5), "the native page toolbar must not cover the remote desktop")
        let landscape = expectation(for: NSPredicate { _, _ in web.frame.width > web.frame.height }, evaluatedWith: nil)
        wait(for: [landscape], timeout: 10)
        XCTAssertTrue(app.buttons["remote.exit"].exists, "trusted App exit remains available over page content")
        snap("remote-native-landscape")
        XCUIDevice.shared.press(.home)
        app.activate()
        XCTAssertTrue(web.staticTexts["Disconnected"].waitForExistence(timeout: 10))
        XCTAssertTrue(app.buttons["page.close"].waitForExistence(timeout: 10), "disconnect restores the normal App chrome")
        web.buttons["Connect test computer"].tap()
        XCTAssertTrue(alert.waitForExistence(timeout: 10))
        alert.buttons["Connect"].tap()
        XCTAssertTrue(web.staticTexts["Connected test session"].waitForExistence(timeout: 10))
        web.buttons["Attempt host action"].tap()
        XCTAssertTrue(web.staticTexts["Host refused"].waitForExistence(timeout: 10))
        web.buttons["Disconnect test session"].tap()
        XCTAssertTrue(app.buttons["page.close"].waitForExistence(timeout: 10))
        web.buttons["Connect test computer"].tap()
        XCTAssertTrue(alert.waitForExistence(timeout: 10))
        alert.buttons["Connect"].tap()
        XCTAssertTrue(app.buttons["remote.exit"].waitForExistence(timeout: 10))
        let draft = web.textViews["Local draft"]
        XCTAssertTrue(draft.waitForExistence(timeout: 5))
        let fullDesktop = web.frame
        draft.tap()
        draft.typeText("Unsent draft")
        let exit = app.buttons["remote.exit"]
        let moved = expectation(for: NSPredicate { _, _ in exit.frame.midX < web.frame.midX }, evaluatedWith: nil)
        wait(for: [moved], timeout: 5)
        XCTAssertEqual(web.frame.height, fullDesktop.height, accuracy: 1, "the software keyboard overlays the full-height remote WebView")
        XCTAssertEqual(web.frame.minY, fullDesktop.minY, accuracy: 1, "typing must not push the remote WebView upwards")
        snap("remote-keyboard-exit-away-from-send")
        exit.tap()
        XCTAssertTrue(alert.buttons["Stay connected"].waitForExistence(timeout: 5))
        alert.buttons["Stay connected"].tap()
        XCTAssertTrue(web.exists, "an accidental tap must not end the connection")
        XCTAssertTrue(web.staticTexts["Connected test session"].exists)
        XCTAssertEqual(draft.value as? String, "Unsent draft", "cancel preserves the local draft")
        app.buttons["remote.exit"].tap()
        XCTAssertTrue(alert.buttons["Disconnect and exit"].waitForExistence(timeout: 5))
        alert.buttons["Disconnect and exit"].tap()
        XCTAssertTrue(web.waitForNonExistence(timeout: 10), "confirmed exit closes a connected remote page")
    }

}
