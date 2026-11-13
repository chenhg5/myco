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
        let summary = app.buttons["steps.summary"].firstMatch
        XCTAssertTrue(summary.exists, "steps fold into one line")
        XCTAssertFalse(app.staticTexts["git status --short"].exists, "and never open by themselves")
        snap("process-folded")
        summary.tap()
        XCTAssertTrue(app.staticTexts["git status --short"].waitForExistence(timeout: 3), "a tap opens them")
        snap("process-open")
    }

    @MainActor
    func testAMembersPageShowsTheWorkItsCoordinatorGaveIt() {
        let app = launch(["-myco-route", "agent:studio-codex-7f3a"])
        let task = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "设计 Hub 的推送接口")).firstMatch
        XCTAssertTrue(task.waitForExistence(timeout: 20), "the coordinator's task is part of the conversation")
        XCTAssertTrue(app.staticTexts["协调者"].exists, "and says who asked")
        XCTAssertTrue(app.buttons["steps.summary"].firstMatch.exists, "with how it is being worked on")
        snap("member-all-work")
    }
}
