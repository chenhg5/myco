# Disk usage and capacity units

Space cleaner defaults to binary units (KiB/MiB/GiB, base 1024) on Windows and decimal units (KB/MB/GB, base 1000) on other platforms. The capacity selector overrides this preference locally and applies to disk cards, scan progress, the map, file lists and the cleanup basket. For example, 500,000,000,000 bytes is 500 GB or about 465.7 GiB; changing units never changes the underlying byte count. Some Windows system interfaces label binary values as GB.

Disk cards retain the current user's capacity boundary: used is total minus available-to-user bytes. Quotas and reserved space can make available-to-user differ from free bytes. Directory scans sum readable file storage and are not a reconciliation of all filesystem usage: metadata, snapshots, inaccessible content and hard-link accounting can differ from system totals.

Windows builds containing the allocation fix query file allocation information for ordinary files and actual compressed/sparse storage for NTFS compressed/sparse files. A template update alone does not update native Windows scanning; an older desktop App still uses logical file lengths. Where the filesystem rejects allocation queries, the scanner falls back to logical file size. Zero allocated bytes is a valid value and is preserved in all file lists and the cleanup basket. Symlinks are not followed to measure their targets.

Moving files to Trash does not guarantee that their storage is immediately free. Emptying Trash, snapshots, shared blocks and filesystem reclamation affect when space becomes available.
