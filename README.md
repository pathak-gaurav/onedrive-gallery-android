# OneDrive Gallery for Android

Install the signed ARM64 APK, tap **Sign in to OneDrive**, and finish Microsoft's browser consent screen. The connector identifies itself as **Rclone**. No Microsoft app registration, client ID, PC connection, or Rclone installation is required on your phone.

For **Phone library**, tap **Allow gallery access** and choose all photos and videos. Original photo metadata access is used to read unchanged original bytes for checksums; GPS information is not analyzed. With limited access the app clearly reports a partial scan.

Version 1.1 installs over 1.0 with the same package and signing key, preserving Microsoft sign-in. It replaces recursive full-drive enumeration with Microsoft's paginated delta feed, persists checkpoints, and uses subsequent delta links to read changes. The old version did not save its in-progress scan, so the first 1.1 run establishes a new index.

## Included

- All duplicates and separate folder scan tabs, file size/type/search filters, side-by-side thumbnails, exact paths, dimensions/dates, and OneDrive browser links.
- Per-copy checkboxes, bulk selection, keep-oldest/newest/shortest-path helpers, deletion review, and fresh ETag/hash/size checks that keep one copy per group.
- Deletion moves selected cloud files to the OneDrive Recycle Bin, closes its review window when deletion finishes, and automatically rescans that same scope. Failed refreshes leave results marked obsolete.
- Phone library compares local MediaStore images/videos concurrently with incoming OneDrive metadata by size and QuickXorHash. Positive matches appear immediately; unmatched files stay **Checking** until the cloud feed completes. Missing cloud hashes or unreadable originals produce **Needs review**. Filenames do not need to match.
- Private size-indexed lookups replace a full cloud-list search for each phone original. Local hashes are cached against URI, size, modified time, MediaStore generation and database version. Live updates are batched, and visible cards retain their thumbnails rather than reloading images on every progress update.
- Select missing originals to upload. Albums are preserved under **Pixel 10 Pro uploads** by default. The app refreshes OneDrive, rehashes selected originals, skips existing matches, streams 5 MiB chunks, refuses overwrites, and refreshes the comparison afterward.
- Browse OneDrive to choose an upload folder, optionally preserve albums, bulk-select filtered missing files, filter by dates/type/extension/size, and sort by taken date, size or filename. Taken dates fall back to modified dates when unavailable.
- **Already uploaded** shows matched originals. Set **Selection action → Remove matched phone copies**, select up to 100 originals, and review. A running comparison pauses at a saved checkpoint; each selected local original is freshly hashed and an existing cloud match is fetched and verified before Android's own deletion confirmation. The gallery refreshes afterward. OneDrive copies are never deleted by this action.
- Photo previews and video playback use a separate native activity without a JavaScript bridge. Cloud images use thumbnails; cloud videos stream from a Microsoft preauthenticated URL. PDF/Office documents open their OneDrive browser preview.
- Background notification/progress and pause/continue. No cloud-original downloads for comparison or full-video buffering. New comparisons resume saved cloud progress and reuse valid local hashes.

## Sources and limits

All local media is included, even if Google Photos has also backed it up. Cloud-only Google Photos and Drive items are excluded. Android does not provide reliable Google backup status for a local file, so each phone result is labeled **On this phone** without guessing whether it is Google-backed. Shared OneDrive shortcuts and Personal Vault are excluded. A scan is a fresh observation of a changing cloud account, not an atomic snapshot.

This is exact original-file comparison, not perceptual similarity. An edited, resized, transcoded, or re-encoded image/video is a different file. Metadata size and resolution help review quality but are not a quality score. Checksums can theoretically collide.

If an upload/deletion response is lost, refresh before retrying. The app does not blindly retry cloud mutations. Files rejected by OneDrive naming rules need to be renamed before upload. Cloud enumeration resumes from saved metadata checkpoints, but interrupted uploads are not resumed; compare again to see what completed. Microsoft can expire a delta token, requiring a fresh enumeration.

## Build

Requires JDK 17, Gradle 8.14.3, Android SDK 36/build-tools 36, Android NDK 29.0.14206865, Go 1.26.x. Microsoft credentials and the PC's Rclone config must never be copied into this project.

The Android library links Rclone v1.75.1's OneDrive backend and OAuth configuration state machine. `oauthutil.OpenURL` supplies a local OAuth URL to the external Android browser. The app automatically selects `/me/drive` and confirms it. Tokens are saved by Rclone in the app-private config; backups and device transfer are excluded.

On Windows use the provided `build.ps1`; it uses the installed SDK and build tools in `%LOCALAPPDATA%\OneDrive Gallery Build`. Private signing material is stored outside the source tree. The release supports ARM64 Android 14+, including the Pixel 10 Pro. The native ELF and APK alignment are checked for 16 KB page compatibility.

`go test ./...` from `engine` covers live matches before the last page, checkpoints, delta refresh, folder renames/deletions, expired tokens, checksum uncertainty, fresh backup verification for phone removal, cloud-deletion safeguards, streaming uploads, URLs and Rclone OAuth startup. `node --test test_model.mjs test_gallery.mjs` covers live batches, selection, filters and sorting. The browser fixture `qa_ui.py` uses fictional data and cannot modify OneDrive or phone files. Native Android playback and deletion prompts still require a phone test; no physical Pixel is connected here.
