# astrowall

A CLI tool that fetches [NASA's Astronomy Picture of the Day](https://apod.nasa.gov/) and sets it as your desktop wallpaper.

Astrowall now also keeps a small local library in SQLite plus a preview-image cache, so startup can sync only missing APOD dates instead of re-fetching everything every time.

## Installation

### From source

```bash
go install github.com/Nikos-Nalmpantis/astrowall@latest
```

### Build locally

```bash
git clone https://github.com/Nikos-Nalmpantis/astrowall.git
cd astrowall
go build -o astrowall .
```

## Usage

```bash
# Today's APOD (uses DEMO_KEY by default)
astrowall

# With your own NASA API key
astrowall --api-key YOUR_KEY

# Securely save a NASA API key in the OS credential manager
astrowall --save-api-key

# Remove the saved NASA API key
astrowall --remove-api-key

# Random APOD
astrowall --random

# APOD for a specific date
astrowall --date 2024-09-27

# Save to a custom path
astrowall --output /path/to/wallpaper.jpg

# Quiet mode (no output)
astrowall --verbose=false

# Sync the local APOD library and preview cache without setting wallpaper
astrowall --sync-only

# Browse the local APOD library in a text TUI
astrowall --tui

# Force Kitty graphics in a compatible terminal or configured tmux session
astrowall --tui --image-protocol kitty

# Set the next wallpaper from your persisted favorites list
astrowall --cycle-favorites
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--api-key` | `-a` | | NASA API key override; final fallback is `DEMO_KEY` |
| `--save-api-key` | | `false` | Interactively validate and securely save a NASA API key |
| `--remove-api-key` | | `false` | Remove the saved NASA API key |
| `--random` | `-r` | `false` | Fetch a random APOD |
| `--verbose` | `-v` | `true` | Show image details after setting wallpaper |
| `--output` | `-o` | `~/Pictures/apod_wallpaper.jpg` | Custom save path |
| `--date` | `-d` | | Fetch APOD for a specific date (YYYY-MM-DD) |
| `--tui` | | `false` | Launch the text-based APOD browser |
| `--image-protocol` | | `auto` | TUI image renderer: `auto`, `kitty`, `wezterm`, or `ansi` |
| `--cycle-favorites` | | `false` | Set the next favorite wallpaper from the local library |
| `--sync-only` | | `false` | Sync the local APOD library and preview cache, then exit |
| `--version` | | | Show version and exit |

## API Key

The tool resolves the API key in this order:

1. `--api-key` flag
2. `NASA_API_KEY` environment variable
3. Saved OS credential
4. Falls back to `DEMO_KEY` (rate-limited to 30 requests/hour)

`astrowall --save-api-key` reads the key without terminal echo, validates it with NASA, and stores it in the operating system credential manager: Secret Service on Linux, Keychain on macOS, or Credential Manager on Windows. The key is never stored in Astrowall's database or configuration files.

For heavier usage, get a free API key at [api.nasa.gov](https://api.nasa.gov/).

```bash
# Set it once in your shell profile
export NASA_API_KEY="your-key-here"
```

## Local Library Cache

On startup, astrowall now:

1. Opens a local SQLite library database
2. Checks the latest APOD date already stored
3. Fetches only the missing dates needed to catch up to today
4. Caches preview images locally for future TUI browsing

By default, metadata is stored under your XDG data directory (usually `~/.local/share/astrowall/`) and preview/full image cache files are stored under your XDG cache directory (usually `~/.cache/astrowall/`).

## Text TUI Mode

`astrowall --tui` launches the first interactive browser for the local APOD library.

Current behavior:

- dashboard header: total local APOD count and favorites count
- wide terminals: separate `Recent APODs` and `Favorites` panes beside the detail view
- `b`: switch the primary pane between the latest 30 Recent items and the full local Archive (loaded on demand); searches and selection stay independent
- compact terminals: the active list beside the detail view; narrow terminals stack the active list above it
- very small terminals: a focused list view; `Tab` still switches between Recent and Favorites
- right pane: the selected item's date, type, cache status, and description
- separate sync progress and action-feedback lines beneath the panes
- helpful messages for an empty library, empty favorites, no search matches, and unavailable previews
- `j` / `k`: move through the list
- `/`: search the active pane by title, date, or description
- `Enter`: apply the current search
- `Esc`: cancel search editing or clear an applied search
- `Tab` / `Shift+Tab`: switch between the primary pane (Recent or Archive) and Favorites
- `q`: quit
- `f`: favorite or unfavorite the selected item
- `s`: refresh missing APODs and retry failed previews; pressing it during a library operation queues one follow-up refresh
- `p`: re-download the selected APOD's preview (including a cached one) without refreshing the entire library; reports when no preview URL is available
- `Enter`: fetch the selected day's image and set it as wallpaper
- `d`: toggle between the selected APOD image and description
- `a`: add, replace, or remove the saved NASA API key

In `auto` mode, direct WezTerm sessions use WezTerm's standard Kitty image placement support, while compatible Kitty and Ghostty sessions use Kitty Unicode placeholders. Other terminals use ANSI half-block output. Auto mode remains conservative inside tmux; force Kitty graphics with `--image-protocol kitty` after confirming support and enabling passthrough:

Previews are fitted and centered without stretching. ANSI previews render in the background while you continue browsing, and native preview errors fall back to ANSI for the selected item.

```tmux
set -g allow-passthrough on
```

Use `--image-protocol ansi` to always use the portable half-block renderer.

The WezTerm backend is intended for direct sessions and falls back to ANSI inside tmux.

## Favorite Cycling

`astrowall --cycle-favorites` cycles through your persisted favorites without launching the TUI.

Current behavior:

- loads the full favorites list from SQLite
- remembers the last favorite wallpaper it set
- advances to the next favorite on each run
- reuses the cached full image when available before downloading again

## Supported Platforms

### Linux

| Desktop Environment | Tool Used |
|---|---|
| GNOME / Unity / Pantheon / Budgie | `gsettings` (sets both light and dark wallpaper) |
| KDE Plasma | `plasma-apply-wallpaperimage` |
| Hyprland | `swww` |
| Sway | `swaymsg` |
| XFCE | `xfconf-query` |
| Cinnamon | `gsettings` |
| MATE | `gsettings` |

Unrecognized DEs fall back to GNOME `gsettings` since many DEs are GNOME-based.

### macOS

Uses AppleScript via `osascript` to set the wallpaper on all desktops.

### Windows

Uses the `SystemParametersInfoW` Win32 API directly.

## Media Type Handling

The NASA APOD API sometimes returns videos instead of images. When this happens:

- **Normal mode**: prints an error and suggests using `--random` or `--date`
- **Random mode**: automatically retries until an image is found

## License

MIT
