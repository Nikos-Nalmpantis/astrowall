package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/reflow/wordwrap"
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

type urlOpenedMsg struct {
	target string
	url    string
	err    error
}

type archiveSyncPreparedMsg struct {
	plan archiveSyncPlan
	err  error
}

type archiveItemSyncedMsg struct {
	date      string
	previewed bool
	err       error
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
	showHelp        bool
	showDescription bool
	syncing         bool
	syncItems       []APODResponse
	syncTotal       int
	syncCompleted   int
	syncPreviewed   int
	syncNow         time.Time
	syncContext     context.Context
	cancelSync      context.CancelFunc
	commands        *commandTracker
	activePane      activePane
	spinner         spinner.Model
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

type commandTracker struct {
	mu     sync.Mutex
	closed bool
	wait   sync.WaitGroup
}

func (t *commandTracker) run(cmd tea.Cmd) tea.Msg {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.wait.Add(1)
	t.mu.Unlock()
	defer t.wait.Done()
	return cmd()
}

func (t *commandTracker) closeAndWait() {
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()
	t.wait.Wait()
}

const (
	statusLineCount        = 2
	verticalOuterInset     = 1
	verticalInterPaneGap   = 0
	horizontalOuterInset   = 2
	horizontalInterPaneGap = 1
	detailHeaderLineCount  = 4
)

var selectedAccent = lipgloss.Color("#EE6FF8")

func newListModel(title string, records []APODRecord) list.Model {
	items := make([]list.Item, 0, len(records))
	for _, record := range records {
		items = append(items, apodListItem{record: record})
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = true
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(selectedAccent)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(selectedAccent)

	listModel := list.New(items, delegate, 0, 0)
	listModel.Title = title
	listModel.SetShowHelp(false)
	listModel.SetShowStatusBar(false)
	listModel.SetShowPagination(true)
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
	spin := spinner.New()
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	m := tuiModel{
		recentList:      recentList,
		favoriteList:    favoriteList,
		detail:          detail,
		recentRecords:   recentRecords,
		favoriteRecords: favoriteRecords,
		apiKey:          apiKey,
		status:          "j/k move • tab switch pane • d description • enter set wallpaper • f favorite • o page • u media • ? help • q quit",
		activePane:      recentPane,
		spinner:         spin,
		listStyle:       lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).Padding(0, 1),
		detailStyle:     lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).Padding(0, 1),
		statusStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		helpStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
	}
	m.updatePaneTitles()
	m.refreshDetail(false)
	return m
}

func (m tuiModel) Init() tea.Cmd {
	if m.db == nil {
		return nil
	}
	return tea.Batch(m.prepareArchiveSyncCmd(), spinnerTickCmd(m.spinner))
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.resize()
		return m, nil

	case spinner.TickMsg:
		if !m.loading && !m.syncing {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		if isHelpToggleKey(msg) {
			m.showHelp = !m.showHelp
			return m, nil
		}
		if m.showHelp {
			switch {
			case isHelpCloseKey(msg):
				m.showHelp = false
				return m, nil
			case msg.String() == "q", msg.String() == "ctrl+c":
				m.cancelBackgroundSync()
				return m, tea.Quit
			default:
				return m, nil
			}
		}

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
		if m.descriptionVisible() && isDetailScrollKey(msg) {
			var cmd tea.Cmd
			m.detail, cmd = m.detail.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "q", "ctrl+c":
			m.cancelBackgroundSync()
			return m, tea.Quit
		case "d":
			if m.selectedRecord().PreviewPath == "" {
				m.refreshDetail(true)
				m.status = "No preview available • showing APOD description"
				return m, nil
			}
			m.showDescription = !m.showDescription
			m.refreshDetail(true)
			if m.showDescription {
				m.status = "Showing APOD description • PgUp/PgDown or Ctrl+U/Ctrl+D scroll • d image"
			} else {
				m.status = "Showing APOD image • d description"
			}
			return m, nil
		case "f":
			if m.loading {
				return m, nil
			}
			record := m.selectedRecord()
			if record.Date == "" {
				return m, nil
			}
			m.status = fmt.Sprintf("Toggling favorite for %s…", record.Title)
			return m, m.trackCmd(toggleFavoriteCmd(m.db, record.Date))
		case "o":
			record := m.selectedRecord()
			if record.Date == "" {
				return m, nil
			}
			url := apodPageURL(record.Date)
			if url == "" {
				m.status = "No APOD page URL available for this item"
				return m, nil
			}
			m.status = fmt.Sprintf("Opening APOD page for %s…", record.Title)
			return m, m.trackCmd(openURLCmd("APOD page", url))
		case "u":
			record := m.selectedRecord()
			if record.Date == "" {
				return m, nil
			}
			url := preferredMediaURL(record)
			if url == "" {
				m.status = "No media URL available for this item"
				return m, nil
			}
			m.status = fmt.Sprintf("Opening media URL for %s…", record.Title)
			return m, m.trackCmd(openURLCmd("media URL", url))
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
			return m, tea.Batch(m.trackCmd(applyWallpaperCmd(m.db, m.paths, record, m.apiKey)), spinnerTickCmd(m.spinner))
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

	case urlOpenedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Failed to open %s: %v", msg.target, msg.err)
			return m, nil
		}
		m.status = fmt.Sprintf("Opened %s", msg.target)
		return m, nil

	case archiveSyncPreparedMsg:
		if msg.err != nil {
			m.syncing = false
			m.status = fmt.Sprintf("Library sync failed: %v", msg.err)
			return m, nil
		}
		if msg.plan.AlreadyUpToDate {
			m.syncing = false
			m.status = "Local APOD library is up to date"
			return m, nil
		}
		if len(msg.plan.Items) == 0 {
			m.syncing = false
			m.status = "Library sync returned no APODs"
			return m, nil
		}
		m.syncItems = msg.plan.Items
		m.syncTotal = len(msg.plan.Items)
		m.status = fmt.Sprintf("Syncing APOD 1/%d…", m.syncTotal)
		return m, m.syncArchiveItemCmd(m.syncItems[0])

	case archiveItemSyncedMsg:
		if msg.err != nil {
			m.syncing = false
			m.syncItems = nil
			m.status = fmt.Sprintf("Library sync failed for %s: %v", msg.date, msg.err)
			return m, nil
		}
		m.syncCompleted++
		if msg.previewed {
			m.syncPreviewed++
		}
		if err := m.reloadRecords(); err != nil {
			m.syncing = false
			m.syncItems = nil
			m.status = fmt.Sprintf("Library refresh failed: %v", err)
			return m, nil
		}
		m.syncItems = m.syncItems[1:]
		if len(m.syncItems) == 0 {
			m.syncing = false
			m.status = fmt.Sprintf("Synced %d APODs and cached %d previews", m.syncCompleted, m.syncPreviewed)
			return m, nil
		}
		m.status = fmt.Sprintf("Syncing APOD %d/%d…", m.syncCompleted+1, m.syncTotal)
		return m, m.syncArchiveItemCmd(m.syncItems[0])

	case favoriteToggledMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Favorite update failed: %v", msg.err)
			return m, nil
		}
		selectedDate := m.selectedRecord().Date
		previousPane := m.activePane
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
		if m.ready {
			m.resize()
		} else {
			m.refreshDetail(false)
		}
		if m.activePane != previousPane || m.selectedRecord().Date != selectedDate {
			m.detail.GotoTop()
		}
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

	detailView := m.detail.View()
	if m.showHelp {
		help := m.detail
		help.SetContent(wordwrap.String(m.renderHelpView(), max(1, help.Width())))
		help.GotoTop()
		detailView = help.View()
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		strings.Repeat(" ", horizontalOuterInset),
		leftColumn,
		strings.Repeat(" ", horizontalInterPaneGap),
		m.detailStyle.Render(detailView),
		strings.Repeat(" ", horizontalOuterInset),
	)

	status := m.status
	if m.loading || m.syncing {
		status = fmt.Sprintf("%s %s", m.spinner.View(), status)
	}
	if m.showHelp {
		status = "Help open — press ? or Esc to close"
	}
	lineWidth := max(1, m.width-horizontalOuterInset*2)
	status = ansi.Truncate(status, lineWidth, "")
	helpLine := ansi.Truncate(fmt.Sprintf("Active pane: %s • Tab/Shift+Tab panes • j/k move • d %s • f favorite • o page • u media • enter wallpaper • ? help • q quit", m.activePaneLabel(), m.detailToggleLabel()), lineWidth, "")
	textInset := strings.Repeat(" ", horizontalOuterInset)

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		layoutSpacer(verticalOuterInset),
		panes,
		textInset+m.statusStyle.Render(status),
		textInset+m.helpStyle.Render(helpLine),
		layoutSpacer(verticalOuterInset),
	)

	view := tea.NewView(body)
	view.AltScreen = true
	return view
}

func layoutSpacer(height int) string {
	if height <= 0 {
		return ""
	}
	return strings.Repeat("\n", height-1) + " "
}

func (m *tuiModel) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	listHorizontalFrame, listVerticalFrame := m.listStyle.GetFrameSize()
	detailHorizontalFrame, detailVerticalFrame := m.detailStyle.GetFrameSize()

	contentHeight := max(1, m.height-statusLineCount-verticalOuterInset*2-verticalInterPaneGap*2)
	availableWidth := max(2, m.width-horizontalOuterInset*2-horizontalInterPaneGap)
	leftOuterWidth := availableWidth / 3
	if m.width >= 40 {
		leftOuterWidth = max(20, leftOuterWidth)
	} else {
		leftOuterWidth = availableWidth / 2
	}
	rightOuterWidth := max(1, availableWidth-leftOuterWidth)
	// The list view reserves one cursor column beyond its configured width.
	listInnerWidth := max(1, leftOuterWidth-listHorizontalFrame-1)
	detailInnerWidth := max(1, rightOuterWidth-detailHorizontalFrame)
	leftInnerHeight := max(2, contentHeight-listVerticalFrame*2-verticalInterPaneGap)
	recentInnerHeight := (leftInnerHeight + 1) / 2
	favoriteInnerHeight := leftInnerHeight / 2
	detailInnerHeight := max(1, contentHeight-detailVerticalFrame)

	m.recentList.SetSize(listInnerWidth, recentInnerHeight)
	m.favoriteList.SetSize(listInnerWidth, favoriteInnerHeight)
	m.detail.SetWidth(detailInnerWidth)
	m.detail.SetHeight(detailInnerHeight)

	previewHeight := max(6, detailInnerHeight-detailHeaderLineCount)
	m.previewArea = imageArea{width: max(12, detailInnerWidth), height: previewHeight}
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
	if m.descriptionVisible() {
		parts = append(parts, "", "Description", "", strings.TrimSpace(record.Description))
	} else if record.PreviewPath != "" {
		if preview, err := renderPreviewBlock(record.PreviewPath, m.previewArea.width, m.previewArea.height); err == nil && preview != "" {
			parts = append(parts, preview)
		} else {
			parts = append(parts, fmt.Sprintf("Preview cache: %s", record.PreviewPath))
			parts = append(parts, "Preview could not be rendered in this terminal session.")
		}
	}
	wrappedContent := wordwrap.String(strings.Join(parts, "\n"), max(20, m.detail.Width()))

	m.detail.SetContent(wrappedContent)
	if resetScroll {
		m.detail.GotoTop()
	}
}

func (m tuiModel) descriptionVisible() bool {
	return m.showDescription || m.selectedRecord().PreviewPath == ""
}

func (m tuiModel) detailToggleLabel() string {
	if m.selectedRecord().PreviewPath == "" {
		return "description"
	}
	if m.showDescription {
		return "image"
	}
	return "description"
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

func runTUI(db *sql.DB, paths AppPaths, apiKey string) error {
	model, err := newTUIModelFromLibrary(db, paths, apiKey, time.Now())
	if err != nil {
		return err
	}
	program := tea.NewProgram(model)
	_, err = program.Run()
	model.cancelBackgroundSync()
	model.commands.closeAndWait()
	return err
}

func newTUIModelFromLibrary(db *sql.DB, paths AppPaths, apiKey string, now time.Time) (tuiModel, error) {
	recentRecords, err := listRecentAPODs(db, 30)
	if err != nil {
		return tuiModel{}, err
	}
	favoriteRecords, err := listFavoriteAPODs(db)
	if err != nil {
		return tuiModel{}, err
	}
	model := newTUIModel(recentRecords, favoriteRecords, apiKey)
	model.db = db
	model.paths = paths
	model.syncing = true
	model.syncNow = now
	model.syncContext, model.cancelSync = context.WithCancel(context.Background())
	model.commands = &commandTracker{}
	model.status = "Checking for new APODs…"
	return model, nil
}

func (m tuiModel) prepareArchiveSyncCmd() tea.Cmd {
	return m.trackCmd(func() tea.Msg {
		plan, err := prepareArchiveSyncContext(m.syncContext, m.db, m.apiKey, m.syncNow)
		return archiveSyncPreparedMsg{plan: plan, err: err}
	})
}

func (m tuiModel) syncArchiveItemCmd(item APODResponse) tea.Cmd {
	return m.trackCmd(func() tea.Msg {
		previewed, err := syncAPODItemContext(m.syncContext, m.db, m.paths, item, m.syncNow)
		return archiveItemSyncedMsg{date: item.Date, previewed: previewed, err: err}
	})
}

func (m tuiModel) trackCmd(cmd tea.Cmd) tea.Cmd {
	if cmd == nil || m.commands == nil {
		return cmd
	}
	return func() tea.Msg {
		return m.commands.run(cmd)
	}
}

func (m tuiModel) cancelBackgroundSync() {
	if m.cancelSync != nil {
		m.cancelSync()
	}
}

func (m *tuiModel) reloadRecords() error {
	recentDate := selectedListDate(m.recentList)
	favoriteDate := selectedListDate(m.favoriteList)

	recent, err := listRecentAPODs(m.db, 30)
	if err != nil {
		return err
	}
	favorites, err := listFavoriteAPODs(m.db)
	if err != nil {
		return err
	}
	m.recentRecords = recent
	m.favoriteRecords = favorites
	m.syncListItems()
	selectListDate(&m.recentList, recentDate)
	selectListDate(&m.favoriteList, favoriteDate)
	if m.ready {
		m.resize()
	} else {
		m.refreshDetail(false)
	}
	return nil
}

func selectedListDate(listModel list.Model) string {
	item, ok := listModel.SelectedItem().(apodListItem)
	if !ok {
		return ""
	}
	return item.record.Date
}

func selectListDate(listModel *list.Model, date string) {
	if date == "" {
		return
	}
	for i, item := range listModel.Items() {
		record, ok := item.(apodListItem)
		if ok && record.record.Date == date {
			listModel.Select(i)
			return
		}
	}
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
		style = style.BorderForeground(selectedAccent).Bold(true)
	}
	horizontalFrame, verticalFrame := style.GetFrameSize()
	outerWidth := listModel.Width() + horizontalFrame + 1
	style = style.
		Width(outerWidth).
		MaxWidth(outerWidth)
	rendered := style.Render(limitBlock(listModel.View(), listModel.Width()+1, listModel.Height()))
	return limitRenderedPane(rendered, listModel.Height()+verticalFrame)
}

func limitBlock(content string, width, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	return strings.Join(lines, "\n")
}

func limitRenderedPane(content string, height int) string {
	lines := strings.Split(content, "\n")
	if height <= 0 || len(lines) <= height {
		return content
	}
	lines[height-1] = lines[len(lines)-1]
	return strings.Join(lines[:height], "\n")
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

func isDetailScrollKey(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		return true
	default:
		return false
	}
}

func isHelpCloseKey(msg tea.KeyPressMsg) bool {
	key := msg.Key()
	if key.Code == tea.KeyEscape || key.Code == tea.KeyEsc {
		return true
	}
	s := msg.String()
	return s == "esc" || s == "escape" || s == "ctrl+["
}

func isHelpToggleKey(msg tea.KeyPressMsg) bool {
	if msg.String() == "?" || msg.String() == "/" {
		return true
	}
	key := msg.Key()
	if key.Text == "?" || key.Text == "/" {
		return true
	}
	if key.Code == '/' || key.ShiftedCode == '?' {
		return true
	}
	return false
}

func spinnerTickCmd(spin spinner.Model) tea.Cmd {
	return func() tea.Msg {
		return spin.Tick()
	}
}

func (m tuiModel) renderHelpView() string {
	return strings.Join([]string{
		"Keybindings",
		"",
		"Navigation",
		"  j / k              Move within the active pane",
		"  Tab                Switch to the next pane",
		"  Shift+Tab          Switch to the previous pane",
		"",
		"Actions",
		"  Enter              Download/apply wallpaper for selected APOD",
		"  d                  Toggle image and description views",
		"  PgUp / PgDown      Scroll the description view",
		"  Ctrl+U / Ctrl+D    Scroll half a description page",
		"  f                  Favorite or unfavorite the selected APOD",
		"  o                  Open the APOD page in your browser",
		"  u                  Open the selected media URL",
		"  ?                  Toggle this help view",
		"  Esc                Close the help view",
		"  q / Ctrl+C         Quit",
		"",
		"Notes",
		"  - The active pane title includes • active",
		"  - Favorites persist in the local SQLite library",
		"  - --cycle-favorites rotates favorites outside the TUI",
	}, "\n")
}

func openURLCmd(target, url string) tea.Cmd {
	return func() tea.Msg {
		err := openURLFunc(url)
		return urlOpenedMsg{target: target, url: url, err: err}
	}
}
