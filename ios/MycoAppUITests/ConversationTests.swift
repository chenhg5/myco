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
        snap("files-preview")
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
    func testARunThatWaitsStandsStillAndSaysWhatFor() {
        // The notes helper set my task aside: it answers later.
        var app = launch(["-myco-start", "waiting", "-myco-route", "agent:studio-claude-c41f"])
        var paused = app.descendants(matching: .any)["run.paused"]
        XCTAssertTrue(paused.waitForExistence(timeout: 20), "an open run that is not at work says so")
        XCTAssertTrue(app.staticTexts["Set aside; it answers later"].exists)
        // It is free meanwhile: what I type is a new message, not an addition.
        let submit = app.descendants(matching: .any)["composer.submit"]
        XCTAssertEqual(submit.label, "Send")
        snap("waiting-held")
        app.terminate()

        // The coordinator handed work out and waits for it.
        app = launch(["-myco-start", "waiting", "-myco-route", "project:myco-ios"])
        paused = app.descendants(matching: .any)["run.paused"]
        XCTAssertTrue(paused.waitForExistence(timeout: 20))
        XCTAssertTrue(app.staticTexts["Waiting for 测试"].exists, "it names whom it waits for")
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
    func testAFileAboutToGoCanBeLookedAtFirst() {
        let app = launch(["-myco-route", "agent:studio-claude-c41f", "-myco-attach", "studio-claude-c41f"])
        let open = app.buttons.matching(identifier: "composer.file.open")
        XCTAssertTrue(open.firstMatch.waitForExistence(timeout: 20), "what was picked shows above the input")
        // A tap on the picture opens it full size, to see it is the right one.
        let submit = app.descendants(matching: .any)["composer.submit"]
        XCTAssertTrue(submit.isHittable)
        open.firstMatch.tap()
        // The system's preview covers the page.
        expectation(for: NSPredicate(format: "hittable == false"), evaluatedWith: submit)
        waitForExpectations(timeout: 10)
        XCTAssertTrue(app.descendants(matching: .any)["QLPreviewControllerView"].waitForExistence(timeout: 5), "it is the system's preview")
        snap("files-look")
        // Closed the way the system's preview is: a picture fills the
        // screen, a tap brings its bar, and the bar has a close button.
        let done = app.buttons["QLOverlayDoneButtonAccessibilityIdentifier"]
        if !done.exists { app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap() }
        XCTAssertTrue(done.waitForExistence(timeout: 5), "the preview can be closed")
        done.tap()
        // It is still waiting to go.
        expectation(for: NSPredicate(format: "hittable == true"), evaluatedWith: submit)
        waitForExpectations(timeout: 10)
        XCTAssertTrue(open.firstMatch.exists)
        XCTAssertTrue(submit.isEnabled)
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
    func testAMembersPageShowsTheWorkItsCoordinatorGaveIt() {
        let app = launch(["-myco-route", "agent:studio-codex-7f3a"])
        let task = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "设计 Hub 的推送接口")).firstMatch
        XCTAssertTrue(task.waitForExistence(timeout: 20), "the coordinator's task is part of the conversation")
        XCTAssertTrue(app.staticTexts["协调者"].exists, "and says who asked")
        // What it reported sits in its run where it came: after the step
        // made before it, never under what it did later.
        let first = app.staticTexts["看了现有的唤醒方式：webhook、GitHub、Claude 三种。"]
        let newest = app.staticTexts["正在写中转服务的设计。"]
        XCTAssertTrue(first.waitForExistence(timeout: 10), "its updates show while it works")
        XCTAssertTrue(newest.exists)
        let summaries = app.buttons.matching(identifier: "steps.summary")
        XCTAssertGreaterThanOrEqual(summaries.count, 2, "its steps are in more than one run of steps")
        XCTAssertLessThan(summaries.element(boundBy: 0).frame.minY, first.frame.minY, "the first steps are above its first update")
        XCTAssertGreaterThan(summaries.element(boundBy: summaries.count - 1).frame.minY, first.frame.minY, "and later steps are below it")
        snap("run-member")
        XCTAssertTrue(app.buttons["steps.summary"].firstMatch.exists, "with how it is being worked on")
        snap("member-all-work")
    }
}
