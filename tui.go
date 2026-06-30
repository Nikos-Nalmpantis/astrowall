package main

import (
	"database/sql"
	"fmt"
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
	title string
	err   error
}

type tuiModel struct {
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
		status:      "j/k move • enter set wallpaper • q quit",
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
			return m, applyWallpaperCmd(record, m.apiKey)
		}

	case wallpaperAppliedMsg:
		m.loading = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Wallpaper update failed: %v", msg.err)
			return m, nil
		}
		m.status = fmt.Sprintf("Wallpaper set to %s", msg.title)
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
		parts = append(parts, fmt.Sprintf("Preview cache: %s", record.PreviewPath))
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

	program := tea.NewProgram(newTUIModel(records, apiKey))
	_, err = program.Run()
	return err
}

func applyWallpaperCmd(record APODRecord, apiKey string) tea.Cmd {
	return func() tea.Msg {
		imagePath, err := resolveImagePath("")
		if err != nil {
			return wallpaperAppliedMsg{title: record.Title, err: err}
		}

		apod, err := fetchAPOD(buildAPODURL(apiKey, false, record.Date))
		if err != nil {
			return wallpaperAppliedMsg{title: record.Title, err: err}
		}
		if apod.MediaType != "image" {
			return wallpaperAppliedMsg{title: record.Title, err: fmt.Errorf("%s is a %s, not an image", record.Date, apod.MediaType)}
		}

		imageURL := apod.HDURL
		if imageURL == "" {
			imageURL = apod.URL
		}
		if imageURL == "" {
			return wallpaperAppliedMsg{title: record.Title, err: fmt.Errorf("no downloadable image URL for %s", record.Date)}
		}

		if err := downloadImage(imageURL, imagePath); err != nil {
			return wallpaperAppliedMsg{title: record.Title, err: err}
		}
		if err := setWallpaper(imagePath); err != nil {
			return wallpaperAppliedMsg{title: record.Title, err: err}
		}

		return wallpaperAppliedMsg{title: record.Title, err: nil}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
