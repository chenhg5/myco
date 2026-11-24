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
        XCTAssertFalse(app.buttons["tab.workbench"].isHittable, "a pushed page hides the tab bar")
        member.tap()

        XCTAssertTrue(app.buttons["conversation.title"].waitForExistence(timeout: 5), "a member's page opens on top")
        back(app)
        XCTAssertTrue(member.waitForExistence(timeout: 5), "back leads to the project")
        back(app)
        XCTAssertTrue(project.waitForExistence(timeout: 5), "and back again to Chats")
        XCTAssertTrue(app.buttons["tab.workbench"].isHittable, "where the tab bar is back")
    }

    @MainActor
    func testFromMembersToAChatThenSendAddAndStop() {
        let app = launch()
        // Members: from the chats page's top bar.
        XCTAssertTrue(app.buttons["chrome.members"].waitForExistence(timeout: 15))
        app.buttons["chrome.members"].tap()
        let row = app.buttons["member.studio-codex-e07b"]
        XCTAssertTrue(row.waitForExistence(timeout: 5), "Members lists someone never talked to")
        row.tap()
        let chat = app.buttons["member.chat"]
        XCTAssertTrue(chat.waitForExistence(timeout: 5), "its card offers to message it")
        chat.tap()

        let input = app.descendants(matching: .any)["composer.input"]
        let submit = app.buttons["composer.submit"]
        XCTAssertTrue(input.waitForExistence(timeout: 5), "the conversation opens")
        // (Voice is not set up here, so the button is not a voice chat.)
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

        // Typing turns it into adding to the open task. (Sending put the
        // keyboard away: a tap in the input brings it back.)
        input.tap()
        input.typeText("British spelling")
        XCTAssertEqual(submit.label, "Add")
        submit.tap()
        XCTAssertTrue(app.staticTexts["British spelling"].waitForExistence(timeout: 5), "the addition shows")
        XCTAssertFalse(app.staticTexts["Added"].exists, "like anything else said: it carries no label")

        expectation(for: isStop, evaluatedWith: submit)
        waitForExpectations(timeout: 5)
        submit.tap()
        XCTAssertTrue(app.staticTexts["Stopped"].waitForExistence(timeout: 5), "stopping says so")
        expectation(for: NSPredicate(format: "label == %@", "Send"), evaluatedWith: submit)
        waitForExpectations(timeout: 5)
    }

    @MainActor
    func testPullingAListDownReadsItAgain() {
        // (Every answer takes a moment here, as on a real connection.)
        let app = launch(["-myco-latency", "600"])
        let chat = app.buttons["chat.project:myco-ios"]
        XCTAssertTrue(chat.waitForExistence(timeout: 30))
        let from = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.3))
        from.press(forDuration: 0.05, thenDragTo: app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.75)))
        // The system's spinner turns under the search field while the
        // workspace is asked. (This test cannot see it: the pull only
        // returns once the app is still again. `RefreshTests` covers the
        // refresh itself.)
        shot("refresh-list")
        XCTAssertTrue(chat.waitForExistence(timeout: 30), "the list is there after the refresh")
    }

    @MainActor
    func testVoiceIsSetUpInSettings() {
        // Settings is where the model and the voice of a voice chat go.
        let app = launch()
        XCTAssertTrue(app.buttons["workspace.button"].waitForExistence(timeout: 15))
        app.buttons["workspace.button"].tap()
        app.buttons["Settings"].tap()
        let voice = app.descendants(matching: .any)["settings.voice"]
        // (It is further down the page than a sheet shows at first.)
        XCTAssertTrue(app.switches["settings.notifications"].waitForExistence(timeout: 5))
        for _ in 0..<4 where !voice.exists { app.swipeUp() }
        XCTAssertTrue(voice.waitForExistence(timeout: 5), "Settings has a line for voice")
        voice.tap()
        XCTAssertTrue(app.textFields["voice.baseURL"].waitForExistence(timeout: 5), "the model's address")
        XCTAssertTrue(app.textFields["voice.model"].exists)
        XCTAssertTrue(app.secureTextFields["voice.modelKey"].exists, "keys are typed blind")
        XCTAssertTrue(app.secureTextFields["voice.speechKey"].exists)
        // The speech service is a choice, not a fixed one.
        XCTAssertTrue(app.descendants(matching: .any)["voice.service"].exists, "the speech service is chosen from a list")
        XCTAssertFalse(app.staticTexts["Voice (Doubao speech synthesis)"].exists, "no service is written into the heading")
        shot("voice-settings")
        // The settings can be kept on the workspaces for the other devices.
        let share = app.switches["voice.share"]
        for _ in 0..<3 where !share.exists { app.swipeUp() }
        XCTAssertTrue(share.waitForExistence(timeout: 5), "sharing with the person's other devices is a switch")
        XCTAssertEqual(share.value as? String, "1", "on unless turned off")
        shot("voice-settings-share")
    }

    /// Needs four forwarding proxies listening on this machine (HTTP,
    /// HTTP with the password me:pw, SOCKS5, SOCKS5 with me:pw, on
    /// consecutive ports from MYCO_PROXY_PORT) and an https address they
    /// can reach (MYCO_PROXY_TARGET); skipped without.
    @MainActor
    func testAProxySetHereIsTriedAndSaved() throws {
        let environment = ProcessInfo.processInfo.environment
        guard let base = Int(environment["MYCO_PROXY_PORT"] ?? ""), let target = environment["MYCO_PROXY_TARGET"], !target.isEmpty else {
            throw XCTSkip("no proxies to try (MYCO_PROXY_PORT, MYCO_PROXY_TARGET)")
        }
        let app = launch(["-myco-proxy-target", target])
        XCTAssertTrue(app.buttons["workspace.button"].waitForExistence(timeout: 15))
        app.buttons["workspace.button"].tap()
        app.buttons["Settings"].tap()
        let row = app.descendants(matching: .any)["settings.proxy"]
        XCTAssertTrue(app.switches["settings.notifications"].waitForExistence(timeout: 5))
        for _ in 0..<4 where !row.exists { app.swipeUp() }
        XCTAssertTrue(row.waitForExistence(timeout: 5), "Settings has a line for the proxy")
        row.tap()
        let mode = app.segmentedControls["proxy.mode"]
        XCTAssertTrue(mode.waitForExistence(timeout: 5))
        XCTAssertFalse(app.textFields["proxy.host"].exists, "as the system has it: nothing to fill in")
        mode.buttons["Set here"].tap()
        let host = app.textFields["proxy.host"], port = app.textFields["proxy.port"]
        XCTAssertTrue(host.waitForExistence(timeout: 5))
        let kind = app.segmentedControls["proxy.kind"]
        let trial = app.buttons["proxy.try"], tried = app.staticTexts["proxy.tried"]
        // (Whatever an earlier run left in a field goes first.)
        func write(_ text: String, into field: XCUIElement) {
            field.tap()
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 40) + text)
        }
        write("127.0.0.1", into: host)
        let user = app.textFields["proxy.user"], password = app.secureTextFields["proxy.password"]
        for _ in 0..<2 where !user.isHittable { app.swipeUp() }
        write("", into: user)
        func setPort(_ number: Int) {
            for _ in 0..<3 where !port.isHittable { app.swipeDown() }
            write(String(number), into: port)
        }
        func choose(_ name: String) {
            let segment = kind.buttons[name]
            // (Clear of the title bar: under it, a tap lands on the bar.)
            for _ in 0..<4 where !segment.isHittable || segment.frame.minY < 170 { app.swipeDown() }
            segment.tap()
            XCTAssertTrue(segment.isSelected, "the proxy's type is \(name)")
        }
        func expectTried(_ words: String, _ message: String) {
            for _ in 0..<3 where !trial.isHittable { app.swipeUp() }
            trial.tap()
            XCTAssertTrue(tried.waitForExistence(timeout: 20), message)
            XCTAssertTrue(tried.label.contains(words), message + ": " + tried.label)
        }
        // SOCKS5, then HTTP, each through a proxy that is really there.
        setPort(base + 2)
        expectTried("Connected in", "a SOCKS5 proxy is tried before it is saved")
        shot("proxy-socks5")
        choose("HTTP")
        setPort(base)
        expectTried("Connected in", "an HTTP proxy too")
        // One that wants a password says so; with it, it connects.
        setPort(base + 1)
        expectTried("wants a user name and password", "a proxy that wants a password says so")
        for _ in 0..<2 where !password.isHittable { app.swipeUp() }
        write("me", into: user)
        write("pw", into: password)
        expectTried("Connected in", "with its user name and password it connects")
        choose("SOCKS5")
        setPort(base + 3)
        expectTried("Connected in", "and so does a SOCKS5 proxy with a password")
        // Nothing listens there: said in words.
        setPort(base + 9)
        for _ in 0..<3 where !trial.isHittable { app.swipeUp() }
        trial.tap()
        XCTAssertTrue(tried.waitForExistence(timeout: 20))
        XCTAssertFalse(tried.label.contains("Connected"), "a proxy that is not there does not connect: \(tried.label)")
        shot("proxy-refused")

        // Saved: in force at once, and Settings says which.
        setPort(base + 3)
        for _ in 0..<3 where !app.buttons["proxy.save"].isHittable { app.swipeUp() }
        app.buttons["proxy.save"].tap()
        XCTAssertTrue(app.staticTexts["proxy.saved"].waitForExistence(timeout: 10), "saved, and said to be in effect")
        shot("proxy-saved")
        back(app)
        XCTAssertTrue(row.waitForExistence(timeout: 5))
        XCTAssertTrue(row.label.contains("SOCKS5 127.0.0.1:\(base + 3)"), row.label)
        // (Back to the system's, so nothing of this test stays.)
        row.tap()
        XCTAssertTrue(mode.waitForExistence(timeout: 5))
        mode.buttons["System"].tap()
        app.buttons["proxy.save"].tap()
        XCTAssertTrue(app.staticTexts["proxy.saved"].waitForExistence(timeout: 10))
        back(app)
        XCTAssertTrue(row.waitForExistence(timeout: 5))
        XCTAssertTrue(row.label.contains("System"), row.label)
    }

    @MainActor
    func testAVoiceIsPickedFromTheListAndTried() {
        // (A scripted voice: trying it needs no key that works and makes no sound.)
        let app = launch(["-myco-voice", "scripted"])
        XCTAssertTrue(app.buttons["workspace.button"].waitForExistence(timeout: 15))
        app.buttons["workspace.button"].tap()
        app.buttons["Settings"].tap()
        let voice = app.descendants(matching: .any)["settings.voice"]
        XCTAssertTrue(app.switches["settings.notifications"].waitForExistence(timeout: 5))
        for _ in 0..<4 where !voice.exists { app.swipeUp() }
        XCTAssertTrue(voice.waitForExistence(timeout: 5))
        voice.tap()
        // With a key for the speech service, its voice is chosen from a list.
        let key = app.secureTextFields["voice.speechKey"]
        XCTAssertTrue(key.waitForExistence(timeout: 5))
        key.tap()
        key.typeText("k")
        let row = app.descendants(matching: .any)["voice.speaker"]
        for _ in 0..<2 where !row.exists { app.swipeUp() }
        XCTAssertTrue(row.waitForExistence(timeout: 5), "the voice is a row that opens the list")
        XCTAssertTrue(row.label.contains("温暖阿虎"), "the default voice goes by its name: \(row.label)")
        row.tap()
        XCTAssertTrue(app.staticTexts["Chinese"].waitForExistence(timeout: 5), "voices are grouped by language")
        shot("voice-list")
        let vivi = app.buttons["voice.pick.zh_female_vv_uranus_bigtts"]
        XCTAssertTrue(vivi.waitForExistence(timeout: 5))
        vivi.tap()
        XCTAssertTrue(row.waitForExistence(timeout: 5))
        expectation(for: NSPredicate(format: "label CONTAINS 'Vivi'"), evaluatedWith: row)
        waitForExpectations(timeout: 5)
        // One line in that voice, before anything is kept.
        let trial = app.buttons["voice.try"]
        XCTAssertTrue(trial.exists)
        trial.tap()
        XCTAssertFalse(app.descendants(matching: .any)["voice.tryProblem"].waitForExistence(timeout: 2), "it could be tried")
        shot("voice-picked")
        // (Nothing of this test's key is kept.)
        for _ in 0..<2 where !key.isHittable { app.swipeDown() }
        key.tap()
        key.typeText(XCUIKeyboardKey.delete.rawValue)
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
        input.tap()
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
        // The logo: the pixel mushroom and the wordmark, read as "myco".
        let logo = app.descendants(matching: .any)["welcome.logo"]
        XCTAssertTrue(logo.exists, "the welcome page shows the logo")
        XCTAssertEqual(logo.label, "myco")
        XCTAssertGreaterThan(logo.frame.width, logo.frame.height * 3, "mushroom and wordmark side by side")
        shot("welcome")
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
    func testNotificationsCanBeSetAndAChatMuted() {
        // Nothing reaches the system here: the test only sets what the person chooses.
        // (`-myco-notifications` hides what an earlier run chose: an argument
        // comes before what the app saved, and is not settings it can read,
        // so every run starts with nothing muted and notifications off.)
        let app = launch(["-myco-notifier", "recorded", "-myco-notifications", "fresh"])
        XCTAssertTrue(app.buttons["chat.project:myco-ios"].waitForExistence(timeout: 15))
        // A long press on a chat mutes it: a crossed bell by its name.
        let chat = app.buttons["chat.agent:studio-claude-c41f"]
        chat.press(forDuration: 1.0)
        XCTAssertTrue(app.buttons["Mute"].waitForExistence(timeout: 5), "a chat can be muted")
        app.buttons["Mute"].tap()
        XCTAssertTrue(app.images["row.muted"].waitForExistence(timeout: 5), "and shows that it is")
        shot("notifications-muted")

        // Settings: on, with quiet hours.
        app.buttons["workspace.button"].tap()
        app.buttons["Settings"].tap()
        let notify = app.switches["settings.notifications"]
        XCTAssertTrue(notify.waitForExistence(timeout: 5))
        XCTAssertFalse(app.switches["settings.quietHours"].exists, "quiet hours only matter once notifications are on")
        notify.switches.firstMatch.tap()
        let quiet = app.switches["settings.quietHours"]
        XCTAssertTrue(quiet.waitForExistence(timeout: 5), "turned on, the rest shows")
        quiet.switches.firstMatch.tap()
        XCTAssertTrue(app.staticTexts["From"].waitForExistence(timeout: 5), "quiet hours have a start and an end")
        XCTAssertTrue(app.staticTexts["To"].exists)
        shot("notifications-settings")
    }

    @MainActor
    func testSeveralWorkspacesSideBySide() {
        // Two workspaces are joined; a few seconds in, something new comes in the second.
        let app = launch(["-myco-start", "two"])
        let button = app.buttons["workspace.button"]
        XCTAssertTrue(app.buttons["chat.project:myco-ios"].waitForExistence(timeout: 15), "it opens on the first one")
        XCTAssertTrue(button.label.contains("团队"))
        sleep(6)
        button.tap()
        let rows = app.buttons.matching(identifier: "workspace.row")
        XCTAssertTrue(rows.firstMatch.waitForExistence(timeout: 5), "the panel lists them")
        XCTAssertEqual(rows.count, 2)
        XCTAssertTrue(app.descendants(matching: .any)["workspace.unread"].waitForExistence(timeout: 10), "the other one shows that something new came")
        shot("workspaces-list")

        // A tap switches.
        rows.element(boundBy: 1).tap()
        expectation(for: NSPredicate(format: "label CONTAINS %@", "个人项目"), evaluatedWith: button)
        waitForExpectations(timeout: 10)
        XCTAssertTrue(app.buttons["tab.chats"].exists, "on its own first page")

        // Joining another keeps both.
        button.tap()
        app.buttons["workspace.join"].tap()
        let field = app.descendants(matching: .any)["join.link"]
        XCTAssertTrue(field.waitForExistence(timeout: 5))
        field.tap()
        field.typeText("https://third.myco.example/c/myco_pair_demo")
        app.buttons["join.submit"].tap()
        expectation(for: NSPredicate(format: "label CONTAINS %@", "third.myco.example"), evaluatedWith: button)
        waitForExpectations(timeout: 10)
        button.tap()
        XCTAssertTrue(rows.firstMatch.waitForExistence(timeout: 5))
        XCTAssertEqual(rows.count, 3, "the new one is added, the others stay")
        shot("workspaces-three")

        // Leaving the one on screen asks first, then shows another.
        app.buttons["Leave this workspace"].tap()
        let leave = app.buttons["Leave"]
        XCTAssertTrue(leave.waitForExistence(timeout: 5), "leaving asks once more")
        leave.tap()
        expectation(for: NSPredicate(format: "label CONTAINS %@", "团队"), evaluatedWith: button)
        waitForExpectations(timeout: 10)
        button.tap()
        XCTAssertTrue(rows.firstMatch.waitForExistence(timeout: 5))
        XCTAssertEqual(rows.count, 2)
    }

    /// Saves what is on screen to $MYCO_SHOTS_DIR, when set (to look at it).
    @MainActor
    private func shot(_ name: String) {
        guard let dir = ProcessInfo.processInfo.environment["MYCO_SHOTS_DIR"], !dir.isEmpty else { return }
        try? XCUIScreen.main.screenshot().pngRepresentation.write(to: URL(fileURLWithPath: dir).appendingPathComponent(name + ".png"))
    }

    @MainActor
    func testAddingAnAgentGivesASentenceWithARealLink() {
        let app = launch()
        XCTAssertTrue(app.buttons["chat.project:myco-ios"].waitForExistence(timeout: 15))
        // (the plus offers an agent, or a computer)
        app.buttons["chrome.add"].firstMatch.tap()
        app.buttons["Add an agent"].firstMatch.tap()
        let sentence = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "curl -fsSL https://demo.myco.example/j/")).firstMatch
        XCTAssertTrue(sentence.waitForExistence(timeout: 5), "the sentence carries the workspace's join link")
        XCTAssertTrue(app.buttons["Copy"].isEnabled)
    }

    @MainActor
    func testAnAgentIsHandedOverToAnotherSession() {
        // The backend engineer's card, for whoever may manage it: a way
        // to hand it over, and the time it was (with a way back).
        let app = launch(["-myco-start", "session", "-myco-route", "member:studio-codex-7f3a"])
        let entry = app.buttons["member.handover"]
        for _ in 0..<4 where !entry.exists { app.swipeUp() }
        XCTAssertTrue(entry.waitForExistence(timeout: 15))
        let history = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "claude.ai cloud → Studio Mac · Codex")).firstMatch
        XCTAssertTrue(history.waitForExistence(timeout: 10), "the earlier handover is in its card")
        XCTAssertTrue(app.buttons["member.handover.back"].exists, "with a way back")
        shot("handover-card")
        entry.tap()
        // Which way, on which computer, and what will happen.
        let start = app.buttons["handover.start"]
        XCTAssertTrue(start.waitForExistence(timeout: 10))
        XCTAssertTrue(app.buttons["handover.machine"].firstMatch.label.contains("Studio Mac"))
        XCTAssertTrue(app.buttons["handover.machine"].firstMatch.label.contains("Where it is now"))
        XCTAssertTrue(start.isEnabled, "its own computer and folder are there to start with")
        shot("handover-sheet")
        for _ in 0..<3 where !app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "The earlier session stops")).firstMatch.exists { app.swipeUp() }
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "The earlier session stops")).firstMatch.exists, "what will happen is said first")
        start.tap()
        // The new session read the handover note and said so.
        let done = app.staticTexts["handover.done"]
        XCTAssertTrue(done.waitForExistence(timeout: 20))
        XCTAssertTrue(done.label.contains("Studio Mac · Codex answers for 后端工程师 now"), done.label)
        XCTAssertTrue(app.staticTexts["handover.reply"].exists)
        shot("handover-done")
    }

    @MainActor
    func testAnOfflineAgentsCardSaysWhy() {
        // The writer's runner said goodbye as it was stopped.
        let app = launch(["-myco-route", "member:laptop-claude-a1e2"])
        let why = app.staticTexts["member.offline.why"]
        XCTAssertTrue(why.waitForExistence(timeout: 15))
        XCTAssertTrue(why.label.hasPrefix("Its runner was stopped"), why.label)
        shot("member-offline")
    }

    @MainActor
    func testAComputerIsAddedWithOneCommand() {
        let app = launch()
        XCTAssertTrue(app.buttons["chat.project:myco-ios"].waitForExistence(timeout: 15))
        app.buttons["chrome.add"].firstMatch.tap()
        app.buttons["Add a computer"].firstMatch.tap()
        // One line to run on that computer, in the folder agents should use.
        let command = app.staticTexts["computer.command"]
        XCTAssertTrue(command.waitForExistence(timeout: 5))
        expectation(for: NSPredicate(format: "label BEGINSWITH %@", "curl -fsSL https://demo.myco.example/j/"), evaluatedWith: command)
        waitForExpectations(timeout: 5)
        XCTAssertTrue(app.staticTexts["computer.waiting"].exists, "it waits for the folder to show up")
        shot("computer-command")
        // The other system's line.
        app.buttons["Windows (PowerShell)"].tap()
        XCTAssertTrue(command.label.hasPrefix("irm 'https://demo.myco.example/j/"), command.label)
        XCTAssertTrue(command.label.hasSuffix("?format=ps1' | iex"), command.label)
        // (The demo runs the command a moment later.) It shows up, and is
        // put in the project chosen.
        let arrived = app.staticTexts["computer.arrived"]
        XCTAssertTrue(arrived.waitForExistence(timeout: 20), "the folder showed up")
        XCTAssertTrue(arrived.label.contains("myco 手机端"), arrived.label)
        shot("computer-arrived")
    }

    @MainActor
    func testAWorkflowDecisionIsMadeAndAWorkflowRun() {
        // The workbench shows what waits for me as soon as it opens.
        let app = launch(["-myco-route", "tab:workbench"])
        let decision = app.descendants(matching: .any)["decision"]
        XCTAssertTrue(decision.waitForExistence(timeout: 15))
        XCTAssertTrue(app.buttons["合并"].exists && app.buttons["不合并"].exists, "its choices")
        XCTAssertTrue(app.buttons.matching(identifier: "workbench.workflow").firstMatch.exists, "the workflows to run")
        XCTAssertTrue(app.buttons.matching(identifier: "workbench.app").firstMatch.exists, "the apps to make a page from")
        XCTAssertTrue(app.switches.matching(identifier: "workbench.automation.switch").firstMatch.exists, "the automations")
        shot("workbench")
        // A choice, with a word.
        let note = app.textFields["decision.note"]
        note.tap()
        note.typeText("OK")
        app.buttons["合并"].tap()
        expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: decision)
        waitForExpectations(timeout: 10)
        // All of them, on their own page.
        app.buttons["workbench.workflows"].tap()
        XCTAssertTrue(app.buttons.matching(identifier: "workflow.row").firstMatch.waitForExistence(timeout: 10))
        shot("workflows")
        // A workflow: its steps; run it with what it asks for.
        app.buttons.matching(identifier: "workflow.row").firstMatch.tap()
        let start = app.buttons["workflow.start"]
        XCTAssertTrue(start.waitForExistence(timeout: 10))
        expectation(for: NSPredicate(format: "isEnabled == true"), evaluatedWith: start)
        waitForExpectations(timeout: 10)
        shot("workflow")
        start.tap()
        let task = app.textViews["workflow.input.task"].exists ? app.textViews["workflow.input.task"] : app.textFields["workflow.input.task"]
        XCTAssertTrue(task.waitForExistence(timeout: 5))
        XCTAssertFalse(app.buttons["workflow.run.go"].isEnabled, "what it needs is not filled in yet")
        task.tap()
        task.typeText("Add a share button")
        app.buttons["workflow.run.go"].tap()
        // The run: going, step by step.
        let status = app.descendants(matching: .any)["run.status"]
        XCTAssertTrue(status.waitForExistence(timeout: 10))
        XCTAssertTrue(status.label.contains("Running"), status.label)
        XCTAssertTrue(app.descendants(matching: .any).matching(identifier: "run.step").firstMatch.exists)
        shot("workflow-run")
        app.buttons["run.cancel"].tap()
        app.buttons["Cancel the run"].tap()
        expectation(for: NSPredicate(format: "label CONTAINS %@", "Cancelled"), evaluatedWith: status)
        waitForExpectations(timeout: 10)
    }

    @MainActor
    func testTheWorkbenchSaysWhatWaitsAndWhereTokensGo() {
        // 运维 asks; a task of 后端工程师's failed: both wait on the workbench.
        let app = launch(["-myco-route", "tab:workbench"])
        let notices = app.buttons.matching(identifier: "workbench.notice")
        XCTAssertTrue(notices.firstMatch.waitForExistence(timeout: 15))
        XCTAssertEqual(notices.count, 2)
        XCTAssertTrue(notices.firstMatch.label.contains("is asking you"), notices.firstMatch.label)
        // Opened: the failure is read, and no longer waits.
        notices.element(boundBy: 1).tap()
        XCTAssertTrue(app.descendants(matching: .any)["composer.input"].waitForExistence(timeout: 10))
        back(app)
        expectation(for: NSPredicate(format: "count == 1"), evaluatedWith: notices)
        waitForExpectations(timeout: 10)
        // Where the tokens go, further down.
        let total = app.staticTexts["workbench.usage.total"]
        for _ in 0..<5 where !total.isHittable { app.swipeUp() }
        XCTAssertTrue(total.waitForExistence(timeout: 10))
        let seven = total.label
        XCTAssertTrue(app.descendants(matching: .any).matching(identifier: "workbench.usage.member").firstMatch.exists)
        shot("workbench-usage")
        app.segmentedControls["workbench.usage.days"].buttons["30 d"].tap()
        expectation(for: NSPredicate(format: "label != %@", seven), evaluatedWith: total)
        waitForExpectations(timeout: 10)
    }

    @MainActor
    func testASuperagentIsNamedAndItsRequestApproved() {
        let app = launch(["-myco-route", "sheet:workspace"])
        let setup = app.buttons["workspace.superagent.setup"]
        XCTAssertTrue(setup.waitForExistence(timeout: 15))
        setup.tap()
        // One of my agents.
        let how = app.segmentedControls["superagent.how"]
        XCTAssertTrue(how.waitForExistence(timeout: 5))
        shot("superagent-setup")
        how.buttons["Use one of your agents"].tap()
        let pick = app.buttons.matching(identifier: "superagent.pick").firstMatch
        XCTAssertTrue(pick.waitForExistence(timeout: 5))
        pick.tap()
        app.buttons["superagent.go"].tap()
        // Its conversation: what it asks for waits above the input.
        let approval = app.descendants(matching: .any)["approval"]
        XCTAssertTrue(approval.waitForExistence(timeout: 20), "its request")
        XCTAssertTrue(approval.label.contains("旧官网") || app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "旧官网")).firstMatch.exists)
        shot("superagent-approval")
        app.buttons["approval.approve"].tap()
        expectation(for: NSPredicate(format: "exists == false"), evaluatedWith: approval)
        waitForExpectations(timeout: 10)
    }

    @MainActor
    func testTheSuperagentIsInTheTopBarNotAChat() {
        // Set up from the sparkle at the top of the page.
        let app = launch(["-myco-route", "tab:chats"])
        let orb = app.buttons["chrome.superagent"]
        XCTAssertTrue(orb.waitForExistence(timeout: 20))
        orb.tap()
        let how = app.segmentedControls["superagent.how"]
        XCTAssertTrue(how.waitForExistence(timeout: 5))
        how.buttons["Use one of your agents"].tap()
        let pick = app.buttons.matching(identifier: "superagent.pick").firstMatch
        XCTAssertTrue(pick.waitForExistence(timeout: 5))
        let name = pick.label
        pick.tap()
        app.buttons["superagent.go"].tap()
        // Its conversation comes up over the page.
        XCTAssertTrue(app.descendants(matching: .any)["approval"].waitForExistence(timeout: 20))
        app.buttons["superagent.close"].tap()
        // Not one of the chats, nor of the members.
        XCTAssertTrue(orb.waitForExistence(timeout: 10))
        shot("superagent-button")
        XCTAssertFalse(app.staticTexts[name].exists && app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'chat.' AND label CONTAINS %@", name)).firstMatch.exists,
                       "its chat is not in the list")
        app.buttons["chrome.members"].tap()
        XCTAssertTrue(app.navigationBars["Members"].waitForExistence(timeout: 5))
        XCTAssertFalse(app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'member.' AND label CONTAINS %@", name)).firstMatch.exists, "nor a member")
        // A tap on the orb: its conversation again.
        back(app)
        orb.tap()
        XCTAssertTrue(app.descendants(matching: .any)["composer.input"].waitForExistence(timeout: 10))
    }

    @MainActor
    func testAnAppMakesAPageThatTalksToTheWorkspace() {
        // From the workbench: all the apps.
        let app = launch(["-myco-route", "tab:workbench"])
        let entry = app.buttons["workbench.apps"]
        XCTAssertTrue(entry.waitForExistence(timeout: 15))
        entry.tap()
        let checklist = app.buttons["app.myco/checklist"]
        XCTAssertTrue(checklist.waitForExistence(timeout: 10))
        XCTAssertTrue(app.buttons["app.myco/whiteboard"].exists)
        XCTAssertTrue(app.staticTexts["apps.own.empty"].exists, "how to get one of the workspace's own is said")
        shot("apps")
        // A shelf shows only its own.
        app.buttons["apps.shelf.creative"].tap()
        XCTAssertTrue(app.buttons["app.myco/whiteboard"].waitForExistence(timeout: 5))
        XCTAssertFalse(checklist.exists)
        app.buttons["apps.shelf.all"].tap()
        // Making one: its name, where, sample data.
        XCTAssertTrue(checklist.waitForExistence(timeout: 5))
        checklist.tap()
        let name = app.textFields["app.use.name"]
        XCTAssertTrue(name.waitForExistence(timeout: 5))
        name.tap()
        name.typeText("Groceries")
        shot("app-use")
        app.buttons["app.use.make"].tap()
        // It opens, and the page reads its records through the app.
        XCTAssertTrue(app.descendants(matching: .any)["page.title"].waitForExistence(timeout: 10))
        let web = app.webViews.firstMatch
        XCTAssertTrue(web.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "买牛奶")).firstMatch.waitForExistence(timeout: 15), "the sample records came")
        XCTAssertTrue(web.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "我是")).firstMatch.exists, "it knows who I am")
        // What it writes comes back.
        let field = web.textFields.firstMatch
        field.tap()
        field.typeText("Bread")
        web.buttons["加"].tap()
        XCTAssertTrue(web.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "Bread")).firstMatch.waitForExistence(timeout: 10))
        shot("app-page")
        app.buttons["page.close"].tap()
        // It is on top of the list, to open again.
        let made = app.buttons.matching(identifier: "apps.made").firstMatch
        XCTAssertTrue(made.waitForExistence(timeout: 10))
        XCTAssertTrue(made.label.contains("Groceries"), made.label)
    }

    @MainActor
    func testAutomationsAreListedSwitchedAndWritten() {
        // On the workbench, each with its switch; all of them one tap on.
        var app = launch(["-myco-route", "tab:workbench"])
        let entry = app.buttons["workbench.automations"]
        XCTAssertTrue(entry.waitForExistence(timeout: 15))
        let switches = app.switches.matching(identifier: "workbench.automation.switch")
        XCTAssertTrue(switches.firstMatch.waitForExistence(timeout: 10))
        XCTAssertEqual(switches.count, 2)
        entry.tap()
        let rows = app.buttons.matching(identifier: "automation.row")
        XCTAssertTrue(rows.firstMatch.waitForExistence(timeout: 10))
        XCTAssertEqual(rows.count, 2)
        // Each says when, to whom, when next and how the last one went.
        let first = rows.firstMatch.label
        XCTAssertTrue(first.contains("每天早上汇总"), first)
        XCTAssertTrue(first.contains("Weekdays at 09:00"), first)
        XCTAssertTrue(first.contains("协调者"), first)
        XCTAssertTrue(first.contains("Next "), first)
        XCTAssertTrue(first.contains("Sent"), first)
        XCTAssertTrue(rows.element(boundBy: 1).label.contains("Paused"), rows.element(boundBy: 1).label)
        shot("automations")
        // A switch turns one off.
        let toggle = app.switches.matching(identifier: "automation.switch").firstMatch
        XCTAssertEqual(toggle.value as? String, "1")
        toggle.switches.firstMatch.exists ? toggle.switches.firstMatch.tap() : toggle.tap()
        expectation(for: NSPredicate(format: "value == %@", "0"), evaluatedWith: toggle)
        waitForExpectations(timeout: 5)
        // A new one: what is missing is said until it can be saved.
        app.buttons["automations.new"].tap()
        let name = app.textFields["automation.name"]
        XCTAssertTrue(name.waitForExistence(timeout: 5))
        XCTAssertEqual(app.staticTexts["automation.missing"].label, "It needs a name")
        XCTAssertFalse(app.buttons["automation.save"].isEnabled)
        name.tap()
        name.typeText("Nightly check")
        let body = app.descendants(matching: .any).matching(identifier: "automation.body").firstMatch
        body.tap()
        body.typeText("Check the services.")
        XCTAssertTrue(app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", "Weekdays at 09:00")).firstMatch.exists, "the schedule is said in words")
        shot("automation-editor")
        XCTAssertFalse(app.staticTexts["automation.missing"].exists)
        app.buttons["automation.save"].tap()
        expectation(for: NSPredicate(format: "count == 3"), evaluatedWith: rows)
        waitForExpectations(timeout: 10)
        XCTAssertTrue(rows.firstMatch.label.contains("Nightly check"))
        app.terminate()

        // None yet: what they are for, and a few to start from.
        app = launch(["-myco-start", "waiting", "-myco-route", "page:automations"])
        XCTAssertTrue(app.staticTexts["Let agents work on their own schedule"].waitForExistence(timeout: 15))
        XCTAssertEqual(app.buttons.matching(identifier: "automations.template").count, 4)
        shot("automations-empty")
        // One with a webhook ends on its address and secret, shown once.
        app.buttons.matching(identifier: "automations.template").element(boundBy: 3).tap()
        XCTAssertTrue(app.buttons["automation.save"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["automation.save"].isEnabled, "a template is ready to save")
        app.buttons["automation.save"].tap()
        XCTAssertTrue(app.buttons["automation.secret.done"].waitForExistence(timeout: 10))
        XCTAssertTrue(app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", "https://demo.myco.example/hooks/")).firstMatch.exists)
        shot("automation-secret")
        app.buttons["automation.secret.done"].tap()
        XCTAssertTrue(rows.firstMatch.waitForExistence(timeout: 10), "the new one is in the list")
    }

    @MainActor
    func testARequestThatFailsSaysWhyAndCanBeTriedAgain() {
        // The first join link fails as a gateway in front of the workspace
        // would: said in words, the raw reason one tap away, and a button
        // to try again.
        let app = launch(["-myco-trouble", "links"])
        XCTAssertTrue(app.buttons["chat.project:myco-ios"].waitForExistence(timeout: 15))
        // (the plus offers an agent, or a computer)
        app.buttons["chrome.add"].firstMatch.tap()
        app.buttons["Add an agent"].firstMatch.tap()
        let words = app.staticTexts["failure.words"]
        XCTAssertTrue(words.waitForExistence(timeout: 5))
        XCTAssertEqual(words.label, "Cannot reach the workspace right now, or it is answering too slowly.")
        XCTAssertFalse(app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "Bad Gateway")).firstMatch.exists, "the raw reason is not what is said")
        XCTAssertFalse(app.buttons["Copy"].isEnabled, "there is no link to copy yet")
        app.buttons["failure.details"].tap()
        let raw = app.staticTexts["failure.detail"]
        XCTAssertTrue(raw.waitForExistence(timeout: 3), "but it is one tap away")
        XCTAssertTrue(raw.label.contains("HTTP 502"), raw.label)
        XCTAssertTrue(raw.label.contains("502 Bad Gateway nginx"), raw.label)
        XCTAssertTrue(app.buttons["failure.copy"].exists, "to copy and pass on")
        shot("failure-details")
        app.buttons["failure.retry"].tap()
        let sentence = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "curl -fsSL https://demo.myco.example/j/")).firstMatch
        XCTAssertTrue(sentence.waitForExistence(timeout: 5), "trying again got the link")
        XCTAssertFalse(words.exists)
        XCTAssertTrue(app.buttons["Copy"].isEnabled)
    }
}
