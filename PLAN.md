# Astrowall improvement plan

Work on **one milestone at a time**. After each milestone, verify it, summarize what changed, and get feedback before starting the next one. Refactor only where it supports the current milestone.

## Product and preferences

Astrowall fetches NASA's Astronomy Picture of the Day and sets it as a desktop wallpaper on Linux, macOS, and Windows. It supports today's, dated, and random APODs; custom output paths; an incremental SQLite library and preview/full-image caches; a keyboard-driven TUI; search in Recent and Favorites; image and description views; terminal graphics (WezTerm, Kitty/Ghostty, ANSI fallback); browser links; persisted favorites and CLI favorite cycling; API-key precedence and OS credential storage; sync-only mode; and video-aware behavior.

The desired direction is a **rich astronomy dashboard** with polished, usable visuals. The primary terminal is **WezTerm directly, without tmux**. Keep the preview prominent and preserve native-image positioning and cleanup as the interface evolves.

## Milestones

### 1. Visual foundation — complete

- [x] Establish a cohesive color palette and consistent typography for pane titles, selection, metadata, statuses, and help.
- [x] Polish existing pane borders and contextual keyboard hints without changing layout geometry or key behavior.
- [x] Verify representative TUI sizes and existing WezTerm placement tests; run project checks.
- [x] Collect feedback on the look before changing the layout.

**Done when:** the existing two-column browser looks more intentional, remains legible at supported sizes, and native previews remain aligned. This is a styling pass, not a new dashboard layout.

### 2. Dashboard header and status — complete

- [x] Add a compact, width-aware header with total stored-library and favorite counts.
- [x] Show sync progress and action feedback on separate lines so background updates do not hide user actions.
- [x] Adjust WezTerm placement for the header and verify multi-size layout, counts during sync, and preview lifecycle.
- [x] Collect feedback on the dashboard before starting responsive layout work.

### 3. Responsive layout and empty states — complete

- [x] Keep two sidebar panes on wide screens; show the active list on compact screens, stack it above details on narrow screens, and use a focused list on very small screens.
- [x] Explain empty library, empty favorites, no search matches, and unavailable previews; allow Tab to open an empty Favorites pane.
- [x] Verify filtering, pane switching, native image cleanup and placement, and bounds at representative sizes.
- [x] Collect feedback on the responsive layout in WezTerm.

### 4. Image presentation and responsiveness — complete

- [x] Preserve proportions and center portrait/panoramic images in ANSI and native Kitty/WezTerm panes.
- [x] Prepare ANSI previews asynchronously with a bounded six-entry cache; discard canceled or stale results after navigation, mode changes, and resize.
- [x] Fall back to ANSI if native image preparation fails without retrying the same failed placement on every frame.
- [x] Verify preview geometry, lifecycle, rapid navigation, and resize with tests and project checks.
- [x] Collect visual feedback in direct WezTerm.

### 5. Full-library browsing — complete

- [x] Load the complete SQLite archive on demand with `b`; keep Recent limited to 30 for startup.
- [x] Preserve independent Recent, Archive, and Favorites searches and list selections while switching; keep new sync items and favorite changes visible in the archive.
- [x] Verify older non-favorites, filtering, toggling favorites, background sync, and native preview request for archived items.
- [x] Collect feedback on Archive browsing in WezTerm.

### 6. Manual refresh and preview retry — complete

- [x] Add `s` to refresh missing APODs and retry previously failed previews using the existing sync planner.
- [x] Add `p` to explicitly re-download the selected preview, including cached images and older Archive items, without syncing unrelated dates.
- [x] Keep progress and errors visible; queue one follow-up refresh if another library operation is in progress.
- [x] Verify successful and failed retries, no-op refreshes, selection and native preview updates.
- [x] Collect feedback on refresh and retry in WezTerm.

### 7. Shared cache-first wallpaper operations — implemented, awaiting feedback

- [x] Reuse stored metadata for today/dated CLI requests and share the full-image cache across CLI, TUI, and favorite cycling.
- [x] Use stored image URLs before requesting metadata again; download missing/empty cache files again.
- [x] Make full-image downloads and CLI output copies atomic, preserving existing files on interrupted transfers.
- [x] Extract shared image-cache operations from `tui.go` into `image_cache.go`.
- [ ] Collect feedback before starting the next milestone.

**Done when:** cached wallpapers work without a new metadata/image request, unsuccessful transfers leave no partial cache files, and all wallpaper modes retain their existing controls.

## Later ideas

- Skip videos when cycling favorites.
- Wallpaper history and scheduled rotation.
- Extract shared wallpaper/cache operations and simplify repeated database row scanning where touched by a milestone.

## Verification notes

For milestone 1, the claim is visual consistency without breaking sizing, navigation, or native image placement. Existing `tui_test.go` geometry and WezTerm lifecycle tests cover layout behavior; `go test ./...` and `go vet ./...` cover project integration. A real WezTerm visual check is still needed to judge the aesthetics and native-image appearance; automated tests cannot establish those perceptual details.

Milestone 1 checks: `go test ./...`, `go test -race ./...`, `go vet ./...`, and `git diff --check` passed. User reviewed the look in WezTerm and approved it.

Milestone 2 checks: `go test ./...`, `go test -race ./...`, `go vet ./...`, and `git diff --check` passed. Model tests cover full-library counts, progress/action coexistence, multiple window sizes, and WezTerm image position. User reviewed the dashboard in WezTerm and approved it.

Milestone 3 evidence: model tests exercise wide, compact, stacked, and list-only layouts, selection and search across pane switches, bounds, and native-image invalidation after resizing. User reviewed the responsive layout in WezTerm and approved it.

Milestone 4 checks: `go test ./...`, `go test -race ./...`, `go vet ./...`, and `git diff --check` passed. Tests cover letterboxing, asynchronous preparation, cached reuse, stale generation rejection, resize, and fallback. User reviewed the previews in WezTerm and approved them.

Milestone 5 evidence: SQLite-backed model tests cover older entries beyond the Recent 30, on-demand loading, independent searches, favorites, preserved selection, new sync items, and WezTerm preview requests. User reviewed Archive browsing in WezTerm and approved it.

Milestone 6 evidence: local HTTP and SQLite model tests cover manual no-op and queued refreshes, retrying a failed preview through sync, selected older-Archive retry, and failed retry persistence. User confirmed `s` and the corrected `p` behavior in WezTerm.

Feedback follow-up: `p` appeared inert after `s` because it accepted only entries with a stored preview error. It now refreshes any selected APOD with a preview URL, replaces cached files atomically, invalidates rendered-image caches, and shows feedback in the smallest layout. Tests cover a successful cached refresh, a failed refresh preserving the cached file, and a missing URL.

Milestone 7 evidence: local HTTP/SQLite tests cover metadata/full-image reuse, random-image reuse, offline today/dated requests, atomic CLI output copies, interrupted transfers with and without existing destinations, retry after failure, empty-cache repair, and cached-video rejection. These checks establish cache/file behavior; desktop wallpaper application and Windows/macOS filesystem behavior require platform-specific testing.

Milestone 7 checks: `go test ./...`, `go test -race ./...`, `go vet ./...`, and `git diff --check` passed. Feature inventory added in `FEATURES.md`. Awaiting user feedback.
