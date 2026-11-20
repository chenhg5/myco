# 空间清理: how to use it

A page that shows what takes the room on the disks of the computer it is
opened on (in the myco App: it reads the disk through `myco.os`), and moves
what the person picks to the trash. Only the person in front of the
computer scans and cleans: nothing about the files is stored in the page.

One collection, `cleanups`: what was cleaned, for everyone to see. A record:
`at` (when), `computer` (its name), `disk`, `freed` (bytes), `count` (how
many items), `by` (who).

    myco page data list --page <pg_id> --collection cleanups

To help someone make room, tell them what to look at (Xcode's DerivedData,
old downloads, caches); they open the page, scan, and choose. You cannot
scan or delete from here.
