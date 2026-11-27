# Local App preferences

Workspace names and pictures edited in **Settings → Workspaces → Name and face**
are personal to this App profile. Saving waits for the local backend to commit
both together. A failed save leaves the dialog open and keeps the previous
settings. These choices survive App upgrades and WebView origin changes; they do
not rename the shared workspace or sync to another computer.

The App stores preferences beside its workspace-list file, in a sibling directory
named `<profile>-preferences`. For the default `console-hubs.json` profile this is
`console-hubs-preferences` inside the system's myco configuration directory
(`~/Library/Application Support/myco` on macOS). Files are private to the local OS
user. `preferences.json` contains metadata and local choices. Picture bytes live
in separate, content-addressed files under `images/`, not in the JSON document.
A metadata replacement is atomic; competing App processes retry a busy profile.

At startup the App reads the file before loading the interface. On first use it
imports an explicit list of existing browser preferences, extracts workspace
pictures into files, and copies referenced chat backgrounds out of IndexedDB.
Existing file values (including deliberate deletions) win over stale browser
copies. Browser storage is a best-effort cache; a full cache cannot discard an
avatar saved by the backend. An unreadable profile or failed migration is shown
with a retry action and is never replaced with empty defaults. Browser-only
consoles and older App backends retain their existing storage behavior.

Migration can copy only data still accessible to the current WebView origin. It
cannot recover preferences or pictures already removed by older versions, OS
cleanup, or an inaccessible former origin. Existing browser data is left intact.

## Storage audit

| User data or choice | Storage after this change | Reason |
| --- | --- | --- |
| Workspace aliases, colours and pictures | Local profile; pictures in files | Explicit personal customization must survive upgrades |
| Theme, language, text size, interface style and simple mode | Local profile | Explicit accessibility and appearance choices |
| Hidden sidebar sections, custom order and pinned conversations | Local profile | Recreating a curated navigation layout is frustrating |
| Focus and conversation backgrounds | Local profile; pictures in files | Preserve chosen appearance without browser quota dependence |
| Usage range and project selection, report-only runs, auto-translation and island setting | Local profile | Preserve recurring viewing preferences |
| Drafts, unsent queues and sent-input history | Local profile | Losing composed text is more costly than losing a tab selection |
| Old-Hub hidden-message fallback, dismissed onboarding/plan cards, completed legacy-file recovery markers | Local profile | Avoid reappearing content and duplicate recovery prompts |
| Sidebar width/open state, folds, last workspace/tab, recent chats, help position and temporary list filters | Browser storage | Small, transient navigation state may reset |
| Page SDK `myco.storage` and isolated page storage | Existing page-scoped store | Application-owned data is outside the preference contract; it is not silently copied into a global settings file |
| Credentials, provider configuration and conversation cache | Existing dedicated services | Never migrate arbitrary browser keys into preferences |

Preference read/write/import and failure cases are covered by Go tests; client
migration, stale-cache precedence, quota failure and retry behavior are covered
by `npm --prefix web run test:preferences`. The UI smoke includes saving a picture,
showing a failed save, retrying, clearing browser storage and relaunching the App.
