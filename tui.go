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
	title    string
	favorite bool
	err      error
}

type activePane int

const (
	recentPane activePane = iota
	favoritesPane
)

type tuiModel struct {
	db              *sql.DB
	paths           AppPaths
	recentList      list.Model
	favoriteList    list.Model
	detail          viewport.Model
	recentRecords   []APODRecord
	favoriteRecords []APODRecord
	apiKey          string
	status          string
	width           int
	height          int
	ready           bool
	loading         bool
	activePane      activePane
	listStyle       lipgloss.Style
	detailStyle     lipgloss.Style
	statusStyle     lipgloss.Style
	helpStyle       lipgloss.Style
	previewArea     imageArea
}

type imageArea struct {
	width  int
	height int
}

func newListModel(title string, records []APODRecord) list.Model {
	items := make([]list.Item, 0, len(records))
	for _, record := range records {
		items = append(items, apodListItem{record: record})
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = true

	listModel := list.New(items, delegate, 0, 0)
	listModel.Title = title
	listModel.SetShowHelp(false)
	listModel.SetShowStatusBar(false)
	listModel.SetShowPagination(false)
	listModel.SetShowFilter(false)
	listModel.SetFilteringEnabled(false)
	listModel.DisableQuitKeybindings()
	return listModel
}

func newTUIModel(recentRecords, favoriteRecords []APODRecord, apiKey string) tuiModel {
	recentList := newListModel("Recent APODs", recentRecords)
	favoriteList := newListModel("Favorites", favoriteRecords)
	detail := viewport.New()
	detail.SetContent("No APODs loaded.")

	m := tuiModel{
		recentList:      recentList,
		favoriteList:    favoriteList,
		detail:          detail,
		recentRecords:   recentRecords,
		favoriteRecords: favoriteRecords,
		apiKey:          apiKey,
		status:          "j/k move • tab switch pane • enter set wallpaper • f favorite • q quit",
		activePane:      recentPane,
		listStyle:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
		detailStyle:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
		statusStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		helpStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
	}
	m.updatePaneTitles()
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
		if isNextPaneKey(msg) {
			m.activePane = m.nextPane(false)
			m.updatePaneTitles()
			m.refreshDetail(true)
			return m, nil
		}
		if isPreviousPaneKey(msg) {
			m.activePane = m.nextPane(true)
			m.updatePaneTitles()
			m.refreshDetail(true)
			return m, nil
		}

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
		m.updateHDPathInRecords(msg.date, msg.path)
		m.syncListItems()
		m.refreshDetail(false)
		m.status = fmt.Sprintf("Wallpaper set to %s", msg.title)
		return m, nil

	case favoriteToggledMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Favorite update failed: %v", msg.err)
			return m, nil
		}
		m.updateFavoriteInRecent(msg.date, msg.favorite)
		favorites, err := listFavoriteAPODs(m.db)
		if err != nil {
			m.status = fmt.Sprintf("Favorite refresh failed: %v", err)
			return m, nil
		}
		m.favoriteRecords = favorites
		if m.activePane == favoritesPane && len(m.favoriteRecords) == 0 {
			m.activePane = recentPane
			m.updatePaneTitles()
		}
		m.syncListItems()
		m.refreshDetail(false)
		if msg.favorite {
			m.status = fmt.Sprintf("Added %s to favorites", msg.title)
		} else {
			m.status = fmt.Sprintf("Removed %s from favorites", msg.title)
		}
		return m, nil
	}

	before := m.activeList().Index()
	updatedList, listCmd := m.activeList().Update(msg)
	m.setActiveList(updatedList)
	if m.activeList().Index() != before {
		m.refreshDetail(true)
	}

	return m, listCmd
}

func (m tuiModel) View() tea.View {
	if !m.ready {
		view := tea.NewView("Loading astrowall TUI…")
		view.AltScreen = true
		return view
	}

	leftColumn := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderListPane(recentPane, m.recentList),
		m.renderListPane(favoritesPane, m.favoriteList),
	)

	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		leftColumn,
		m.detailStyle.Render(m.detail.View()),
	)

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		panes,
		m.statusStyle.Render(m.status),
		m.helpStyle.Render(fmt.Sprintf("Active pane: %s • Tab/Shift+Tab switch panes • j/k navigate • f favorite • enter set wallpaper • q quit", m.activePaneLabel())),
	)

	view := tea.NewView(body)
	view.AltScreen = true
	return view
}

func (m *tuiModel) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	contentHeight := max(10, m.height-4)
	leftWidth := max(30, m.width/3)
	rightWidth := max(40, m.width-leftWidth-6)
	recentHeight := max(4, contentHeight/2)
	favoriteHeight := max(4, contentHeight-recentHeight)

	m.recentList.SetSize(leftWidth, recentHeight)
	m.favoriteList.SetSize(leftWidth, favoriteHeight)
	m.detail.SetWidth(rightWidth)
	m.detail.SetHeight(contentHeight)
	m.previewArea = imageArea{width: max(12, rightWidth-4), height: max(6, contentHeight/2)}
	m.refreshDetail(false)
}

func (m *tuiModel) refreshDetail(resetScroll bool) {
	record := m.selectedRecord()
	if record.Date == "" {
		m.detail.SetContent("No APOD records are available for the active pane.")
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
	item := m.activeList().SelectedItem()
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
	recentRecords, err := listRecentAPODs(db, 30)
	if err != nil {
		return err
	}
	favoriteRecords, err := listFavoriteAPODs(db)
	if err != nil {
		return err
	}
	paths, err := resolveAppPaths()
	if err != nil {
		return err
	}

	model := newTUIModel(recentRecords, favoriteRecords, apiKey)
	model.db = db
	model.paths = paths
	program := tea.NewProgram(model)
	_, err = program.Run()
	return err
}

func (m *tuiModel) syncListItems() {
	m.syncSingleList(&m.recentList, m.recentRecords)
	m.syncSingleList(&m.favoriteList, m.favoriteRecords)
	m.updatePaneTitles()
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
		record, err := recordByDate(db, date)
		if err != nil {
			return favoriteToggledMsg{date: date, err: err}
		}
		favorite, err := toggleFavorite(db, date)
		return favoriteToggledMsg{date: date, title: record.Title, favorite: favorite, err: err}
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

func (m *tuiModel) syncSingleList(target *list.Model, records []APODRecord) {
	items := make([]list.Item, 0, len(records))
	for _, record := range records {
		items = append(items, apodListItem{record: record})
	}
	selected := target.Index()
	target.SetItems(items)
	if len(items) == 0 {
		target.Select(0)
		return
	}
	if selected >= len(items) {
		selected = len(items) - 1
	}
	target.Select(selected)
}

func (m tuiModel) activeList() list.Model {
	if m.activePane == favoritesPane {
		return m.favoriteList
	}
	return m.recentList
}

func (m *tuiModel) setActiveList(updated list.Model) {
	if m.activePane == favoritesPane {
		m.favoriteList = updated
		return
	}
	m.recentList = updated
}

func (m tuiModel) nextPane(reverse bool) activePane {
	if reverse {
		if m.activePane == recentPane {
			if len(m.favoriteRecords) > 0 {
				return favoritesPane
			}
			return recentPane
		}
		return recentPane
	}

	if m.activePane == recentPane && len(m.favoriteRecords) > 0 {
		return favoritesPane
	}
	return recentPane
}

func (m tuiModel) renderListPane(pane activePane, listModel list.Model) string {
	style := m.listStyle
	if m.activePane == pane {
		style = style.BorderForeground(lipgloss.Color("39")).Bold(true)
	}
	return style.Render(listModel.View())
}

func (m *tuiModel) updatePaneTitles() {
	if m.activePane == recentPane {
		m.recentList.Title = "Recent APODs • active"
		m.favoriteList.Title = "Favorites"
		return
	}
	m.recentList.Title = "Recent APODs"
	m.favoriteList.Title = "Favorites • active"
}

func (m tuiModel) activePaneLabel() string {
	if m.activePane == favoritesPane {
		return "Favorites"
	}
	return "Recent APODs"
}

func (m *tuiModel) updateFavoriteInRecent(date string, favorite bool) {
	for i := range m.recentRecords {
		if m.recentRecords[i].Date == date {
			m.recentRecords[i].Favorite = favorite
		}
	}
}

func (m *tuiModel) updateHDPathInRecords(date, path string) {
	for i := range m.recentRecords {
		if m.recentRecords[i].Date == date {
			m.recentRecords[i].HDPath = path
		}
	}
	for i := range m.favoriteRecords {
		if m.favoriteRecords[i].Date == date {
			m.favoriteRecords[i].HDPath = path
		}
	}
}

func isNextPaneKey(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	if key.Code == tea.KeyTab && key.Mod == 0 {
		return true
	}
	return msg.String() == "tab" || msg.String() == "ctrl+i"
}

func isPreviousPaneKey(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	if key.Code == tea.KeyTab && key.Mod == tea.ModShift {
		return true
	}
	return msg.String() == "shift+tab"
}
