package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type apodListItem struct {
	record APODRecord
}

func (i apodListItem) FilterValue() string {
	return i.record.Title + " " + i.record.Date
}

func (i apodListItem) Title() string {
	return i.record.Title
}

func (i apodListItem) Description() string {
	if i.record.Favorite {
		return i.record.Date + " ★"
	}
	return i.record.Date
}

type wallpaperAppliedMsg struct {
	date  string
	path  string
	title string
	err   error
}

type favoriteToggledMsg struct {
	date     string
	favorite bool
	err      error
}

type tuiModel struct {
	db          *sql.DB
	paths       AppPaths
	list        list.Model
	detail      viewport.Model
	records     []APODRecord
	apiKey      string
	status      string
	width       int
	height      int
	ready       bool
	loading     bool
	listStyle   lipgloss.Style
	detailStyle lipgloss.Style
	statusStyle lipgloss.Style
	helpStyle   lipgloss.Style
	previewArea imageArea
}

type imageArea struct {
	width  int
	height int
}

func newTUIModel(records []APODRecord, apiKey string) tuiModel {
	items := make([]list.Item, 0, len(records))
	for _, record := range records {
		items = append(items, apodListItem{record: record})
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = true

	listModel := list.New(items, delegate, 0, 0)
	listModel.Title = "Recent APODs"
	listModel.SetShowHelp(false)
	listModel.SetShowStatusBar(false)
	listModel.SetShowPagination(false)
	listModel.SetShowFilter(false)
	listModel.SetFilteringEnabled(false)
	listModel.DisableQuitKeybindings()

	detail := viewport.New()
	detail.SetContent("No APODs loaded.")

	m := tuiModel{
		list:        listModel,
		detail:      detail,
		records:     records,
		apiKey:      apiKey,
		status:      "j/k move • enter set wallpaper • f favorite • q quit",
		listStyle:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
		detailStyle: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
		statusStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		helpStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
	}
	m.refreshDetail(false)
	return m
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.resize()
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "f":
			if m.loading {
				return m, nil
			}
			record := m.selectedRecord()
			if record.Date == "" {
				return m, nil
			}
			m.status = fmt.Sprintf("Toggling favorite for %s…", record.Title)
			return m, toggleFavoriteCmd(m.db, record.Date)
		case "enter":
			if m.loading {
				return m, nil
			}
			record := m.selectedRecord()
			if record.Date == "" {
				return m, nil
			}
			m.loading = true
			m.status = fmt.Sprintf("Setting wallpaper for %s…", record.Title)
			return m, applyWallpaperCmd(m.db, m.paths, record, m.apiKey)
		}

	case wallpaperAppliedMsg:
		m.loading = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Wallpaper update failed: %v", msg.err)
			return m, nil
		}
		for i := range m.records {
			if m.records[i].Date == msg.date {
				m.records[i].HDPath = msg.path
				break
			}
		}
		m.syncListItems()
		m.refreshDetail(false)
		m.status = fmt.Sprintf("Wallpaper set to %s", msg.title)
		return m, nil

	case favoriteToggledMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Favorite update failed: %v", msg.err)
			return m, nil
		}
		for i := range m.records {
			if m.records[i].Date == msg.date {
				m.records[i].Favorite = msg.favorite
				break
			}
		}
		m.syncListItems()
		m.refreshDetail(false)
		if msg.favorite {
			m.status = fmt.Sprintf("Added %s to favorites", m.selectedRecord().Title)
		} else {
			m.status = fmt.Sprintf("Removed %s from favorites", m.selectedRecord().Title)
		}
		return m, nil
	}

	before := m.list.Index()
	var listCmd tea.Cmd
	m.list, listCmd = m.list.Update(msg)
	if m.list.Index() != before {
		m.refreshDetail(true)
	}

	var detailCmd tea.Cmd
	m.detail, detailCmd = m.detail.Update(msg)
	return m, tea.Batch(listCmd, detailCmd)
}

func (m tuiModel) View() tea.View {
	if !m.ready {
		view := tea.NewView("Loading astrowall TUI…")
		view.AltScreen = true
		return view
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		m.listStyle.Render(m.list.View()),
		m.detailStyle.Render(m.detail.View()),
	)

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		panes,
		m.statusStyle.Render(m.status),
		m.helpStyle.Render("List uses built-in vim navigation. Viewport scrolls with j/k when focused by Bubble Tea update flow."),
	)

	view := tea.NewView(body)
	view.AltScreen = true
	return view
}

func (m *tuiModel) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	contentHeight := max(8, m.height-4)
	leftWidth := max(30, m.width/3)
	rightWidth := max(40, m.width-leftWidth-6)

	m.list.SetSize(leftWidth, contentHeight)
	m.detail.SetWidth(rightWidth)
	m.detail.SetHeight(contentHeight)
	m.previewArea = imageArea{width: max(12, rightWidth-4), height: max(6, contentHeight/2)}
	m.refreshDetail(false)
}

func (m *tuiModel) refreshDetail(resetScroll bool) {
	record := m.selectedRecord()
	if record.Date == "" {
		m.detail.SetContent("No APOD records are available yet. Run astrowall --sync-only first.")
		if resetScroll {
			m.detail.GotoTop()
		}
		return
	}

	var parts []string
	parts = append(parts, record.Title)
	parts = append(parts, fmt.Sprintf("Date: %s", record.Date))
	parts = append(parts, fmt.Sprintf("Type: %s", record.MediaType))
	if record.Favorite {
		parts = append(parts, "Favorite: yes")
	}
	if record.PreviewPath != "" {
		if preview, err := renderPreviewBlock(record.PreviewPath, m.previewArea.width, m.previewArea.height); err == nil && preview != "" {
			parts = append(parts, preview)
		} else {
			parts = append(parts, fmt.Sprintf("Preview cache: %s", record.PreviewPath))
			parts = append(parts, "Preview could not be rendered in this terminal session.")
		}
	}
	parts = append(parts, "")
	parts = append(parts, strings.TrimSpace(record.Description))
	m.detail.SetContent(strings.Join(parts, "\n"))
	if resetScroll {
		m.detail.GotoTop()
	}
}

func (m tuiModel) selectedRecord() APODRecord {
	item := m.list.SelectedItem()
	if item == nil {
		return APODRecord{}
	}
	selected, ok := item.(apodListItem)
	if !ok {
		return APODRecord{}
	}
	return selected.record
}

func runTUI(db *sql.DB, apiKey string) error {
	records, err := listRecentAPODs(db, 30)
	if err != nil {
		return err
	}
	paths, err := resolveAppPaths()
	if err != nil {
		return err
	}

	model := newTUIModel(records, apiKey)
	model.db = db
	model.paths = paths
	program := tea.NewProgram(model)
	_, err = program.Run()
	return err
}

func (m *tuiModel) syncListItems() {
	items := make([]list.Item, 0, len(m.records))
	for _, record := range m.records {
		items = append(items, apodListItem{record: record})
	}
	selected := m.list.Index()
	m.list.SetItems(items)
	if len(items) == 0 {
		m.list.Select(0)
		return
	}
	if selected >= len(items) {
		selected = len(items) - 1
	}
	m.list.Select(selected)
}

func applyWallpaperCmd(db *sql.DB, paths AppPaths, record APODRecord, apiKey string) tea.Cmd {
	return func() tea.Msg {
		cachedPath, err := ensureHDImageCached(db, paths, record, apiKey)
		if err != nil {
			return wallpaperAppliedMsg{date: record.Date, title: record.Title, err: err}
		}
		if err := setWallpaper(cachedPath); err != nil {
			return wallpaperAppliedMsg{date: record.Date, title: record.Title, path: cachedPath, err: err}
		}

		return wallpaperAppliedMsg{date: record.Date, title: record.Title, path: cachedPath, err: nil}
	}
}

func toggleFavoriteCmd(db *sql.DB, date string) tea.Cmd {
	return func() tea.Msg {
		favorite, err := toggleFavorite(db, date)
		return favoriteToggledMsg{date: date, favorite: favorite, err: err}
	}
}
func ensureHDImageCached(db *sql.DB, paths AppPaths, record APODRecord, apiKey string) (string, error) {
	storedRecord, err := recordByDate(db, record.Date)
	if err == nil {
		record = storedRecord
	}

	if record.HDPath != "" {
		if _, err := os.Stat(record.HDPath); err == nil {
			return record.HDPath, nil
		}
	}

	apod, err := fetchAPOD(buildAPODURL(apiKey, false, record.Date))
	if err != nil {
		return "", err
	}
	if apod.MediaType != "image" {
		return "", fmt.Errorf("%s is a %s, not an image", record.Date, apod.MediaType)
	}

	imageURL := apod.HDURL
	if imageURL == "" {
		imageURL = apod.URL
	}
	if imageURL == "" {
		return "", fmt.Errorf("no downloadable image URL for %s", record.Date)
	}

	fullPath := filepath.Join(paths.FullDir, record.Date+fileExtensionFromURL(imageURL))
	if _, err := os.Stat(fullPath); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("checking HD cache for %s: %w", record.Date, err)
		}
		if err := downloadImage(imageURL, fullPath); err != nil {
			return "", err
		}
	}

	if err := updateHDPath(db, record.Date, fullPath); err != nil {
		return "", err
	}
	return fullPath, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
