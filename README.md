# astrowall

A CLI tool that fetches [NASA's Astronomy Picture of the Day](https://apod.nasa.gov/) and sets it as your desktop wallpaper.

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
```

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--api-key` | `-a` | `DEMO_KEY` | NASA API key |
| `--random` | `-r` | `false` | Fetch a random APOD |
| `--verbose` | `-v` | `true` | Show image details after setting wallpaper |
| `--output` | `-o` | `~/Pictures/apod_wallpaper.jpg` | Custom save path |
| `--date` | `-d` | | Fetch APOD for a specific date (YYYY-MM-DD) |
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
