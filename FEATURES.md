# Astrowall feature inventory

## Wallpaper CLI

- Today's APOD, a specific date, or a random APOD.
- HD images preferred, with the standard image URL as a fallback.
- Default wallpaper output under `~/Pictures`, or a custom output path with parent directories created automatically.
- Verbose title/date/description, NASA page link, and saved-path output; quiet mode for normal wallpaper requests.
- Persisted favorite cycling in newest-first order, wrapping around and remembering the last successful choice.
- Cache-first metadata for today/dated requests and shared full-image reuse across CLI, TUI, and cycling.
- Atomic image downloads and output copies; interrupted transfers preserve existing files, and missing/empty cache files are retried.
- Video-aware normal mode and automatic image retries in random mode.
- Version output and command-line help.

## Library and synchronization

- SQLite APOD metadata: date, title, explanation, media type, URLs, copyright, cache paths, preview errors, favorites, and fetch time.
- Separate data, preview-cache, and full-image-cache directories, respecting `XDG_DATA_HOME` and `XDG_CACHE_HOME`.
- First sync fetches the latest 30 days; later syncs catch up from the latest stored date.
- Sync-only CLI mode with a completion summary.
- Background TUI synchronization with progress and preview-error counts.
- Manual refresh (`s`), including failed-preview retries, with one queued refresh during an active library operation.
- Selected-preview refresh (`p`) for cached or unavailable previews, including older Archive entries.
- Preview refresh replaces cached files atomically and invalidates rendered previews.
- Metadata refresh preserves favorites and full-image cache paths.
- Wallpaper modes warn and continue when startup synchronization fails.

## Astronomy dashboard

- Latest 30 Recent APODs, an on-demand full local Archive (`b`), and persisted Favorites.
- Separate searches and selections in Recent, Archive, and Favorites.
- Search by title, date, or description; apply with Enter and cancel/clear with Esc.
- Keyboard navigation (`j`/`k`), pane switching (Tab/Shift+Tab), and contextual shortcut hints/help (`?`).
- Library/favorite counts in a width-aware dashboard header.
- Separate background-sync progress and action-feedback lines.
- Responsive wide two-list layout, compact active-list layout, narrow stacked layout, and very-small-terminal list view.
- Empty-library, empty-favorites, no-match, and unavailable-preview guidance.
- Selected title/date/media type, favorite badge, and preview-error details.
- Image/description toggle (`d`); paged and half-page description scrolling.
- Set the selected image as wallpaper (Enter) and toggle its favorite state (`f`).
- Open the NASA APOD page (`o`) or original media (`u`) in a browser, including videos.
- Add, replace, or remove the saved NASA API key (`a`).

## Terminal image rendering

- Automatic terminal detection or explicit `auto`, `wezterm`, `kitty`, and `ansi` selection.
- Direct WezTerm Kitty image placements and Kitty/Ghostty Unicode placeholders.
- Conservative tmux auto-detection, with explicitly selectable Kitty passthrough.
- Portable ANSI half-block fallback when native graphics are unavailable or fail.
- Proportion-preserving, centered previews for portrait and panoramic images.
- Asynchronous ANSI preparation with a bounded six-entry rendered-preview cache.
- Cancellation/stale-result rejection during navigation, resize, and view changes.
- Native placement updates and cleanup when changing selection, searching, opening dialogs/help, resizing, and quitting.

## API credentials

- Precedence: explicit CLI key, `NASA_API_KEY`, saved OS credential, then `DEMO_KEY`.
- Interactive hidden key entry and NASA validation before saving.
- OS credential storage: Linux Secret Service, macOS Keychain, and Windows Credential Manager.
- CLI save/remove operations and TUI credential management with current-key-source feedback.
- Background saved-key lookup with a timeout in the TUI.
- API request errors avoid exposing keys and response bodies.

## Desktop integration

- Linux: GNOME/Unity/Pantheon/Budgie, KDE Plasma, Hyprland, Sway, XFCE, Cinnamon, and MATE.
- GNOME sets both light and dark wallpaper variants; unknown Linux desktops try GNOME settings.
- macOS: AppleScript sets the wallpaper on all desktops.
- Windows: native `SystemParametersInfoW` wallpaper API.
- Browser launch uses `xdg-open`, `open`, or Windows URL handling.

## Current boundaries and next opportunities

- Archive means the complete **local** library; historical NASA backfill is not yet available.
- Favorite cycling currently includes videos, which can stop a cycle with an error.
- Cached images are checked for regular-file status and nonzero size, not decoded for integrity; pre-existing nonempty partial files may need removal.
- Wallpaper history, scheduled rotation, and a displayed current-wallpaper indicator are not yet implemented.
- `tui.go` still owns substantial navigation, sync, rendering, and image lifecycle logic; focused extractions can make later dashboard work easier.

Milestone progress and visual preferences are recorded in [PLAN.md](PLAN.md).
