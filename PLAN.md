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

### 3. Responsive layout and empty states — pending

Give narrow terminals usable pane arrangements and contextual hints. Clarify empty library, empty favorites, no search results, and unavailable preview states. Verify keyboard navigation, filtering, geometry, and image cleanup.

### 4. Image presentation and responsiveness — pending

Keep ANSI previews proportional and centered; prepare expensive previews without blocking navigation; disregard stale results. Verify portrait/panorama rendering, resize, and rapid selection changes. Check the final appearance in direct WezTerm.

### 5. Full-library browsing — pending

Expose stored entries older than the newest 30, with useful search/navigation and predictable loading. Verify selection, filtering, favorites, and cache reuse.

## Later ideas

- Manual refresh and preview retry from the TUI.
- Cache-first and atomic full-image downloads across CLI and TUI.
- Skip videos when cycling favorites.
- Wallpaper history and scheduled rotation.
- Extract shared wallpaper/cache operations and simplify repeated database row scanning where touched by a milestone.

## Verification notes

For milestone 1, the claim is visual consistency without breaking sizing, navigation, or native image placement. Existing `tui_test.go` geometry and WezTerm lifecycle tests cover layout behavior; `go test ./...` and `go vet ./...` cover project integration. A real WezTerm visual check is still needed to judge the aesthetics and native-image appearance; automated tests cannot establish those perceptual details.

Milestone 1 checks: `go test ./...`, `go test -race ./...`, `go vet ./...`, and `git diff --check` passed. User reviewed the look in WezTerm and approved it.

Milestone 2 checks: `go test ./...`, `go test -race ./...`, `go vet ./...`, and `git diff --check` passed. Model tests cover full-library counts, progress/action coexistence, multiple window sizes, and WezTerm image position. User reviewed the dashboard in WezTerm and approved it.
