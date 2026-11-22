# Reader

A personal, local PDF/TXT bookshelf. Import files using the page's file picker
or drag and drop. No book bytes, passwords, reading progress, preferences or
bookmarks are published as page records. Books are kept in this browser's
IndexedDB, partitioned by page and member. Another device must import its own
copy. Clearing the App/browser's website data removes this library: retain the
original files or use Manage book > Save original file (up to 20 MiB; larger
originals must be kept separately).

There are no readable book collections or registered agent actions. Do not claim
to have read a locally imported book, and do not put binary files into page data.
For help with a passage, ask the reader to share the passage or attach the file
through the conversation explicitly. The optional sample shelf contains three
original short essays, not commercially published ebooks.

PDF retains its fixed layout (zoom, outline, selectable text and text search).
Font family, size, weight, line spacing and text width apply to TXT. TXT supports
UTF-8, UTF-16 with BOM and GB18030 fallback. Scanned PDFs display normally but
require OCR outside this app for text selection/search. PDF themes change the
surrounding paper, not colors embedded in the PDF itself.

Limits: PDF 100 MiB / 10,000 pages; TXT 5 MiB. All PDF worker, CMaps and standard
fonts, image decoders and ICC profiles are bundled locally. Reduced-motion preferences disable page animations.

Reading settings offer horizontal pages, vertical pages and continuous scroll.
Mode changes retain source anchors. Continuous PDFs restore the page and its
intra-page offset and render only nearby pages. In paged vertical mode, Up/Down,
wheel and up/down swipes turn pages; continuous mode uses natural scrolling.
All settings and progress remain local, with backward-compatible defaults for
libraries created before reading modes were introduced.
