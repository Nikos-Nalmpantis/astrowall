package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/muesli/reflow/wordwrap"
	flag "github.com/spf13/pflag"
)

var version = "dev"

func main() {
	var (
		apiKey         string
		random         bool
		verbose        bool
		output         string
		date           string
		imageProtocol  string
		tuiMode        bool
		cycleFavorites bool
		syncOnly       bool
		saveKey        bool
		removeKey      bool
		showVer        bool
	)

	flag.StringVarP(&apiKey, "api-key", "a", "", "NASA API key (or set NASA_API_KEY env var, default: DEMO_KEY)")
	flag.BoolVarP(&random, "random", "r", false, "Fetch a random APOD instead of today's")
	flag.BoolVarP(&verbose, "verbose", "v", true, "Show details about the image")
	flag.StringVarP(&output, "output", "o", "", "Save image to this path (default: ~/Pictures/apod_wallpaper.jpg)")
	flag.StringVarP(&date, "date", "d", "", "Fetch APOD for a specific date (YYYY-MM-DD)")
	flag.StringVar(&imageProtocol, "image-protocol", "auto", "TUI image protocol: auto, kitty, wezterm, or ansi")
	flag.BoolVar(&tuiMode, "tui", false, "Launch the text-based APOD browser")
	flag.BoolVar(&cycleFavorites, "cycle-favorites", false, "Set the next favorite wallpaper from the local library")
	flag.BoolVar(&syncOnly, "sync-only", false, "Sync the local APOD library and preview cache, then exit")
	flag.BoolVar(&saveKey, "save-api-key", false, "Validate and securely save a NASA API key")
	flag.BoolVar(&removeKey, "remove-api-key", false, "Remove the saved NASA API key")
	flag.BoolVar(&showVer, "version", false, "Show version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "astrowall - fetch NASA's Astronomy Picture of the Day and set it as your wallpaper\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  astrowall [flags]\n\nFlags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if showVer {
		fmt.Printf("astrowall %s\n", version)
		return
	}

	protocol, err := parseImageProtocol(imageProtocol)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if random && date != "" {
		fmt.Fprintln(os.Stderr, "Error: --random and --date cannot be used together.")
		os.Exit(1)
	}
	if saveKey && removeKey {
		fmt.Fprintln(os.Stderr, "Error: --save-api-key and --remove-api-key cannot be used together.")
		os.Exit(1)
	}
	if saveKey || removeKey {
		if apiKey != "" || random || date != "" || output != "" || tuiMode || cycleFavorites || syncOnly || protocol != imageProtocolAuto {
			fmt.Fprintln(os.Stderr, "Error: API key management flags cannot be combined with other operation modes.")
			os.Exit(1)
		}
		if removeKey {
			if err := removeSavedAPIKey(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Removed saved NASA API key.")
			return
		}
		key, err := readAPIKey(os.Stdin, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := saveAPIKey(key); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("NASA API key validated and saved securely.")
		return
	}

	if tuiMode && (random || date != "" || output != "") {
		fmt.Fprintln(os.Stderr, "Error: --tui cannot be combined with --random, --date, or --output.")
		os.Exit(1)
	}
	if !tuiMode && protocol != imageProtocolAuto {
		fmt.Fprintln(os.Stderr, "Error: --image-protocol can only be used with --tui.")
		os.Exit(1)
	}
	if tuiMode && syncOnly {
		fmt.Fprintln(os.Stderr, "Error: --tui and --sync-only cannot be used together.")
		os.Exit(1)
	}
	if cycleFavorites && (random || date != "" || output != "" || tuiMode || syncOnly) {
		fmt.Fprintln(os.Stderr, "Error: --cycle-favorites cannot be combined with --random, --date, --output, --tui, or --sync-only.")
		os.Exit(1)
	}

	var (
		key                   string
		keySource             apiKeySource
		keyErr                error
		lookupCredentialInTUI bool
	)
	if tuiMode {
		if explicitKey, explicitSource, ok := resolveExplicitAPIKey(apiKey, os.Getenv); ok {
			key, keySource = explicitKey, explicitSource
		} else {
			key, keySource = "DEMO_KEY", apiKeySourceDemo
			lookupCredentialInTUI = true
		}
	} else {
		key, keySource, keyErr = resolveAPIKeyWithSource(apiKey, os.Getenv)
	}
	if keyErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v; using DEMO_KEY.\n", keyErr)
	}

	paths, db, err := initializeLibrary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing local library: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if tuiMode {
		if err := runTUI(db, paths, key, keySource, lookupCredentialInTUI, protocol); err != nil {
			fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
			os.Exit(1)
		}
		return
	}

	result, err := runStartupSync(db, paths, key, time.Now(), syncOnly, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error syncing APOD library: %v\n", err)
		os.Exit(1)
	}
	if syncOnly {
		printSyncSummary(os.Stdout, result)
		return
	}
	if cycleFavorites {
		result, err := cycleFavoriteWallpaper(db, paths, key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error cycling favorite wallpaper: %v\n", err)
			os.Exit(1)
		}
		printFavoriteCycleSummary(os.Stdout, result)
		if result.HistoryError != nil {
			fmt.Fprintf(os.Stderr, "Warning: wallpaper applied, but %v\n", result.HistoryError)
		}
		return
	}
	imagePath, err := resolveImagePath(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving output path: %v\n", err)
		os.Exit(1)
	}

	var apod APODResponse
	for {
		apod, err = resolveWallpaperAPOD(db, key, random, date, time.Now())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching APOD: %v\n", err)
			os.Exit(1)
		}

		if apod.MediaType != "image" {
			if !random {
				fmt.Fprintf(os.Stderr, "Today's APOD is a %s, not an image. Use --random or --date to try another.\n", apod.MediaType)
				os.Exit(1)
			}
			continue
		}

		cachedPath, err := ensureHDImageCached(db, paths, APODRecord{Date: apod.Date}, key)
		if err != nil {
			var localErr localImageError
			if random && !errors.As(err, &localErr) {
				continue
			}
			fmt.Fprintf(os.Stderr, "Error downloading image: %v\n", err)
			os.Exit(1)
		}
		if err := copyImageAtomic(cachedPath, imagePath); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving image: %v\n", err)
			os.Exit(1)
		}
		break
	}

	historyErr, err := applyRecordedWallpaper(db, apod.Date, imagePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting wallpaper: %v\n", err)
		os.Exit(1)
	}
	if historyErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: wallpaper applied, but %v\n", historyErr)
	}

	if verbose {
		printDetails(&apod, imagePath)
	}
}

func resolveAPIKey(flagValue string) string {
	key, _, _ := resolveAPIKeyWithSource(flagValue, os.Getenv)
	return key
}

func initializeLibrary() (AppPaths, *sql.DB, error) {
	paths, err := resolveAppPaths()
	if err != nil {
		return AppPaths{}, nil, err
	}

	db, err := openLibrary(paths.DBPath)
	if err != nil {
		return AppPaths{}, nil, err
	}

	return paths, db, nil
}

func runStartupSync(db *sql.DB, paths AppPaths, apiKey string, now time.Time, syncOnly bool, stderr io.Writer) (SyncResult, error) {
	result, err := syncAPODArchive(db, paths, apiKey, now)
	if err != nil {
		if syncOnly {
			return SyncResult{}, err
		}
		fmt.Fprintf(stderr, "Warning: could not sync local APOD library: %v\n", err)
		return SyncResult{}, nil
	}
	return result, nil
}

func printSyncSummary(w io.Writer, result SyncResult) {
	if result.AlreadyUpToDate {
		fmt.Fprintln(w, "Local APOD library is already up to date.")
		return
	}

	fmt.Fprintf(
		w,
		"Synced %d APOD items and cached %d previews (%d preview errors) for %s through %s.\n",
		result.FetchedCount,
		result.PreviewedCount,
		result.PreviewFailedCount,
		result.StartDate,
		result.EndDate,
	)
}

func printFavoriteCycleSummary(w io.Writer, result FavoriteCycleResult) {
	fmt.Fprintf(w, "Set favorite wallpaper to %s (%s).\n", result.Title, result.Date)
	fmt.Fprintf(w, "- Image: %s\n", result.ImagePath)
}

func printDetails(apod *APODResponse, imagePath string) {
	fmt.Printf("- Title: %s\n", apod.Title)
	fmt.Printf("- Date: %s\n", apod.Date)
	if apod.Explanation != "" {
		explanation := fmt.Sprintf("- Explanation: %s", apod.Explanation)
		fmt.Println(wordwrap.String(explanation, 100))
	}
	if len(apod.Date) >= 10 {
		page := apodPageURL(apod.Date)
		fmt.Printf("- APOD page: %s\n", page)
	}
	fmt.Printf("- Saved to: %s\n", imagePath)
}
