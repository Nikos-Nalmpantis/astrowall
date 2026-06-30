package main

import (
	"database/sql"
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
		apiKey   string
		random   bool
		verbose  bool
		output   string
		date     string
		tuiMode  bool
		syncOnly bool
		showVer  bool
	)

	flag.StringVarP(&apiKey, "api-key", "a", "", "NASA API key (or set NASA_API_KEY env var, default: DEMO_KEY)")
	flag.BoolVarP(&random, "random", "r", false, "Fetch a random APOD instead of today's")
	flag.BoolVarP(&verbose, "verbose", "v", true, "Show details about the image")
	flag.StringVarP(&output, "output", "o", "", "Save image to this path (default: ~/Pictures/apod_wallpaper.jpg)")
	flag.StringVarP(&date, "date", "d", "", "Fetch APOD for a specific date (YYYY-MM-DD)")
	flag.BoolVar(&tuiMode, "tui", false, "Launch the text-based APOD browser")
	flag.BoolVar(&syncOnly, "sync-only", false, "Sync the local APOD library and preview cache, then exit")
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

	key := resolveAPIKey(apiKey)

	if random && date != "" {
		fmt.Fprintln(os.Stderr, "Error: --random and --date cannot be used together.")
		os.Exit(1)
	}
	if tuiMode && (random || date != "" || output != "") {
		fmt.Fprintln(os.Stderr, "Error: --tui cannot be combined with --random, --date, or --output.")
		os.Exit(1)
	}
	if tuiMode && syncOnly {
		fmt.Fprintln(os.Stderr, "Error: --tui and --sync-only cannot be used together.")
		os.Exit(1)
	}

	paths, db, err := initializeLibrary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing local library: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	result, err := runStartupSync(db, paths, key, time.Now(), syncOnly, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error syncing APOD library: %v\n", err)
		os.Exit(1)
	}
	if syncOnly {
		printSyncSummary(os.Stdout, result)
		return
	}
	if tuiMode {
		if err := runTUI(db, key); err != nil {
			fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
			os.Exit(1)
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
		uri := buildAPODURL(key, random, date)
		apod, err = fetchAPOD(uri)
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

		imageURL := apod.HDURL
		if imageURL == "" {
			imageURL = apod.URL
		}

		err = downloadImage(imageURL, imagePath)
		if err != nil {
			if random {
				continue
			}
			fmt.Fprintf(os.Stderr, "Error downloading image: %v\n", err)
			os.Exit(1)
		}
		break
	}

	if err := setWallpaper(imagePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error setting wallpaper: %v\n", err)
		os.Exit(1)
	}

	if verbose {
		printDetails(&apod, imagePath)
	}
}

func resolveAPIKey(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if value := os.Getenv("NASA_API_KEY"); value != "" {
		return value
	}
	return "DEMO_KEY"
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
		"Synced %d APOD items and cached %d previews for %s through %s.\n",
		result.FetchedCount,
		result.PreviewedCount,
		result.StartDate,
		result.EndDate,
	)
}

func printDetails(apod *APODResponse, imagePath string) {
	fmt.Printf("- Title: %s\n", apod.Title)
	fmt.Printf("- Date: %s\n", apod.Date)
	if apod.Explanation != "" {
		explanation := fmt.Sprintf("- Explanation: %s", apod.Explanation)
		fmt.Println(wordwrap.String(explanation, 100))
	}
	if len(apod.Date) >= 10 {
		page := fmt.Sprintf("https://apod.nasa.gov/apod/ap%s%s%s.html",
			apod.Date[2:4], apod.Date[5:7], apod.Date[8:10])
		fmt.Printf("- APOD page: %s\n", page)
	}
	fmt.Printf("- Saved to: %s\n", imagePath)
}
