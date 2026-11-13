import XCTest

/// The risky parts of a conversation, in a simulator: where a long one
/// opens and how it scrolls, the keyboard coming and going, going back
/// with a swipe, and a task's steps.
final class ConversationTests: XCTestCase {
    static let lastReply = "这是最后一条回复。"

    @MainActor
    private func launch(_ arguments: [String]) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-myco-lang", "en", "-myco-appearance", "light"] + arguments
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

        // Saying something new brings the end back into view.
        input.tap()
        input.typeText("One more question")
        app.buttons["composer.submit"].tap()
        let mine = app.staticTexts["One more question"]
        XCTAssertTrue(mine.waitForExistence(timeout: 5), "what I said shows")
        XCTAssertTrue(mine.isHittable, "and the conversation is back at its end")
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
        try XCTSkipUnless(keyboard.waitForExistence(timeout: 5), "this simulator shows no software keyboard (a hardware keyboard is connected)")

        XCTAssertLessThan(input.frame.minY, restingY, "the input rides up with the keyboard")
        XCTAssertLessThanOrEqual(input.frame.maxY, keyboard.frame.minY + 1, "and sits on top of it")
        XCTAssertTrue(last.isHittable, "the newest message is still on screen")
        XCTAssertLessThanOrEqual(last.frame.maxY, input.frame.minY, "above the input")
        snap("keyboard-up")

        // Dragging the conversation down takes the keyboard with it.
        let list = app.scrollViews.firstMatch
        list.swipeDown(velocity: .slow)
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
        XCTAssertTrue(app.tabBars.buttons["Projects"].isHittable, "with the tab bar")
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
    func testWorkOthersGaveAnAgentIsOnItsPage() {
        let app = launch(["-myco-route", "agent:studio-codex-7f3a"])
        let capsule = app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "From 协调者")).firstMatch
        XCTAssertTrue(capsule.waitForExistence(timeout: 20), "the coordinator's task shows as one line")
        snap("agent-incoming")
    }
}
