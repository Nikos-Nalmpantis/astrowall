package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"
)

var version = "dev"

func main() {
	var (
		apiKey  string
		random  bool
		verbose bool
		output  string
		date    string
		showVer bool
	)

	flag.StringVarP(&apiKey, "api-key", "a", "", "NASA API key (or set NASA_API_KEY env var, default: DEMO_KEY)")
	flag.BoolVarP(&random, "random", "r", false, "Fetch a random APOD instead of today's")
	flag.BoolVarP(&verbose, "verbose", "v", true, "Show details about the image")
	flag.StringVarP(&output, "output", "o", "", "Save image to this path (default: ~/Pictures/apod_wallpaper.jpg)")
	flag.StringVarP(&date, "date", "d", "", "Fetch APOD for a specific date (YYYY-MM-DD)")
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

	// Resolve API key: --api-key > NASA_API_KEY env > DEMO_KEY fallback.
	key := apiKey
	if key == "" {
		key = os.Getenv("NASA_API_KEY")
	}
	if key == "" {
		key = "DEMO_KEY"
	}

	if random && date != "" {
		fmt.Fprintln(os.Stderr, "Error: --random and --date cannot be used together.")
		os.Exit(1)
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

func printDetails(apod *APODResponse, imagePath string) {
	fmt.Printf("- Wallpaper set: %s\n", apod.Title)
	fmt.Printf("- Date: %s\n", apod.Date)
	if apod.Explanation != "" {
		fmt.Printf("- Explanation: %s\n", apod.Explanation)
	}
	if len(apod.Date) >= 10 {
		page := fmt.Sprintf("https://apod.nasa.gov/apod/ap%s%s%s.html",
			apod.Date[2:4], apod.Date[5:7], apod.Date[8:10])
		fmt.Printf("- APOD page: %s\n", page)
	}
	fmt.Printf("- Saved to: %s\n", imagePath)
}
