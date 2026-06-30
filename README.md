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

# Set the next wallpaper from your persisted favorites list
astrowall --cycle-favorites
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--api-key` | `-a` | `DEMO_KEY` | NASA API key |
| `--random` | `-r` | `false` | Fetch a random APOD |
| `--verbose` | `-v` | `true` | Show image details after setting wallpaper |
| `--output` | `-o` | `~/Pictures/apod_wallpaper.jpg` | Custom save path |
| `--date` | `-d` | | Fetch APOD for a specific date (YYYY-MM-DD) |
| `--tui` | | `false` | Launch the text-based APOD browser |
| `--cycle-favorites` | | `false` | Set the next favorite wallpaper from the local library |
| `--sync-only` | | `false` | Sync the local APOD library and preview cache, then exit |
| `--version` | | | Show version and exit |

## API Key

The tool resolves the API key in this order:

1. `--api-key` flag
2. `NASA_API_KEY` environment variable
3. Falls back to `DEMO_KEY` (rate-limited to 30 requests/hour)

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

- left column: separate `Recent APODs` and `Favorites` panes
- right pane: the selected item's date, type, cache status, and description
- `j` / `k`: move through the list
- `Tab` / `Shift+Tab`: switch between `Recent APODs` and `Favorites`
- `q`: quit
- `f`: favorite or unfavorite the selected item
- `Enter`: fetch the selected day's image and set it as wallpaper

The preview image is rendered in-terminal from the cached preview file using ANSI half-block output, with graceful fallback to text metadata if rendering fails.

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
