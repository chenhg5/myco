# Workspace page links on Android

In a conversation, tap a page link from the current workspace to open it inside
myco. Both Markdown links and pasted web addresses are supported, including links
in your own messages. Links to another workspace, ordinary websites and invalid
page addresses keep the usual external-link behavior.

A page address may include a fragment after `#`, up to 1024 characters. myco
passes that location to the page without interpreting its contents. For example,
an app can use a fragment to select an item or a section. Query parameters before
`#` are not part of this internal page-link format.

Changing the location of an already open page keeps the same document and its
state. Opening the same location again notifies the page with `hashchange`, so it
can restore its position. Closing the page returns to the conversation.

The fragment is not sent in workspace API requests or page file paths. The page
uses your existing workspace permissions; no credential is added to a copied
link. Templates can continue to use `myco.ready`'s optional `page.url`, as supplied
by the workspace, to construct shareable addresses.
