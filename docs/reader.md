# Reader

Application source paths and Node test commands below refer to the separate [myco-ai/apps](https://github.com/myco-ai/apps) checkout. Host bridge implementation remains in myco-ai/myco.


Reader is a personal bookshelf for PDF and TXT files. Add the official Reader
app, then choose **Import books**, or drop files onto the shelf. The optional
sample shelf contains three original short essays, so you can try reading
without importing a book first.

## Your library

- Import PDF files up to 100 MiB and TXT files up to 5 MiB. Importing the same
  bytes twice keeps one entry and preserves its progress and bookmarks.
- Search by title, sort by last read, date added or title, and choose covers or
  a compact list. Filters show all books, those in progress or finished books.
- A book's menu lets you rename it, save a copy of the original file, or remove
  it from this local library. Saving originals uses the host's download bridge
  and is offered for files up to 20 MiB; retain larger source files yourself.
  Removing a book also removes its local progress and
  bookmarks, with confirmation. It does not delete the original source file.

Books, progress, bookmarks and preferences are stored in this App/browser's
IndexedDB, separately for each page and member. **They are not uploaded to the
workspace and do not sync between devices.** Reopening the page on this device
restores them. Clearing website/App data, using a different browser profile or
losing this device can remove the library: keep the original files. Storage
errors are reported; an uncommitted import is never shown as a saved book.

## Reading

**Reading settings > Reading mode** offers three modes:

- **Horizontal pages:** swipe left/right or use the side arrows.
- **Vertical pages:** swipe up for the next page, down for the previous page;
  use the top/bottom arrows, mouse wheel, Up/Down or Page Up/Page Down.
- **Continuous scroll:** scroll naturally with touch, trackpad or mouse wheel.
  Up/Down scroll a little; Page Up/Page Down and Space move a screenful. There
  are no page-turn animations or gesture interception in this mode.

Switching modes retains the current source position. A continuous PDF remembers
both the page and the position within that page, including when reopened.
Vertical paged PDFs fit the whole page by default; zooming in allows panning
inside it. Continuous PDFs paint only nearby pages, including in long books
with different page sizes.

Move further with the progress bar, chapter list, bookmarks or search results. Focus mode hides the toolbar; Escape or **Show
toolbar** brings it back. Fullscreen is offered where the host allows it.

**Reading settings** has App theme, Paper, Warm, Sage and Night backgrounds;
Slide, Leaf, Fade and instant transitions. The Leaf transition is a brief
perspective animation, not a physical paper-curl simulation. System Reduce
Motion disables all transitions.

TXT books are laid out using browser typography and real column pagination.
Choose serif, sans serif or monospaced type, 14–36 px size, 1.4–2.6 line spacing,
bold, and text width. The reader keeps a source character anchor when the text
reflows. UTF-8, UTF-16 with a byte-order mark, and GB18030 fallback are supported.
Chinese numbered headings and English chapter/part/book headings populate the
contents automatically. Search is literal, case-insensitive, and limited to
100 displayed matches.

PDF keeps its original page layout. Use the toolbar to zoom or reset to fit
width; scroll within a tall/zoomed page. Font and line-spacing controls apply
only to TXT. PDF outline entries provide its contents; text can be selected
and searched. Scanned books display normally but need external OCR for search
and selection. Themes change the PDF's surroundings, not its embedded colors.
Encrypted PDFs ask for a password each time they open; passwords are not saved.
PDFs over 10,000 pages are rejected. The worker, CMaps, standard fonts, image
decoders (including JPEG2000/JBIG2) and ICC profiles ship
with the app and do not require an external font service.

## Agent access

Reader currently has no shared book collections or agent actions. An agent
cannot read locally imported books through page data. To discuss a passage,
share it or attach the book explicitly through your conversation. This local
file boundary is separate from the framework's shared-page collaboration and
page export capabilities.

## Development checks

```sh
cd templates && node build.mjs reader
cd ..
node templates/reader.test.mjs
node templates/reader.ui-test.mjs
node templates/localization.test.mjs
```

The browser test uses the production bundle and the desktop host's real CSP.
It covers import validation, duplicate detection, transaction aborts, storage
isolation, reflow and restoring progress, settings, contents/search/bookmarks,
PDF canvas/text/search, JPEG2000 scan rendering, passwords, downloads, removal,
mobile touch, vertical paging, continuous scroll, source-position restoration,
virtual PDF rendering and reduced motion. It requires Playwright Chromium;
`READER_ARTIFACTS` selects the screenshot directory. Native file pickers and WKWebView should also be checked on devices;
browser tests alone are not native-device acceptance.
