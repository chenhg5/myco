import XCTest

/// Walks the key paths of the navigation draft on the demo workspace: into
/// a project and a member and back, from Members to a chat, and a task
/// that is sent, added to, and stopped.
final class WalkthroughTests: XCTestCase {
    @MainActor
    private func launch(_ extra: [String] = []) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-myco-lang", "en", "-myco-appearance", "light"] + (extra.contains("-myco-start") ? extra : ["-myco-start", "demo"] + extra)
        app.launch()
        return app
    }

    @MainActor
    private func back(_ app: XCUIApplication) {
        app.navigationBars.buttons.element(boundBy: 0).tap()
    }

    @MainActor
    func testIntoAProjectAndAMemberAndBack() {
        let app = launch()
        let project = app.buttons["chat.project:myco-ios"]
        XCTAssertTrue(project.waitForExistence(timeout: 15), "Chats lists the project")
        project.tap()

        // The project is its coordinator's conversation, with the members above.
        let member = app.buttons["strip.studio-codex-7f3a"]
        XCTAssertTrue(member.waitForExistence(timeout: 5), "the project shows its members")
        XCTAssertTrue(app.buttons["composer.submit"].exists, "and an input")
        XCTAssertFalse(app.buttons["tab.members"].isHittable, "a pushed page hides the tab bar")
        member.tap()

        XCTAssertTrue(app.buttons["conversation.title"].waitForExistence(timeout: 5), "a member's page opens on top")
        back(app)
        XCTAssertTrue(member.waitForExistence(timeout: 5), "back leads to the project")
        back(app)
        XCTAssertTrue(project.waitForExistence(timeout: 5), "and back again to Chats")
        XCTAssertTrue(app.buttons["tab.members"].isHittable, "where the tab bar is back")
    }

    @MainActor
    func testFromMembersToAChatThenSendAddAndStop() {
        let app = launch()
        XCTAssertTrue(app.buttons["tab.members"].waitForExistence(timeout: 15))
        app.buttons["tab.members"].tap()
        let row = app.buttons["member.studio-codex-e07b"]
        XCTAssertTrue(row.waitForExistence(timeout: 5), "Members lists someone never talked to")
        row.tap()
        let chat = app.buttons["member.chat"]
        XCTAssertTrue(chat.waitForExistence(timeout: 5), "its card offers to message it")
        chat.tap()

        let input = app.descendants(matching: .any)["composer.input"]
        let submit = app.buttons["composer.submit"]
        XCTAssertTrue(input.waitForExistence(timeout: 5), "the conversation opens")
        XCTAssertEqual(submit.label, "Send")
        XCTAssertFalse(submit.isEnabled, "nothing typed: nothing to send")

        input.tap()
        input.typeText("Translate the readme")
        XCTAssertTrue(submit.isEnabled)
        submit.tap()
        XCTAssertTrue(app.staticTexts["Translate the readme"].waitForExistence(timeout: 5), "what I said shows")

        // A task is open and nothing is typed: the button stops it.
        let isStop = NSPredicate(format: "label == %@", "Stop")
        expectation(for: isStop, evaluatedWith: submit)
        waitForExpectations(timeout: 5)

        // Typing turns it into adding to the open task.
        input.typeText("British spelling")
        XCTAssertEqual(submit.label, "Add")
        submit.tap()
        XCTAssertTrue(app.staticTexts["British spelling"].waitForExistence(timeout: 5), "the addition shows")
        XCTAssertTrue(app.staticTexts["Added"].exists, "marked as an addition")

        expectation(for: isStop, evaluatedWith: submit)
        waitForExpectations(timeout: 5)
        submit.tap()
        XCTAssertTrue(app.staticTexts["Stopped"].waitForExistence(timeout: 5), "stopping says so")
        expectation(for: NSPredicate(format: "label == %@", "Send"), evaluatedWith: submit)
        waitForExpectations(timeout: 5)
    }

    @MainActor
    func testAMessageQueuedBehindARunningTaskWaitsAboveTheInput() {
        let app = launch(["-myco-route", "agent:studio-codex-e07b"])
        let input = app.descendants(matching: .any)["composer.input"]
        let submit = app.buttons["composer.submit"]
        XCTAssertTrue(input.waitForExistence(timeout: 15))
        input.tap()
        input.typeText("Translate the readme")
        submit.tap()
        XCTAssertTrue(app.staticTexts["Translate the readme"].waitForExistence(timeout: 5))

        // Something typed while it runs: a long press on the button offers
        // to send it after this one.
        input.typeText("Then the changelog")
        submit.press(forDuration: 1.0)
        let after = app.buttons["Send after this one"]
        XCTAssertTrue(after.waitForExistence(timeout: 3), "the long press offers to queue it")
        after.tap()
        let row = app.descendants(matching: .any)["queue.row"]
        XCTAssertTrue(row.waitForExistence(timeout: 5), "it waits above the input")
        XCTAssertFalse(app.staticTexts["Then the changelog"].exists, "and is not part of the conversation yet")

        // When the first is answered, the queued one becomes a message.
        XCTAssertTrue(app.staticTexts["Then the changelog"].waitForExistence(timeout: 20), "its turn comes when the first ends")
        XCTAssertFalse(row.exists, "and the queue is empty")
    }

    @MainActor
    func testTheWelcomePageLeadsIntoTheDemo() {
        let app = launch(["-myco-start", "welcome"])
        XCTAssertTrue(app.buttons["welcome.join"].waitForExistence(timeout: 15))
        app.buttons["welcome.demo"].tap()
        XCTAssertTrue(app.buttons["chat.project:myco-ios"].waitForExistence(timeout: 10), "the demo workspace opens on Chats")
    }

    @MainActor
    func testALinkThatIsNotOneSaysSo() {
        let app = launch(["-myco-start", "welcome"])
        XCTAssertTrue(app.buttons["welcome.join"].waitForExistence(timeout: 15))
        app.buttons["welcome.join"].tap()
        let field = app.descendants(matching: .any)["join.link"]
        XCTAssertTrue(field.waitForExistence(timeout: 5), "the join sheet opens")
        XCTAssertFalse(app.buttons["join.submit"].isEnabled, "nothing pasted: nothing to join with")
        field.tap()
        field.typeText("https://example.com/not-a-link")
        app.buttons["join.submit"].tap()
        XCTAssertTrue(app.staticTexts["join.problem"].waitForExistence(timeout: 5), "it says the text is not a link")
        XCTAssertTrue(field.exists, "and stays on the sheet")
    }

    @MainActor
    func testAnEmptyWorkspaceOffersToAddAnAgent() {
        let app = launch(["-myco-start", "empty"])
        XCTAssertTrue(app.staticTexts["Add an agent and start talking"].waitForExistence(timeout: 15))
        app.buttons["workspace.button"].tap()
        XCTAssertTrue(app.buttons["Join another workspace"].waitForExistence(timeout: 5), "the workspace panel opens")
    }

    @MainActor
    func testAddingAnAgentGivesASentenceWithARealLink() {
        let app = launch()
        XCTAssertTrue(app.buttons["chat.project:myco-ios"].waitForExistence(timeout: 15))
        app.buttons["Add an agent"].firstMatch.tap()
        let sentence = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "curl -fsSL https://demo.myco.example/j/")).firstMatch
        XCTAssertTrue(sentence.waitForExistence(timeout: 5), "the sentence carries the workspace's join link")
        XCTAssertTrue(app.buttons["Copy"].isEnabled)
    }
}
