package main

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

func (m *tuiModel) toggleHistory() tea.Cmd {
	if m.historyMode {
		m.historyMode = false
		m.activePane = recentPane
		m.status = "Returned to " + m.activePaneLabel()
	} else {
		if m.db == nil {
			m.status = "Wallpaper history unavailable without a local library"
			return nil
		}
		if err := m.loadHistory(); err != nil {
			m.status = fmt.Sprintf("History load failed: %v", err)
			return nil
		}
		m.historyMode = true
		m.status = "Browsing wallpaper history • most recently applied first"
	}
	m.activePane = recentPane
	m.updatePaneTitles()
	m.refreshDetail(true)
	return m.requestNativeImage()
}

func (m *tuiModel) loadHistory() error {
	records, err := listWallpaperHistory(m.db)
	if err != nil {
		return err
	}
	selected := selectedListDate(m.historyList)
	m.historyRecords = records
	m.historyLoaded = true
	m.syncSingleList(&m.historyList, records)
	selectListDate(&m.historyList, selected)
	return nil
}

func (m *tuiModel) reloadHistory() error {
	if !m.historyLoaded || m.db == nil {
		return nil
	}
	return m.loadHistory()
}
