package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
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
	return strings.Join([]string{i.record.Title, i.record.Date, i.record.Description}, " ")
}

func (i apodListItem) Title() string {
	return i.record.Title
}

func (i apodListItem) Description() string {
	description := i.record.Date
	if i.record.PreviewError != "" {
		description += " • preview error"
	}
	if i.record.Favorite {
		description += " ★"
	}
	return description
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
	date         string
	previewed    bool
	previewError string
	err          error
}

type nativeImagePreparedMsg struct {
	image nativeImage
	key   string
	err   error
}

type nativeImageActivatedMsg struct {
	image nativeImage
	key   string
}

type apiKeySavedMsg struct {
	apiKey string
	err    error
}

type apiKeyRemovedMsg struct {
	err error
}

type savedAPIKeyLoadedMsg struct {
	apiKey string
	err    error
}

type activePane int

const (
	recentPane activePane = iota
	favoritesPane
)

type tuiModel struct {
	db               *sql.DB
	paths            AppPaths
	recentList       list.Model
	favoriteList     list.Model
	detail           viewport.Model
	recentRecords    []APODRecord
	favoriteRecords  []APODRecord
	apiKey           string
	apiKeySource     apiKeySource
	apiKeyInput      textinput.Model
	showAPIKeyInput  bool
	savingAPIKey     bool
	lookupCredential bool
	syncStarted      bool
	pendingSavedKey  savedAPIKeyLoadedMsg
	hasPendingKey    bool
	status           string
	syncStatus       string
	libraryCount     int
	width            int
	height           int
	ready            bool
	loading          bool
	showHelp         bool
	showDescription  bool
	imageProtocol    imageProtocol
	nativeImage      nativeImage
	nativeRequest    string
	nativeTarget     string
	nativeGeneration uint64
	nativePending    map[uint32]struct{}
	nativeKnown      map[uint32]struct{}
	nativeContext    context.Context
	cancelNative     context.CancelFunc
	tmux             bool
	syncing          bool
	syncItems        []APODResponse
	syncTotal        int
	syncCompleted    int
	syncPreviewed    int
	syncFailed       int
	syncNow          time.Time
	syncContext      context.Context
	cancelSync       context.CancelFunc
	commands         *commandTracker
	nativeOutput     *nativeImageOutput
	activePane       activePane
	searchOriginal   [2]string
	spinner          spinner.Model
	listStyle        lipgloss.Style
	detailStyle      lipgloss.Style
	statusStyle      lipgloss.Style
	helpStyle        lipgloss.Style
	previewArea      imageArea
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
	headerLineCount        = 1
	statusLineCount        = 3
	verticalOuterInset     = 1
	verticalInterPaneGap   = 0
	horizontalOuterInset   = 2
	horizontalInterPaneGap = 1
)

func newListModel(title string, records []APODRecord) list.Model {
	items := make([]list.Item, 0, len(records))
	for _, record := range records {
		items = append(items, apodListItem{record: record})
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = true
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.Foreground(colorStarlight)
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.Foreground(colorMuted)
	delegate.Styles.DimmedTitle = delegate.Styles.DimmedTitle.Foreground(colorMuted)
	delegate.Styles.DimmedDesc = delegate.Styles.DimmedDesc.Foreground(colorBorder)
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(colorStarlight).BorderForeground(selectedAccent).Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(colorViolet).BorderForeground(selectedAccent)
	delegate.Styles.FilterMatch = delegate.Styles.FilterMatch.Foreground(colorCyan)

	listModel := list.New(items, delegate, 0, 0)
	listModel.Title = title
	listModel.SetShowHelp(false)
	listModel.SetShowStatusBar(false)
	listModel.SetShowPagination(true)
	listModel.SetShowFilter(true)
	listModel.SetFilteringEnabled(true)
	listModel.Styles.Title = listModel.Styles.Title.Background(colorPanel).Foreground(colorCyan).Bold(true)
	listModel.Styles.Filter.Focused.Prompt = listModel.Styles.Filter.Focused.Prompt.Foreground(colorCyan)
	listModel.Styles.Filter.Blurred.Prompt = listModel.Styles.Filter.Blurred.Prompt.Foreground(colorCyan)
	listModel.Styles.Filter.Cursor.Color = colorViolet
	listModel.Styles.ActivePaginationDot = listModel.Styles.ActivePaginationDot.Foreground(colorViolet)
	listModel.Styles.InactivePaginationDot = listModel.Styles.InactivePaginationDot.Foreground(colorBorder)
	listModel.KeyMap.AcceptWhileFiltering.SetKeys("enter")
	listModel.DisableQuitKeybindings()
	return listModel
}

func newTUIModel(recentRecords, favoriteRecords []APODRecord, apiKey string) tuiModel {
	recentList := newListModel("Recent APODs", recentRecords)
	favoriteList := newListModel("Favorites", favoriteRecords)
	detail := viewport.New()
	detail.SetContent("No APODs loaded.")
	spin := spinner.New()
	spin.Style = accentText
	keyInput := textinput.New()
	keyInput.Prompt = "NASA API key: "
	keyInput.Placeholder = "Paste your key"
	keyInput.EchoMode = textinput.EchoPassword
	keyInput.EchoCharacter = '•'
	keyInput.CharLimit = 256

	m := tuiModel{
		recentList:      recentList,
		favoriteList:    favoriteList,
		detail:          detail,
		recentRecords:   recentRecords,
		favoriteRecords: favoriteRecords,
		apiKey:          apiKey,
		apiKeySource:    apiKeySourceDemo,
		apiKeyInput:     keyInput,
		status:          "Select an APOD to explore or set as wallpaper",
		libraryCount:    len(recentRecords),
		activePane:      recentPane,
		spinner:         spin,
		nativeOutput:    newNativeImageOutput(io.Discard),
		listStyle:       lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(colorBorder).Padding(0, 1),
		detailStyle:     lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(colorCyan).Padding(0, 1),
		statusStyle:     lipgloss.NewStyle(),
		helpStyle:       secondaryText,
	}
	m.updatePaneTitles()
	m.refreshDetail(false)
	return m
}

func (m tuiModel) Init() tea.Cmd {
	if m.db == nil {
		return nil
	}
	if m.lookupCredential {
		return tea.Batch(loadSavedAPIKeyCmd(), spinnerTickCmd(m.spinner))
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
		return m, m.requestNativeImage()

	case spinner.TickMsg:
		if !m.loading && !m.syncing {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		if m.showAPIKeyInput {
			switch msg.String() {
			case "ctrl+c":
				m.cancelBackgroundSync()
				return m, tea.Sequence(m.clearNativeImage(), tea.Quit)
			case "esc", "escape", "ctrl+[":
				if m.savingAPIKey {
					return m, nil
				}
				m.closeAPIKeyInput()
				m.status = "NASA API key update cancelled"
				return m, m.requestNativeImage()
			case "enter":
				if m.savingAPIKey {
					return m, nil
				}
				apiKey := strings.TrimSpace(m.apiKeyInput.Value())
				if apiKey == "" {
					m.status = "NASA API key cannot be empty"
					return m, nil
				}
				m.savingAPIKey = true
				m.lookupCredential = false
				m.status = "Validating and saving NASA API key…"
				return m, saveAPIKeyCmd(apiKey)
			case "ctrl+r":
				if m.savingAPIKey {
					return m, nil
				}
				m.savingAPIKey = true
				m.lookupCredential = false
				m.status = "Removing saved NASA API key…"
				return m, removeAPIKeyCmd()
			default:
				var cmd tea.Cmd
				m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
				return m, cmd
			}
		}
		if m.showHelp {
			switch {
			case isHelpToggleKey(msg), isHelpCloseKey(msg):
				m.showHelp = false
				return m, m.requestNativeImage()
			case msg.String() == "q", msg.String() == "ctrl+c":
				m.cancelBackgroundSync()
				return m, tea.Sequence(m.clearNativeImage(), tea.Quit)
			default:
				return m, nil
			}
		}
		if m.activeList().SettingFilter() {
			if msg.String() == "ctrl+c" {
				m.cancelBackgroundSync()
				return m, tea.Sequence(m.clearNativeImage(), tea.Quit)
			}
			if isHelpCloseKey(msg) {
				return m, m.cancelSearch()
			}
			if msg.String() == "enter" {
				activeList := m.activeList()
				if strings.TrimSpace(activeList.FilterValue()) == "" {
					activeList.ResetFilter()
				} else {
					activeList.SetFilterText(activeList.FilterValue())
					activeList.SetFilterState(list.FilterApplied)
					activeList.FilterInput.Blur()
				}
				m.setActiveList(activeList)
				m.searchOriginal[m.activePane] = ""
				m.updatePaneTitles()
				m.refreshDetail(true)
				return m, m.requestNativeImage()
			}
			return m, m.updateSearch(msg)
		}
		if msg.String() == "/" {
			return m, tea.Batch(m.startSearch(), m.clearNativeImage())
		}
		if isHelpCloseKey(msg) && m.activeList().IsFiltered() {
			return m, m.clearSearch()
		}
		if isHelpToggleKey(msg) {
			m.showHelp = true
			return m, m.clearNativeImage()
		}

		if isNextPaneKey(msg) {
			m.activePane = m.nextPane(false)
			m.updatePaneTitles()
			m.refreshDetail(true)
			return m, m.requestNativeImage()
		}
		if isPreviousPaneKey(msg) {
			m.activePane = m.nextPane(true)
			m.updatePaneTitles()
			m.refreshDetail(true)
			return m, m.requestNativeImage()
		}
		if m.descriptionVisible() && isDetailScrollKey(msg) {
			var cmd tea.Cmd
			m.detail, cmd = m.detail.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "q", "ctrl+c":
			m.cancelBackgroundSync()
			return m, tea.Sequence(m.clearNativeImage(), tea.Quit)
		case "a":
			m.showAPIKeyInput = true
			m.apiKeyInput.Reset()
			m.status = fmt.Sprintf("NASA API key source: %s", m.apiKeySource)
			return m, tea.Batch(m.apiKeyInput.Focus(), m.clearNativeImage())
		case "d":
			if m.selectedRecord().PreviewPath == "" {
				m.refreshDetail(true)
				m.status = "No preview available • showing APOD description"
				return m, m.clearNativeImage()
			}
			m.showDescription = !m.showDescription
			m.refreshDetail(true)
			if m.showDescription {
				m.status = "Showing APOD description • PgUp/PgDown or Ctrl+U/Ctrl+D scroll • d image"
				return m, m.clearNativeImage()
			} else {
				m.status = "Showing APOD image • d description"
				return m, m.requestNativeImage()
			}
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
		m.syncStarted = true
		if msg.err != nil {
			m.syncing = false
			m.syncStatus = fmt.Sprintf("Library sync failed: %v", msg.err)
			return m, nil
		}
		if msg.plan.AlreadyUpToDate {
			m.syncing = false
			m.syncStatus = "Local APOD library is up to date"
			return m, nil
		}
		if len(msg.plan.Items) == 0 {
			m.syncing = false
			m.syncStatus = "Library sync returned no APODs"
			return m, nil
		}
		m.syncItems = msg.plan.Items
		m.syncTotal = len(msg.plan.Items)
		m.syncStatus = fmt.Sprintf("Syncing APOD 1/%d…", m.syncTotal)
		return m, m.syncArchiveItemCmd(m.syncItems[0])

	case archiveItemSyncedMsg:
		if msg.err != nil {
			m.syncing = false
			m.syncItems = nil
			m.syncStatus = fmt.Sprintf("Library sync failed for %s: %v", msg.date, msg.err)
			return m, m.requestNativeImage()
		}
		m.syncCompleted++
		if msg.previewed {
			m.syncPreviewed++
		}
		if msg.previewError != "" {
			m.syncFailed++
		}
		if err := m.reloadRecords(); err != nil {
			m.syncing = false
			m.syncItems = nil
			m.syncStatus = fmt.Sprintf("Library refresh failed: %v", err)
			return m, nil
		}
		m.syncItems = m.syncItems[1:]
		if len(m.syncItems) == 0 {
			m.syncing = false
			m.syncStatus = fmt.Sprintf("Synced %d APODs, cached %d previews, %d preview errors", m.syncCompleted, m.syncPreviewed, m.syncFailed)
			return m, m.requestNativeImage()
		}
		m.syncStatus = fmt.Sprintf("Syncing APOD %d/%d…", m.syncCompleted+1, m.syncTotal)
		return m, tea.Batch(m.syncArchiveItemCmd(m.syncItems[0]), m.requestNativeImage())

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
		return m, m.requestNativeImage()

	case nativeImagePreparedMsg:
		if msg.key != m.nativeRequest || msg.err != nil || !m.nativeImageWanted() {
			if msg.err != nil && msg.key == m.nativeRequest {
				m.nativeRequest = ""
				m.nativeTarget = ""
				m.status = fmt.Sprintf("Native preview unavailable; using ANSI: %v", msg.err)
			}
			return m, nil
		}
		if m.nativePending == nil {
			m.nativePending = make(map[uint32]struct{})
		}
		if msg.image.id != 0 {
			m.nativePending[msg.image.id] = struct{}{}
			m.nativeKnown[msg.image.id] = struct{}{}
		}
		if msg.image.protocol == imageProtocolWezTerm {
			m.nativeImage = msg.image
			m.refreshDetail(false)
			m.nativeOutput.setPlacement(msg.image.placement)
			delete(m.nativePending, msg.image.id)
			return m, tea.Raw(msg.image.transmission)
		}
		return m, tea.Sequence(
			tea.Raw(msg.image.transmission),
			func() tea.Msg { return nativeImageActivatedMsg{image: msg.image, key: msg.key} },
		)

	case nativeImageActivatedMsg:
		delete(m.nativePending, msg.image.id)
		if msg.key != m.nativeRequest || !m.nativeImageWanted() {
			return m, tea.Raw(kittyDeleteImage(msg.image.id, m.tmux))
		}
		m.nativeImage = msg.image
		m.refreshDetail(false)
		return m, nil

	case apiKeySavedMsg:
		m.savingAPIKey = false
		m.lookupCredential = false
		if msg.err != nil {
			m.applyPendingSavedKey()
			m.status = fmt.Sprintf("NASA API key was not saved: %v", msg.err)
			return m, m.startArchiveSync()
		}
		m.hasPendingKey = false
		if m.apiKeySource != apiKeySourceFlag && m.apiKeySource != apiKeySourceEnv {
			m.apiKey = msg.apiKey
			m.apiKeySource = apiKeySourceCredential
		}
		m.closeAPIKeyInput()
		m.status = fmt.Sprintf("NASA API key validated and saved securely • current source: %s", m.apiKeySource)
		return m, tea.Batch(m.startArchiveSync(), m.requestNativeImage())

	case apiKeyRemovedMsg:
		m.savingAPIKey = false
		m.lookupCredential = false
		if msg.err != nil {
			m.applyPendingSavedKey()
			m.status = fmt.Sprintf("Saved NASA API key was not removed: %v", msg.err)
			return m, m.startArchiveSync()
		}
		m.hasPendingKey = false
		if m.apiKeySource == apiKeySourceCredential {
			m.apiKey = "DEMO_KEY"
			m.apiKeySource = apiKeySourceDemo
		}
		m.closeAPIKeyInput()
		m.status = fmt.Sprintf("Saved NASA API key removed • current source: %s", m.apiKeySource)
		return m, tea.Batch(m.startArchiveSync(), m.requestNativeImage())

	case savedAPIKeyLoadedMsg:
		if !m.lookupCredential {
			if m.savingAPIKey {
				m.pendingSavedKey = msg
				m.hasPendingKey = true
				return m, nil
			}
			return m, m.startArchiveSync()
		}
		m.lookupCredential = false
		if msg.err == nil && msg.apiKey != "" {
			m.apiKey = msg.apiKey
			m.apiKeySource = apiKeySourceCredential
			m.syncStatus = "Saved NASA API key loaded • checking for new APODs…"
		} else if msg.err != nil {
			m.status = fmt.Sprintf("Saved key unavailable; using DEMO_KEY: %v", msg.err)
		} else {
			m.syncStatus = "Using DEMO_KEY • checking for new APODs…"
		}
		return m, m.startArchiveSync()

	}
	if m.showAPIKeyInput {
		var cmd tea.Cmd
		m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
		return m, cmd
	}
	if m.activeList().SettingFilter() {
		return m, m.updateSearch(msg)
	}

	before := m.activeList().Index()
	updatedList, listCmd := m.activeList().Update(msg)
	m.setActiveList(updatedList)
	if m.activeList().Index() != before {
		m.refreshDetail(true)
		return m, tea.Batch(listCmd, m.requestNativeImage())
	}

	return m, listCmd
}

func (m *tuiModel) applyPendingSavedKey() {
	if !m.hasPendingKey {
		return
	}
	if m.pendingSavedKey.err == nil && m.pendingSavedKey.apiKey != "" && m.apiKeySource != apiKeySourceFlag && m.apiKeySource != apiKeySourceEnv {
		m.apiKey = m.pendingSavedKey.apiKey
		m.apiKeySource = apiKeySourceCredential
	}
	m.hasPendingKey = false
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
	if m.showAPIKeyInput {
		detailView = m.renderAPIKeyInput()
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		strings.Repeat(" ", horizontalOuterInset),
		leftColumn,
		strings.Repeat(" ", horizontalInterPaneGap),
		m.detailStyle.Render(detailView),
		strings.Repeat(" ", horizontalOuterInset),
	)

	status := m.status
	if m.loading {
		status = fmt.Sprintf("%s %s", m.spinner.View(), status)
	}
	if m.showHelp {
		status = "Help open — press ? or Esc to close"
	} else if m.showAPIKeyInput && !m.savingAPIKey {
		status = "Enter validate/save • Ctrl+R remove saved key • Esc cancel"
	}
	lineWidth := max(1, m.width-horizontalOuterInset*2)
	syncStatus := m.syncStatus
	if syncStatus == "" {
		syncStatus = "Library ready"
	}
	if m.syncing {
		syncStatus = fmt.Sprintf("%s %s", m.spinner.View(), syncStatus)
	}
	syncStatus = ansi.Truncate(syncStatus, lineWidth, "")
	status = ansi.Truncate(status, lineWidth, "")
	helpLine := shortcutHints(lineWidth, m.activePaneLabel(), m.detailToggleLabel())
	textInset := strings.Repeat(" ", horizontalOuterInset)

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		textInset+m.renderDashboardHeader(lineWidth),
		layoutSpacer(verticalOuterInset),
		panes,
		textInset+styleStatus(syncStatus),
		textInset+m.statusStyle.Render(styleStatus(status)),
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

	contentHeight := max(1, m.height-headerLineCount-statusLineCount-verticalOuterInset*2-verticalInterPaneGap*2)
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
	m.apiKeyInput.SetWidth(max(1, detailInnerWidth-lipgloss.Width(m.apiKeyInput.Prompt)))

	m.previewArea = imageArea{width: detailInnerWidth}
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
	m.previewArea.height = max(0, m.detail.Height()-m.detailHeaderHeight(record))

	parts := m.detailHeader(record)
	if m.descriptionVisible() {
		parts = append(parts, "", accentText.Bold(true).Render("Description"), "", strings.TrimSpace(record.Description))
	} else if record.PreviewPath != "" && m.nativeImageMatches(record.PreviewPath) {
		parts = append(parts, m.nativeImage.placeholders)
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

func runTUI(db *sql.DB, paths AppPaths, apiKey string, source apiKeySource, lookupCredential bool, protocol imageProtocol) error {
	output := newNativeImageOutput(os.Stdout)
	model, err := newTUIModelFromLibrary(db, paths, apiKey, protocol, time.Now())
	if err != nil {
		return err
	}
	model.nativeOutput = output
	model.apiKeySource = source
	model.lookupCredential = lookupCredential
	model.syncStarted = !lookupCredential
	if lookupCredential {
		model.syncStatus = "Checking OS credential store for a saved NASA API key…"
	}
	program := tea.NewProgram(model, tea.WithOutput(output))
	finalModel, err := program.Run()
	model.cancelBackgroundSync()
	final, hasFinalModel := finalModel.(tuiModel)
	if hasFinalModel {
		final.cancelNativeImage()
	} else {
		model.cancelNativeImage()
	}
	model.commands.closeAndWait()
	if hasFinalModel && final.allNativeDeleteSequence() != "" {
		_, cleanupErr := os.Stdout.WriteString(final.allNativeDeleteSequence())
		if err == nil {
			err = cleanupErr
		}
	}
	return err
}

func saveAPIKeyCmd(apiKey string) tea.Cmd {
	return func() tea.Msg {
		err := saveAPIKey(apiKey)
		return apiKeySavedMsg{apiKey: apiKey, err: err}
	}
}

func removeAPIKeyCmd() tea.Cmd {
	return func() tea.Msg {
		return apiKeyRemovedMsg{err: removeSavedAPIKey()}
	}
}

func loadSavedAPIKeyCmd() tea.Cmd {
	return func() tea.Msg {
		apiKey, err := lookupSavedAPIKey(credentialLookupTimeout)
		if credentialNotFound(err) {
			err = nil
		}
		return savedAPIKeyLoadedMsg{apiKey: apiKey, err: err}
	}
}

func (m *tuiModel) closeAPIKeyInput() {
	m.showAPIKeyInput = false
	m.savingAPIKey = false
	m.apiKeyInput.Reset()
	m.apiKeyInput.Blur()
}

func (m tuiModel) renderAPIKeyInput() string {
	return strings.Join([]string{
		accentText.Bold(true).Render("NASA API Key"),
		"",
		secondaryText.Render("Current source: ") + accentText.Render(string(m.apiKeySource)),
		"",
		m.apiKeyInput.View(),
		"",
		"The key is validated with NASA before it is saved.",
		"It is stored in your operating system credential manager.",
	}, "\n")
}

func newTUIModelFromLibrary(db *sql.DB, paths AppPaths, apiKey string, protocol imageProtocol, now time.Time) (tuiModel, error) {
	recentRecords, err := listRecentAPODs(db, 30)
	if err != nil {
		return tuiModel{}, err
	}
	favoriteRecords, err := listFavoriteAPODs(db)
	if err != nil {
		return tuiModel{}, err
	}
	model := newTUIModel(recentRecords, favoriteRecords, apiKey)
	model.imageProtocol = resolveImageProtocol(protocol, os.Getenv)
	model.tmux = os.Getenv("TMUX") != ""
	model.db = db
	model.libraryCount, err = apodCount(db)
	if err != nil {
		return tuiModel{}, err
	}
	model.paths = paths
	model.syncing = true
	model.syncNow = now
	model.syncContext, model.cancelSync = context.WithCancel(context.Background())
	model.nativeContext, model.cancelNative = context.WithCancel(context.Background())
	model.nativePending = make(map[uint32]struct{})
	model.nativeKnown = make(map[uint32]struct{})
	model.nativeOutput = newNativeImageOutput(io.Discard)
	model.commands = &commandTracker{}
	model.syncStatus = "Checking for new APODs…"
	return model, nil
}

func (m tuiModel) nativeImageWanted() bool {
	record := m.selectedRecord()
	return (m.imageProtocol == imageProtocolKitty || m.imageProtocol == imageProtocolWezTerm) && !m.showDescription && !m.showHelp && !m.showAPIKeyInput && !m.activeList().SettingFilter() && record.PreviewPath != "" && m.previewArea.width > 0 && m.previewArea.height > 0
}

func (m tuiModel) nativeImageKey() string {
	if !m.nativeImageWanted() {
		return ""
	}
	record := m.selectedRecord()
	x, y := m.nativeImagePosition()
	return fmt.Sprintf("%s:%s:%dx%d@%d,%d", m.imageProtocol, record.PreviewPath, m.previewArea.width, m.previewArea.height, x, y)
}

func (m tuiModel) nativeImageMatches(path string) bool {
	return m.nativeImage.protocol == m.imageProtocol && m.nativeImage.path == path && m.nativeImage.width == m.previewArea.width && m.nativeImage.height == m.previewArea.height
}

func (m tuiModel) nativeImagePosition() (int, int) {
	listHorizontalFrame, _ := m.listStyle.GetFrameSize()
	leftOuterWidth := m.recentList.Width() + listHorizontalFrame + 1
	detailX := horizontalOuterInset + leftOuterWidth + horizontalInterPaneGap
	x := detailX + m.detailStyle.GetBorderLeftSize() + m.detailStyle.GetPaddingLeft()
	headerLines := m.detailHeaderHeight(m.selectedRecord())
	y := headerLineCount + verticalOuterInset + m.detailStyle.GetBorderTopSize() + m.detailStyle.GetPaddingTop() + headerLines
	return x, y
}

func (m tuiModel) detailHeaderHeight(record APODRecord) int {
	return strings.Count(wordwrap.String(strings.Join(m.detailHeader(record), "\n"), max(20, m.detail.Width())), "\n") + 1
}

func (m tuiModel) detailHeader(record APODRecord) []string {
	parts := []string{
		primaryText.Bold(true).Render(record.Title),
		secondaryText.Render("Date: ") + accentText.Render(record.Date),
		secondaryText.Render("Type: ") + accentText.Render(strings.ToUpper(record.MediaType)),
	}
	if record.Favorite {
		parts = append(parts, favoriteText.Render("★ Favorite"))
	}
	if record.PreviewError != "" {
		parts = append(parts, statusError.Render("Preview error: "+record.PreviewError))
	}
	return parts
}

func (m *tuiModel) requestNativeImage() tea.Cmd {
	key := m.nativeImageKey()
	if key == "" {
		return m.clearNativeImage()
	}
	if key == m.nativeTarget {
		return nil
	}
	cleanup := m.clearNativeImage()
	if m.cancelNative != nil {
		m.cancelNative()
	}
	m.nativeContext, m.cancelNative = context.WithCancel(context.Background())
	m.nativeGeneration++
	m.nativeTarget = key
	m.nativeRequest = fmt.Sprintf("%d:%s", m.nativeGeneration, key)
	request := m.nativeRequest
	path := m.selectedRecord().PreviewPath
	width := m.previewArea.width
	height := m.previewArea.height
	tmux := m.tmux
	protocol := m.imageProtocol
	x, y := m.nativeImagePosition()
	ctx := m.nativeContext
	prepare := m.trackCmd(func() tea.Msg {
		image, err := prepareNativeImage(ctx, protocol, path, width, height, x, y, tmux)
		return nativeImagePreparedMsg{image: image, key: request, err: err}
	})
	return tea.Sequence(cleanup, prepare)
}

func (m *tuiModel) clearNativeImage() tea.Cmd {
	m.nativeRequest = ""
	m.nativeTarget = ""
	if m.cancelNative != nil {
		m.cancelNative()
	}
	if m.nativeOutput != nil {
		m.nativeOutput.setPlacement("")
	}
	sequence := m.currentNativeDeleteSequence()
	if sequence == "" {
		return nil
	}
	m.nativeImage = nativeImage{}
	m.nativePending = make(map[uint32]struct{})
	m.refreshDetail(false)
	return tea.Raw(sequence)
}

func (m tuiModel) currentNativeDeleteSequence() string {
	var sequence strings.Builder
	ids := make(map[uint32]struct{}, len(m.nativePending)+1)
	if m.nativeImage.id != 0 {
		ids[m.nativeImage.id] = struct{}{}
	}
	for id := range m.nativePending {
		ids[id] = struct{}{}
	}
	for id := range ids {
		sequence.WriteString(kittyDeleteImage(id, m.tmux))
	}
	return sequence.String()
}

func (m tuiModel) allNativeDeleteSequence() string {
	var sequence strings.Builder
	for id := range m.nativeKnown {
		sequence.WriteString(kittyDeleteImage(id, m.tmux))
	}
	return sequence.String()
}

func (m tuiModel) cancelNativeImage() {
	if m.cancelNative != nil {
		m.cancelNative()
	}
}

func (m tuiModel) prepareArchiveSyncCmd() tea.Cmd {
	return m.trackCmd(func() tea.Msg {
		plan, err := prepareArchiveSyncContext(m.syncContext, m.db, m.apiKey, m.syncNow)
		return archiveSyncPreparedMsg{plan: plan, err: err}
	})
}

func (m *tuiModel) startArchiveSync() tea.Cmd {
	if m.syncStarted {
		return nil
	}
	m.syncStarted = true
	return m.prepareArchiveSyncCmd()
}

func (m tuiModel) syncArchiveItemCmd(item APODResponse) tea.Cmd {
	return m.trackCmd(func() tea.Msg {
		result, err := syncAPODItemContext(m.syncContext, m.db, m.paths, item, m.syncNow)
		return archiveItemSyncedMsg{date: item.Date, previewed: result.Previewed, previewError: result.PreviewError, err: err}
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
	m.libraryCount, err = apodCount(m.db)
	if err != nil {
		return err
	}
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
	for i, item := range listModel.VisibleItems() {
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
	query := target.FilterValue()
	filterState := target.FilterState()
	target.SetItems(items)
	if filterState != list.Unfiltered {
		target.SetFilterText(query)
		target.SetFilterState(filterState)
		if filterState != list.Filtering {
			target.FilterInput.Blur()
		}
	}
	visibleItems := target.VisibleItems()
	if len(visibleItems) == 0 {
		target.Select(0)
		return
	}
	if selected >= len(visibleItems) {
		selected = len(visibleItems) - 1
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

func (m *tuiModel) updateSearch(msg tea.Msg) tea.Cmd {
	beforeDate := m.selectedRecord().Date
	activeList := m.activeList()
	updatedInput, cmd := activeList.FilterInput.Update(msg)
	activeList.FilterInput = updatedInput
	query := activeList.FilterValue()
	activeList.SetFilterText(query)
	activeList.SetFilterState(list.Filtering)
	activeList.FilterInput.Focus()
	m.setActiveList(activeList)
	m.updatePaneTitles()
	if m.selectedRecord().Date != beforeDate {
		m.refreshDetail(true)
	}
	return cmd
}

func (m *tuiModel) startSearch() tea.Cmd {
	activeList := m.activeList()
	m.searchOriginal[m.activePane] = activeList.FilterValue()
	activeList.SetFilterText(activeList.FilterValue())
	activeList.SetFilterState(list.Filtering)
	activeList.FilterInput.Focus()
	m.setActiveList(activeList)
	return textinput.Blink
}

func (m *tuiModel) cancelSearch() tea.Cmd {
	activeList := m.activeList()
	original := m.searchOriginal[m.activePane]
	if original == "" {
		activeList.ResetFilter()
	} else {
		activeList.SetFilterText(original)
		activeList.FilterInput.Blur()
	}
	m.searchOriginal[m.activePane] = ""
	m.setActiveList(activeList)
	m.updatePaneTitles()
	m.refreshDetail(true)
	return m.requestNativeImage()
}

func (m *tuiModel) clearSearch() tea.Cmd {
	activeList := m.activeList()
	activeList.ResetFilter()
	m.setActiveList(activeList)
	m.updatePaneTitles()
	m.refreshDetail(true)
	return m.requestNativeImage()
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
	m.recentList.Title = m.paneTitle("Recent APODs", recentPane, m.recentList)
	m.favoriteList.Title = m.paneTitle("Favorites", favoritesPane, m.favoriteList)
}

func (m tuiModel) paneTitle(title string, pane activePane, listModel list.Model) string {
	if listModel.IsFiltered() {
		query := ansi.Truncate(strings.TrimSpace(listModel.FilterValue()), 16, "…")
		title += fmt.Sprintf(" • %q (%d)", query, len(listModel.VisibleItems()))
	}
	if m.activePane == pane {
		title += " • active"
	}
	return title
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
	if msg.String() == "?" {
		return true
	}
	key := msg.Key()
	if key.Text == "?" {
		return true
	}
	if key.ShiftedCode == '?' {
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
		accentText.Bold(true).Render("Keybindings"),
		"",
		favoriteText.Render("Navigation"),
		"  j / k              Move within the active pane",
		"  Tab                Switch to the next pane",
		"  Shift+Tab          Switch to the previous pane",
		"  /                  Search the active pane",
		"  Esc                Clear an applied search",
		"",
		favoriteText.Render("Actions"),
		"  Enter              Download/apply wallpaper for selected APOD",
		"  d                  Toggle image and description views",
		"  a                  Add or replace the saved NASA API key",
		"  PgUp / PgDown      Scroll the description view",
		"  Ctrl+U / Ctrl+D    Scroll half a description page",
		"  f                  Favorite or unfavorite the selected APOD",
		"  o                  Open the APOD page in your browser",
		"  u                  Open the selected media URL",
		"  ?                  Toggle this help view",
		"  Esc                Close the help view",
		"  q / Ctrl+C         Quit",
		"",
		favoriteText.Render("Notes"),
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
