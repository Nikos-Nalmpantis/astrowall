package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestNewTUIModel_SelectsNewestRecord(t *testing.T) {
	records := []APODRecord{
		{Date: "2024-09-27", Title: "Newest", Description: "Latest item.", MediaType: "image", URL: "https://example.com/1.jpg"},
		{Date: "2024-09-26", Title: "Older", Description: "Older item.", MediaType: "image", URL: "https://example.com/2.jpg"},
	}

	m := newTUIModel(records, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	selected := m.selectedRecord()
	if selected.Date != "2024-09-27" {
		t.Fatalf("selected.Date = %q, want 2024-09-27", selected.Date)
	}
	if !strings.Contains(m.detail.View(), "Latest item.") {
		t.Fatalf("detail view = %q, want description", m.detail.View())
	}
}

func TestTUIModelTogglesImageAndDescription(t *testing.T) {
	records := []APODRecord{{
		Date:        "2024-09-27",
		Title:       "Previewed",
		Description: "A description shown only in description mode.",
		MediaType:   "image",
		PreviewPath: filepath.Join(t.TempDir(), "missing-preview.jpg"),
	}}
	m := newTUIModel(records, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)

	if strings.Contains(m.detail.View(), records[0].Description) {
		t.Fatalf("image detail contains description before toggle: %q", m.detail.View())
	}
	updated, _ = m.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	m = updated.(tuiModel)
	if !m.showDescription {
		t.Fatal("showDescription = false after d")
	}
	if !strings.Contains(m.detail.View(), records[0].Description) {
		t.Fatalf("description detail = %q", m.detail.View())
	}
	if !strings.Contains(m.detail.View(), "Description") {
		t.Fatalf("description heading missing from %q", m.detail.View())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	m = updated.(tuiModel)
	if m.showDescription {
		t.Fatal("showDescription = true after second d")
	}
	if strings.Contains(m.detail.View(), records[0].Description) {
		t.Fatalf("image detail contains description after second toggle: %q", m.detail.View())
	}
}

func TestTUIModelShowsDescriptionWhenPreviewIsUnavailable(t *testing.T) {
	record := APODRecord{Date: "2024-09-27", Title: "No preview", Description: "Automatically visible description."}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)

	if !strings.Contains(m.detail.View(), record.Description) {
		t.Fatalf("detail = %q, want automatic description", m.detail.View())
	}
	updated, _ = m.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	m = updated.(tuiModel)
	if m.showDescription || !strings.Contains(m.status, "No preview available") {
		t.Fatalf("showDescription = %t, status = %q", m.showDescription, m.status)
	}
}

func TestTUIModelDescriptionModePersistsAcrossNavigation(t *testing.T) {
	records := []APODRecord{
		{Date: "2024-09-27", Title: "Newest", Description: "Newest description.", PreviewPath: "/new.jpg"},
		{Date: "2024-09-26", Title: "Older", Description: "Older description.", PreviewPath: "/old.jpg"},
	}
	m := newTUIModel(records, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	m = updated.(tuiModel)

	if !m.showDescription || m.selectedRecord().Date != "2024-09-26" {
		t.Fatalf("showDescription = %t, selected date = %q", m.showDescription, m.selectedRecord().Date)
	}
	if !strings.Contains(m.detail.View(), "Older description.") {
		t.Fatalf("detail = %q", m.detail.View())
	}
}

func TestTUIModelScrollsDescriptionWithoutChangingSelection(t *testing.T) {
	records := []APODRecord{
		{Date: "2024-09-27", Title: "Newest", Description: strings.Repeat("Long description line.\n", 60), PreviewPath: "/preview.jpg"},
		{Date: "2024-09-26", Title: "Older", Description: "Older description."},
	}
	m := newTUIModel(records, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	m = updated.(tuiModel)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m = updated.(tuiModel)
	if m.detail.YOffset() == 0 {
		t.Fatal("description viewport did not scroll")
	}
	if got := m.selectedRecord().Date; got != "2024-09-27" {
		t.Fatalf("selected date = %q after detail scroll", got)
	}
}

func TestTUIModelOpensMaskedAPIKeyInput(t *testing.T) {
	m := newTUIModel(nil, nil, "DEMO_KEY")
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	m = updated.(tuiModel)
	if !m.showAPIKeyInput || !m.apiKeyInput.Focused() || cmd == nil {
		t.Fatalf("showAPIKeyInput = %t, focused = %t, cmd = %v", m.showAPIKeyInput, m.apiKeyInput.Focused(), cmd)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Text: "SECRET", Code: 'S'})
	m = updated.(tuiModel)
	view := m.renderAPIKeyInput()
	if strings.Contains(view, "SECRET") || !strings.Contains(view, "••••••") {
		t.Fatalf("API key input view = %q", view)
	}
}

func TestTUIModelCancelsAPIKeyInput(t *testing.T) {
	m := newTUIModel(nil, nil, "DEMO_KEY")
	m.showAPIKeyInput = true
	m.apiKeyInput.Focus()
	m.apiKeyInput.SetValue("SECRET")
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(tuiModel)
	if m.showAPIKeyInput || m.apiKeyInput.Value() != "" {
		t.Fatalf("showAPIKeyInput = %t, value = %q", m.showAPIKeyInput, m.apiKeyInput.Value())
	}
}

func TestTUIModelAPIKeyInputAcceptsLetterQ(t *testing.T) {
	m := newTUIModel(nil, nil, "DEMO_KEY")
	m.showAPIKeyInput = true
	m.apiKeyInput.Focus()
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "q", Code: 'q'})
	m = updated.(tuiModel)
	if m.apiKeyInput.Value() != "q" {
		t.Fatalf("command = %v, value = %q", cmd, m.apiKeyInput.Value())
	}
}

func TestTUIModelCanQuitWhileSavingAPIKey(t *testing.T) {
	m := newTUIModel(nil, nil, "DEMO_KEY")
	m.showAPIKeyInput = true
	m.savingAPIKey = true
	_, cmd := m.Update(tea.KeyPressMsg{Text: "ctrl+c", Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+C command = nil while saving API key")
	}
}

func TestTUIModelSavesAPIKeyFromInput(t *testing.T) {
	withCredentialStubs(t)
	validateKey = func(string) error { return nil }
	keyringSet = func(service, account, key string) error { return nil }
	m := newTUIModel(nil, nil, "DEMO_KEY")
	m.showAPIKeyInput = true
	m.apiKeyInput.Focus()
	m.apiKeyInput.SetValue("SAVED_KEY")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(tuiModel)
	if !m.savingAPIKey || cmd == nil {
		t.Fatalf("savingAPIKey = %t, cmd = %v", m.savingAPIKey, cmd)
	}
	updated, _ = m.Update(cmd())
	m = updated.(tuiModel)
	if m.showAPIKeyInput || m.apiKey != "SAVED_KEY" || m.apiKeySource != apiKeySourceCredential {
		t.Fatalf("dialog = %t, key = %q, source = %q", m.showAPIKeyInput, m.apiKey, m.apiKeySource)
	}
}

func TestTUIModelKeepsInputOpenAfterAPIKeyFailure(t *testing.T) {
	m := newTUIModel(nil, nil, "DEMO_KEY")
	m.showAPIKeyInput = true
	m.savingAPIKey = true
	m.apiKeyInput.SetValue("INVALID")
	updated, _ := m.Update(apiKeySavedMsg{apiKey: "INVALID", err: errors.New("invalid key")})
	m = updated.(tuiModel)
	if !m.showAPIKeyInput || m.savingAPIKey || m.apiKey != "DEMO_KEY" {
		t.Fatalf("dialog = %t, saving = %t, key = %q", m.showAPIKeyInput, m.savingAPIKey, m.apiKey)
	}
}

func TestTUIModelSavingCredentialPreservesExplicitKey(t *testing.T) {
	m := newTUIModel(nil, nil, "ENV_KEY")
	m.apiKeySource = apiKeySourceEnv
	m.showAPIKeyInput = true
	updated, _ := m.Update(apiKeySavedMsg{apiKey: "SAVED_KEY"})
	m = updated.(tuiModel)
	if m.apiKey != "ENV_KEY" || m.apiKeySource != apiKeySourceEnv {
		t.Fatalf("key = %q, source = %q", m.apiKey, m.apiKeySource)
	}
}

func TestTUIModelRemovingCredentialPreservesExplicitKey(t *testing.T) {
	m := newTUIModel(nil, nil, "FLAG_KEY")
	m.apiKeySource = apiKeySourceFlag
	m.showAPIKeyInput = true
	updated, _ := m.Update(apiKeyRemovedMsg{})
	m = updated.(tuiModel)
	if m.apiKey != "FLAG_KEY" || m.apiKeySource != apiKeySourceFlag {
		t.Fatalf("key = %q, source = %q", m.apiKey, m.apiKeySource)
	}
}

func TestTUIModelLoadsSavedKeyBeforeStartingSync(t *testing.T) {
	m := newTUIModel(nil, nil, "DEMO_KEY")
	m.lookupCredential = true
	m.apiKeySource = apiKeySourceDemo
	updated, cmd := m.Update(savedAPIKeyLoadedMsg{apiKey: "SAVED_KEY"})
	m = updated.(tuiModel)
	if m.apiKey != "SAVED_KEY" || m.apiKeySource != apiKeySourceCredential || m.lookupCredential {
		t.Fatalf("key = %q, source = %q, lookup = %t", m.apiKey, m.apiKeySource, m.lookupCredential)
	}
	if cmd == nil {
		t.Fatal("archive sync command = nil after credential lookup")
	}
}

func TestTUIModelUsesDemoAfterSavedKeyLookupFailure(t *testing.T) {
	m := newTUIModel(nil, nil, "DEMO_KEY")
	m.lookupCredential = true
	updated, cmd := m.Update(savedAPIKeyLoadedMsg{err: errors.New("timed out")})
	m = updated.(tuiModel)
	if m.apiKey != "DEMO_KEY" || m.apiKeySource != apiKeySourceDemo || m.lookupCredential {
		t.Fatalf("key = %q, source = %q, lookup = %t", m.apiKey, m.apiKeySource, m.lookupCredential)
	}
	if cmd == nil || !strings.Contains(m.status, "using DEMO_KEY") {
		t.Fatalf("command = %v, status = %q", cmd, m.status)
	}
}

func TestTUIModelIgnoresStaleSavedKeyLookupAfterRemoval(t *testing.T) {
	m := newTUIModel(nil, nil, "SAVED_KEY")
	m.apiKeySource = apiKeySourceCredential
	m.lookupCredential = true
	updated, _ := m.Update(apiKeyRemovedMsg{})
	m = updated.(tuiModel)
	updated, _ = m.Update(savedAPIKeyLoadedMsg{apiKey: "STALE_KEY"})
	m = updated.(tuiModel)
	if m.apiKey != "DEMO_KEY" || m.apiKeySource != apiKeySourceDemo {
		t.Fatalf("key = %q, source = %q", m.apiKey, m.apiKeySource)
	}
}

func TestTUIModelIgnoresStaleSavedKeyLookupAfterSave(t *testing.T) {
	m := newTUIModel(nil, nil, "DEMO_KEY")
	m.lookupCredential = true
	m.savingAPIKey = true
	updated, _ := m.Update(apiKeySavedMsg{apiKey: "NEW_KEY"})
	m = updated.(tuiModel)
	updated, _ = m.Update(savedAPIKeyLoadedMsg{apiKey: "STALE_KEY"})
	m = updated.(tuiModel)
	if m.apiKey != "NEW_KEY" || m.apiKeySource != apiKeySourceCredential {
		t.Fatalf("key = %q, source = %q", m.apiKey, m.apiKeySource)
	}
}

func TestTUIModelDefersStaleLookupUntilSaveCompletes(t *testing.T) {
	m := newTUIModel(nil, nil, "DEMO_KEY")
	m.lookupCredential = false
	m.savingAPIKey = true
	updated, cmd := m.Update(savedAPIKeyLoadedMsg{apiKey: "OLD_KEY"})
	m = updated.(tuiModel)
	if cmd != nil || m.syncStarted || !m.hasPendingKey {
		t.Fatalf("command = %v, syncStarted = %t, pending = %t", cmd, m.syncStarted, m.hasPendingKey)
	}
	updated, _ = m.Update(apiKeySavedMsg{apiKey: "NEW_KEY"})
	m = updated.(tuiModel)
	if m.apiKey != "NEW_KEY" || m.hasPendingKey {
		t.Fatalf("key = %q, pending = %t", m.apiKey, m.hasPendingKey)
	}
}

func TestTUIModelUsesDeferredCredentialAfterSaveFailure(t *testing.T) {
	m := newTUIModel(nil, nil, "DEMO_KEY")
	m.lookupCredential = false
	m.savingAPIKey = true
	updated, _ := m.Update(savedAPIKeyLoadedMsg{apiKey: "OLD_KEY"})
	m = updated.(tuiModel)
	updated, cmd := m.Update(apiKeySavedMsg{apiKey: "NEW_KEY", err: errors.New("save failed")})
	m = updated.(tuiModel)
	if m.apiKey != "OLD_KEY" || m.apiKeySource != apiKeySourceCredential {
		t.Fatalf("key = %q, source = %q", m.apiKey, m.apiKeySource)
	}
	if cmd == nil || !m.syncStarted {
		t.Fatalf("command = %v, syncStarted = %t", cmd, m.syncStarted)
	}
}

func TestTUIModelActivatesAndClearsNativePreview(t *testing.T) {
	record := APODRecord{Date: "2024-09-27", Title: "Native", PreviewPath: "/preview.jpg"}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	m.imageProtocol = imageProtocolKitty
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	key := m.nativeRequest
	if key == "" {
		t.Fatal("native request key is empty")
	}

	m.nativeKnown = make(map[uint32]struct{})
	native := nativeImage{id: 42, path: record.PreviewPath, width: m.previewArea.width, height: m.previewArea.height, protocol: imageProtocolKitty, placeholders: "native placeholders"}
	updated, _ = m.Update(nativeImageActivatedMsg{image: native, key: key})
	m = updated.(tuiModel)
	if !strings.Contains(m.detail.View(), native.placeholders) {
		t.Fatalf("detail = %q, want native placeholders", m.detail.View())
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	m = updated.(tuiModel)
	if m.nativeImage.id != 0 {
		t.Fatalf("native image ID = %d after description toggle", m.nativeImage.id)
	}
	if cmd == nil {
		t.Fatal("description toggle did not return native cleanup command")
	}
	msg, ok := cmd().(tea.RawMsg)
	if !ok || !strings.Contains(fmt.Sprint(msg.Msg), "a=d,d=I,i=42") {
		t.Fatalf("cleanup message = %#v", msg)
	}
}

func TestTUIModelANSIProtocolRequestsPreviewWithoutNativeImage(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", PreviewPath: "/preview.jpg"}}, nil, "KEY")
	m.imageProtocol = imageProtocolANSI
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	if cmd == nil || m.nativeRequest != "" || m.ansiKey == "" {
		t.Fatalf("ANSI resize command = %v, native request = %q, ANSI key = %q", cmd, m.nativeRequest, m.ansiKey)
	}
}

func TestTUIANSIRequestsAreAsynchronousAndIgnoreStaleResults(t *testing.T) {
	records := []APODRecord{
		{Date: "2024-09-27", Title: "First", PreviewPath: "/first.jpg"},
		{Date: "2024-09-26", Title: "Second", PreviewPath: "/second.jpg"},
	}
	m := newTUIModel(records, nil, "KEY")
	m.imageProtocol = imageProtocolANSI
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	firstKey, firstGeneration := m.ansiKey, m.ansiGeneration
	if cmd == nil || !strings.Contains(m.detail.View(), "Preparing image preview") {
		t.Fatalf("initial preview must be deferred; cmd=%v detail=%q", cmd, m.detail.View())
	}
	updated, _ = m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	m = updated.(tuiModel)
	secondKey := m.ansiKey
	if secondKey == firstKey || m.ansiGeneration == firstGeneration {
		t.Fatalf("navigation did not replace request: old=%q new=%q", firstKey, secondKey)
	}
	updated, _ = m.Update(ansiPreviewPreparedMsg{key: firstKey, generation: firstGeneration, preview: "STALE PREVIEW"})
	m = updated.(tuiModel)
	if strings.Contains(m.detail.View(), "STALE PREVIEW") {
		t.Fatalf("stale image shown: %q", m.detail.View())
	}
	updated, _ = m.Update(ansiPreviewPreparedMsg{key: secondKey, generation: m.ansiGeneration, preview: "CURRENT PREVIEW"})
	m = updated.(tuiModel)
	if !strings.Contains(m.detail.View(), "CURRENT PREVIEW") {
		t.Fatalf("current preview missing: %q", m.detail.View())
	}
	updated, _ = m.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	m = updated.(tuiModel)
	if m.ansiKey != "" || strings.Contains(m.detail.View(), "CURRENT PREVIEW") {
		t.Fatalf("description retained ANSI image: key=%q detail=%q", m.ansiKey, m.detail.View())
	}
	updated, _ = m.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	m = updated.(tuiModel)
	if !strings.Contains(m.detail.View(), "CURRENT PREVIEW") || m.ansiKey != secondKey {
		t.Fatalf("cached ANSI image not reused: key=%q detail=%q", m.ansiKey, m.detail.View())
	}
}

func TestTUIANSIResizeRejectsOldResultAndReusesCacheBySize(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", PreviewPath: "/preview.jpg"}}, nil, "KEY")
	m.imageProtocol = imageProtocolANSI
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	oldKey := m.ansiKey
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(tuiModel)
	if m.ansiKey == oldKey {
		t.Fatal("resize must request new preview dimensions")
	}
	updated, _ = m.Update(ansiPreviewPreparedMsg{key: oldKey, generation: m.ansiGeneration - 1, preview: "OLD SIZE"})
	m = updated.(tuiModel)
	if strings.Contains(m.detail.View(), "OLD SIZE") {
		t.Fatal("old-size image displayed")
	}
	updated, _ = m.Update(ansiPreviewPreparedMsg{key: m.ansiKey, generation: m.ansiGeneration, preview: "NEW SIZE"})
	m = updated.(tuiModel)
	if !strings.Contains(m.detail.View(), "NEW SIZE") {
		t.Fatal("new-size image not displayed")
	}
}

func TestTUIANSIRejectsOldGenerationAfterReturningToSameSelection(t *testing.T) {
	m := newTUIModel([]APODRecord{
		{Date: "2024-09-27", PreviewPath: "/first.jpg"},
		{Date: "2024-09-26", PreviewPath: "/second.jpg"},
	}, nil, "KEY")
	m.imageProtocol = imageProtocolANSI
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	oldKey, oldGeneration := m.ansiKey, m.ansiGeneration
	updated, _ = m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "k", Code: 'k'})
	m = updated.(tuiModel)
	if m.ansiKey != oldKey || m.ansiGeneration == oldGeneration {
		t.Fatalf("expected new generation for same key: key=%q generation=%d", m.ansiKey, m.ansiGeneration)
	}
	updated, _ = m.Update(ansiPreviewPreparedMsg{key: oldKey, generation: oldGeneration, preview: "OLD REQUEST"})
	m = updated.(tuiModel)
	if strings.Contains(m.detail.View(), "OLD REQUEST") {
		t.Fatal("old request applied after returning to same selection")
	}
}

func TestTUIWezTermPreparationFailureFallsBackWithoutRetryLoop(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", PreviewPath: "/preview.jpg"}}, nil, "KEY")
	m.imageProtocol = imageProtocolWezTerm
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	updated, cmd := m.Update(nativeImagePreparedMsg{key: m.nativeRequest, err: os.ErrNotExist})
	m = updated.(tuiModel)
	if cmd == nil || m.nativeImageWanted() || m.ansiKey == "" || m.nativeRequest != "" {
		t.Fatalf("fallback state: cmd=%v native=%t ansi=%q request=%q", cmd, m.nativeImageWanted(), m.ansiKey, m.nativeRequest)
	}
	updated, cmd = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	if cmd != nil || m.nativeRequest != "" {
		t.Fatalf("unchanged failed preview retried: cmd=%v native=%q", cmd, m.nativeRequest)
	}
}

func TestTUIModelRendersWezTermPreviewAfterReservationFrame(t *testing.T) {
	record := APODRecord{Date: "2024-09-27", PreviewPath: "/preview.jpg"}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	m.imageProtocol = imageProtocolWezTerm
	m.nativePending = make(map[uint32]struct{})
	m.nativeKnown = make(map[uint32]struct{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	native := nativeImage{id: 42, path: record.PreviewPath, width: m.previewArea.width, height: m.previewArea.height, protocol: imageProtocolWezTerm, transmission: "wezterm sequence", placement: "wezterm placement", placeholders: "blank grid"}

	updated, cmd := m.Update(nativeImagePreparedMsg{image: native, key: m.nativeRequest})
	m = updated.(tuiModel)
	if cmd == nil || m.nativeImage.id != 42 {
		t.Fatalf("command = %v, native image = %#v", cmd, m.nativeImage)
	}
	if !strings.Contains(m.detail.View(), native.placeholders) {
		t.Fatalf("detail = %q, want reservation grid", m.detail.View())
	}
	if strings.Contains(m.View().Content, native.transmission) {
		t.Fatal("WezTerm placement is emitted before the reservation frame settles")
	}
	if m.nativeOutput == nil {
		t.Fatal("native output is nil")
	}
}

func TestTUIModelKeepsWezTermPreviewAcrossUnrelatedFrames(t *testing.T) {
	record := APODRecord{Date: "2024-09-27", PreviewPath: "/preview.jpg"}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	m.imageProtocol = imageProtocolWezTerm
	m.nativePending = make(map[uint32]struct{})
	m.nativeKnown = make(map[uint32]struct{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	native := nativeImage{id: 42, path: record.PreviewPath, width: m.previewArea.width, height: m.previewArea.height, protocol: imageProtocolWezTerm, transmission: "wezterm sequence", placeholders: blankImageGrid(m.previewArea.width, m.previewArea.height)}
	updated, _ = m.Update(nativeImageActivatedMsg{image: native, key: m.nativeRequest})
	m = updated.(tuiModel)

	updated, cmd := m.Update(urlOpenedMsg{target: "APOD page"})
	m = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("status update scheduled an image-clearing command")
	}
	if m.nativeImage.id != 42 {
		t.Fatal("WezTerm placement was discarded after unrelated update")
	}
}

func TestTUIModelWezTermCleanupDeletesPlacement(t *testing.T) {
	record := APODRecord{Date: "2024-09-27", PreviewPath: "/preview.jpg"}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	m.imageProtocol = imageProtocolWezTerm
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	m.nativeImage = nativeImage{id: 42, path: record.PreviewPath, protocol: imageProtocolWezTerm}

	cmd := m.clearNativeImage()
	if cmd == nil {
		t.Fatal("iTerm cleanup command = nil")
	}
	if m.nativeImage.protocol != "" {
		t.Fatalf("native protocol = %q after cleanup", m.nativeImage.protocol)
	}
	msg := cmd().(tea.RawMsg)
	if !strings.Contains(fmt.Sprint(msg.Msg), "a=d,d=I,i=42") {
		t.Fatalf("cleanup = %#v", msg)
	}
}

func TestTUIModelFallsBackToANSIOnNativePreparationError(t *testing.T) {
	record := APODRecord{Date: "2024-09-27", PreviewPath: "/preview.jpg"}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	m.imageProtocol = imageProtocolWezTerm
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	request := m.nativeRequest

	updated, _ = m.Update(nativeImagePreparedMsg{key: request, err: os.ErrNotExist})
	m = updated.(tuiModel)
	if m.nativeTarget != "" {
		t.Fatalf("native target = %q after preparation error", m.nativeTarget)
	}
	if m.nativeFallback == "" || m.ansiKey == "" {
		t.Fatal("native failure did not start ANSI fallback")
	}
	if cmd := m.requestNativeImage(); cmd != nil {
		t.Fatal("native failure immediately retried instead of preserving ANSI fallback")
	}
}

func TestTUIModelNativeImagePositionAccountsForFavoriteMetadata(t *testing.T) {
	record := APODRecord{Date: "2024-09-27", PreviewPath: "/preview.jpg"}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	x, y := m.nativeImagePosition()
	m.recentRecords[0].Favorite = true
	m.syncListItems()
	_, favoriteY := m.nativeImagePosition()
	if x <= 0 || y <= 0 || favoriteY != y {
		t.Fatalf("normal position = %d,%d; favorite y = %d", x, y, favoriteY)
	}
}

func TestTUIModelNativeImagePositionAccountsForDashboardHeader(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", Title: "Recent", PreviewPath: "/preview.jpg"}}, nil, "KEY")
	m.imageProtocol = imageProtocolWezTerm
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	_, y := m.nativeImagePosition()
	want := headerLineCount + verticalOuterInset + m.detailStyle.GetBorderTopSize() + m.detailStyle.GetPaddingTop() + m.detailHeaderHeight(m.selectedRecord())
	if y != want || y+m.previewArea.height > m.height-statusLineCount-verticalOuterInset {
		t.Fatalf("image y=%d height=%d, want start %d inside %d rows", y, m.previewArea.height, want, m.height)
	}
}

func TestTUIModelNativeImagePositionAccountsForWrappedTitle(t *testing.T) {
	short := APODRecord{Date: "2024-09-27", Title: "Short", PreviewPath: "/preview.jpg"}
	m := newTUIModel([]APODRecord{short}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m = updated.(tuiModel)
	_, shortY := m.nativeImagePosition()

	m.recentRecords[0].Title = strings.Repeat("A wrapped title ", 12)
	m.syncListItems()
	m.refreshDetail(true)
	_, wrappedY := m.nativeImagePosition()
	if wrappedY <= shortY {
		t.Fatalf("wrapped title y = %d, short title y = %d", wrappedY, shortY)
	}
	if wrappedY+m.previewArea.height > headerLineCount+verticalOuterInset+m.detailStyle.GetBorderTopSize()+m.detail.Height() {
		t.Fatalf("native image bottom %d exceeds detail content bottom", wrappedY+m.previewArea.height)
	}
}

func TestTUIModelNativePreviewNeverExceedsDetailWidth(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", PreviewPath: "/preview.jpg"}}, nil, "KEY")
	m.imageProtocol = imageProtocolWezTerm
	for _, width := range []int{20, 30, 40, 60} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 20})
		m = updated.(tuiModel)
		if m.previewArea.width > m.detail.Width() {
			t.Fatalf("window width %d: preview width %d exceeds detail width %d", width, m.previewArea.width, m.detail.Width())
		}
	}
}

func TestTUIModelResizeInvalidatesNativePreview(t *testing.T) {
	record := APODRecord{Date: "2024-09-27", PreviewPath: "/preview.jpg"}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	m.imageProtocol = imageProtocolKitty
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	firstKey := m.nativeRequest
	m.nativeImage = nativeImage{id: 42, path: record.PreviewPath, width: m.previewArea.width, height: m.previewArea.height, protocol: imageProtocolKitty}

	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	if cmd == nil || m.nativeRequest == firstKey {
		t.Fatalf("resize command = %v, request key = %q", cmd, m.nativeRequest)
	}
}

func TestTUIModelStaleNativeActivationCannotDeleteNewGeneration(t *testing.T) {
	record := APODRecord{Date: "2024-09-27", PreviewPath: "/preview.jpg"}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	m.imageProtocol = imageProtocolKitty
	m.nativePending = make(map[uint32]struct{})
	m.nativeKnown = make(map[uint32]struct{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	oldKey := m.nativeRequest
	oldImage := nativeImage{id: 41, path: record.PreviewPath, width: m.previewArea.width, height: m.previewArea.height, protocol: imageProtocolKitty}

	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	newKey := m.nativeRequest
	newImage := nativeImage{id: 42, path: record.PreviewPath, width: m.previewArea.width, height: m.previewArea.height, protocol: imageProtocolKitty, placeholders: "new"}
	updated, _ = m.Update(nativeImageActivatedMsg{image: newImage, key: newKey})
	m = updated.(tuiModel)
	updated, cmd := m.Update(nativeImageActivatedMsg{image: oldImage, key: oldKey})
	m = updated.(tuiModel)

	if m.nativeImage.id != 42 {
		t.Fatalf("active native image = %d, want 42", m.nativeImage.id)
	}
	msg := cmd().(tea.RawMsg)
	sequence := fmt.Sprint(msg.Msg)
	if !strings.Contains(sequence, "i=41") || strings.Contains(sequence, "i=42") {
		t.Fatalf("stale cleanup = %q", sequence)
	}
}

func TestTUIModelWindowResizeSetsReady(t *testing.T) {
	m := newTUIModel(nil, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := updated.(tuiModel)
	if !model.ready {
		t.Fatal("model.ready = false, want true")
	}
	if model.recentList.Width() == 0 || model.favoriteList.Width() == 0 {
		t.Fatal("list width = 0, want resized lists")
	}
	if model.detail.Width() == 0 || model.detail.Height() == 0 {
		t.Fatal("detail viewport not resized")
	}
}

func TestTUIModelKeepsListPaneHeightsIndependentOfContent(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent"}}
	m := newTUIModel(recent, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	recentHeight := m.recentList.Height()
	favoriteHeight := m.favoriteList.Height()
	recentRenderedWidth := lipgloss.Width(m.renderListPane(recentPane, m.recentList))
	favoriteRenderedWidth := lipgloss.Width(m.renderListPane(favoritesPane, m.favoriteList))

	m.favoriteRecords = make([]APODRecord, 20)
	for i := range m.favoriteRecords {
		m.favoriteRecords[i] = APODRecord{Date: fmt.Sprintf("2024-08-%02d", i+1), Title: strings.Repeat("Favorite ", i+1), Favorite: true}
	}
	m.syncListItems()
	m.resize()

	if m.recentList.Height() != recentHeight {
		t.Fatalf("recent height changed from %d to %d after adding favorites", recentHeight, m.recentList.Height())
	}
	if m.favoriteList.Height() != favoriteHeight {
		t.Fatalf("favorite height changed from %d to %d after adding favorites", favoriteHeight, m.favoriteList.Height())
	}
	if got := lipgloss.Width(m.renderListPane(recentPane, m.recentList)); got != recentRenderedWidth {
		t.Fatalf("rendered recent width changed from %d to %d after adding favorites", recentRenderedWidth, got)
	}
	if got := lipgloss.Width(m.renderListPane(favoritesPane, m.favoriteList)); got != favoriteRenderedWidth {
		t.Fatalf("rendered favorite width changed from %d to %d after adding favorites", favoriteRenderedWidth, got)
	}
	if difference := m.recentList.Height() - m.favoriteList.Height(); difference < 0 || difference > 1 {
		t.Fatalf("list pane height difference = %d, want 0 or 1", difference)
	}
	if m.favoriteList.Paginator.TotalPages <= 1 {
		t.Fatalf("favorite pages = %d, want pagination", m.favoriteList.Paginator.TotalPages)
	}
}

func TestTUIModelFitsWindow(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent", Description: strings.Repeat("Description ", 30)}}
	favorites := []APODRecord{{Date: "2024-08-01", Title: "Favorite", Favorite: true}}

	for _, size := range []struct {
		width  int
		height int
	}{{120, 40}, {80, 24}, {60, 20}} {
		for _, showHelp := range []bool{false, true} {
			m := newTUIModel(recent, favorites, "KEY")
			updated, _ := m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			m = updated.(tuiModel)
			m.showHelp = showHelp
			view := m.View().Content

			if got := lipgloss.Width(view); got > size.width {
				t.Errorf("%dx%d help=%t view width = %d", size.width, size.height, showHelp, got)
			}
			if got := lipgloss.Height(view); got > size.height {
				t.Errorf("%dx%d help=%t view height = %d", size.width, size.height, showHelp, got)
			}
		}
	}
}

func TestTUIResponsiveLayoutsKeepSelectionSearchAndImageInsidePanes(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent", PreviewPath: "/preview.jpg"}}
	favorites := []APODRecord{{Date: "2024-09-26", Title: "Favorite", Favorite: true, PreviewPath: "/favorite.jpg"}}
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 40}, {Width: 79, Height: 24}, {Width: 60, Height: 20}, {Width: 49, Height: 24}, {Width: 35, Height: 18}, {Width: 30, Height: 14}, {Width: 25, Height: 10}} {
		m := newTUIModel(recent, favorites, "KEY")
		m.imageProtocol = imageProtocolWezTerm
		updated, _ := m.Update(size)
		m = updated.(tuiModel)
		check := func(stage string) {
			t.Helper()
			view := m.View().Content
			if got := lipgloss.Width(view); got > size.Width {
				t.Errorf("%dx%d %s: width %d", size.Width, size.Height, stage, got)
			}
			if got := lipgloss.Height(view); got > size.Height {
				t.Errorf("%dx%d %s: height %d", size.Width, size.Height, stage, got)
			}
			if m.nativeImageWanted() {
				x, y := m.nativeImagePosition()
				if x < horizontalOuterInset || y < headerLineCount+verticalOuterInset || x+m.previewArea.width > size.Width-horizontalOuterInset || y+m.previewArea.height > size.Height-statusLineCount-verticalOuterInset {
					t.Errorf("%dx%d %s: preview at %d,%d size %dx%d outside pane", size.Width, size.Height, stage, x, y, m.previewArea.width, m.previewArea.height)
				}
			}
		}
		check("recent")
		if size.Width < minimumDashboardWidth && !m.minimalLayout() {
			t.Fatal("tiny window should use list-only layout")
		}
		updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = updated.(tuiModel)
		if m.selectedRecord().Title != "Favorite" {
			t.Fatalf("%dx%d: Tab did not select favorite", size.Width, size.Height)
		}
		check("favorites")
		m = typeSearchQuery(t, m, "No match")
		if !strings.Contains(m.detail.View(), "No matches") {
			t.Errorf("%dx%d: no-results message missing: %q", size.Width, size.Height, m.detail.View())
		}
		check("search")
	}
}

func TestTUIEmptyFavoritesCanBeVisitedWithTab(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", Title: "Recent"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(tuiModel)
	if m.activePane != favoritesPane || !strings.Contains(ansi.Strip(m.View().Content), "No favorites yet") {
		t.Fatalf("empty favorites not reachable or not explained: %q", m.View().Content)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = updated.(tuiModel)
	if m.activePane != recentPane || m.selectedRecord().Title != "Recent" {
		t.Fatal("could not return to Recent from empty Favorites")
	}
}

func TestTUISearchEmptyFavoritesExplainsNoMatches(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", Title: "Recent"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(tuiModel)
	m = typeSearchQuery(t, m, "Moon")
	if !strings.Contains(ansi.Strip(m.detail.View()), "No matches in Favorites") {
		t.Fatalf("empty favorites search detail = %q", m.detail.View())
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(tuiModel)
	if !strings.Contains(ansi.Strip(m.detail.View()), "No favorites yet") {
		t.Fatalf("cleared search detail = %q", m.detail.View())
	}
}

func TestTUIEmptyStates(t *testing.T) {
	for _, tc := range []struct {
		name      string
		recent    []APODRecord
		favorites []APODRecord
		pane      activePane
		want      string
	}{
		{"library", nil, nil, recentPane, "library is empty"},
		{"favorites", []APODRecord{{Date: "2024-09-27", Title: "Recent"}}, nil, favoritesPane, "No favorites yet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTUIModel(tc.recent, tc.favorites, "KEY")
			m.activePane = tc.pane
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			m = updated.(tuiModel)
			if !strings.Contains(m.detail.View(), tc.want) {
				t.Fatalf("detail = %q, want %q", m.detail.View(), tc.want)
			}
			if m.previewArea.height != 0 || m.nativeImageWanted() {
				t.Fatalf("empty pane requested image: area = %#v", m.previewArea)
			}
		})
	}
}

func TestTUIResizeFromWideToStackedReplacesNativePlacement(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", Title: "Recent", PreviewPath: "/preview.jpg"}}, nil, "KEY")
	m.imageProtocol = imageProtocolWezTerm
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(tuiModel)
	oldKey := m.nativeRequest
	m.nativeImage = nativeImage{id: 42, path: "/preview.jpg", protocol: imageProtocolWezTerm, width: m.previewArea.width, height: m.previewArea.height}
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	m = updated.(tuiModel)
	if cmd == nil || m.nativeRequest == oldKey || m.nativeTarget == "" || m.nativeImage.id != 0 {
		t.Fatalf("resize must clear and replace placement: old=%q new=%q active=%d", oldKey, m.nativeRequest, m.nativeImage.id)
	}
	if x, _ := m.nativeImagePosition(); x != horizontalOuterInset+m.detailStyle.GetBorderLeftSize()+m.detailStyle.GetPaddingLeft() {
		t.Fatalf("stacked preview x=%d", x)
	}
}

func TestTUIModelUsesFixedPaneGapsAcrossResizes(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent"}}
	favorites := []APODRecord{{Date: "2024-08-01", Title: "Favorite", Favorite: true}}
	m := newTUIModel(recent, favorites, "KEY")

	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 120, Height: 40}} {
		updated, _ := m.Update(size)
		m = updated.(tuiModel)

		listHorizontalFrame, listVerticalFrame := m.listStyle.GetFrameSize()
		detailHorizontalFrame, _ := m.detailStyle.GetFrameSize()
		leftWidth := m.recentList.Width() + listHorizontalFrame + 1
		rightWidth := m.detail.Width() + detailHorizontalFrame
		if got := size.Width - leftWidth - rightWidth; got != horizontalOuterInset*2+horizontalInterPaneGap {
			t.Fatalf("%dx%d total horizontal spacing = %d, want %d", size.Width, size.Height, got, horizontalOuterInset*2+horizontalInterPaneGap)
		}

		contentHeight := size.Height - headerLineCount - statusLineCount - verticalOuterInset*2 - verticalInterPaneGap*2
		listHeight := m.recentList.Height() + m.favoriteList.Height() + listVerticalFrame*2
		if got := contentHeight - listHeight; got != verticalInterPaneGap {
			t.Fatalf("%dx%d inter-pane vertical spacing = %d, want %d", size.Width, size.Height, got, verticalInterPaneGap)
		}

		leftColumn := lipgloss.JoinVertical(
			lipgloss.Left,
			m.renderListPane(recentPane, m.recentList),
			m.renderListPane(favoritesPane, m.favoriteList),
		)
		lines := strings.Split(ansi.Strip(leftColumn), "\n")
		recentHeight := lipgloss.Height(m.renderListPane(recentPane, m.recentList))
		if !strings.HasPrefix(lines[recentHeight+verticalInterPaneGap], "╔") {
			t.Fatalf("%dx%d favorites did not start at computed boundary: %q", size.Width, size.Height, lines[recentHeight+verticalInterPaneGap])
		}

		leftPane := m.renderListPane(recentPane, m.recentList)
		detailPane := m.detailStyle.Render(m.detail.View())
		if got := lipgloss.Width(leftPane); got != leftWidth {
			t.Fatalf("%dx%d rendered left width = %d, allocated %d", size.Width, size.Height, got, leftWidth)
		}
		if got := lipgloss.Width(detailPane); got != rightWidth {
			t.Fatalf("%dx%d rendered detail width = %d, allocated %d", size.Width, size.Height, got, rightWidth)
		}
		if got := horizontalOuterInset*2 + lipgloss.Width(leftPane) + horizontalInterPaneGap + lipgloss.Width(detailPane); got != size.Width {
			t.Fatalf("%dx%d rendered pane row width = %d", size.Width, size.Height, got)
		}
	}
}

func TestTUIModelPaneSizesRespondToWindowResize(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", Title: "Recent"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	smallListWidth := m.recentList.Width()
	smallDetailWidth := m.detail.Width()
	smallRecentHeight := m.recentList.Height()

	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	if m.recentList.Width() <= smallListWidth {
		t.Fatalf("list width did not grow: %d to %d", smallListWidth, m.recentList.Width())
	}
	if m.detail.Width() <= smallDetailWidth {
		t.Fatalf("detail width did not grow: %d to %d", smallDetailWidth, m.detail.Width())
	}
	if m.recentList.Height() <= smallRecentHeight {
		t.Fatalf("recent height did not grow: %d to %d", smallRecentHeight, m.recentList.Height())
	}
}

func TestTUIModelListPanesRenderBottomBorders(t *testing.T) {
	recent := make([]APODRecord, 20)
	for i := range recent {
		recent[i] = APODRecord{Date: fmt.Sprintf("2024-08-%02d", i+1), Title: "Recent"}
	}
	m := newTUIModel(recent, recent, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)

	for name, pane := range map[string]string{
		"recent":    m.renderListPane(recentPane, m.recentList),
		"favorites": m.renderListPane(favoritesPane, m.favoriteList),
	} {
		lines := strings.Split(pane, "\n")
		bottom := ansi.Strip(lines[len(lines)-1])
		if !strings.HasPrefix(bottom, "╚") || !strings.HasSuffix(bottom, "╝") {
			t.Fatalf("%s bottom border = %q", name, bottom)
		}
	}
}

func TestTUIModelUsesEqualOuterAndContentGaps(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", Title: "Recent"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")

	if !strings.Contains(lines[0], "ASTROWALL") {
		t.Fatalf("header = %q, want brand", lines[0])
	}
	if !strings.HasPrefix(lines[headerLineCount+verticalOuterInset], strings.Repeat(" ", horizontalOuterInset)+"╔") {
		t.Fatalf("first pane line = %q, want %d-cell left inset", lines[headerLineCount+verticalOuterInset], horizontalOuterInset)
	}
	if strings.TrimSpace(lines[len(lines)-1]) != "" {
		t.Fatalf("bottom inset = %q, want blank", lines[len(lines)-1])
	}

	detailBottom := headerLineCount + verticalOuterInset + lipgloss.Height(m.detailStyle.Render(m.detail.View()))
	if !strings.HasPrefix(lines[detailBottom+verticalInterPaneGap], strings.Repeat(" ", horizontalOuterInset)) {
		t.Fatalf("status line = %q, want %d-cell left inset", lines[detailBottom+verticalInterPaneGap], horizontalOuterInset)
	}
}

func TestRunTUIModelUsesRecentRecords(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}
	db, err := openLibrary(paths.DBPath)
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()

	if err := upsertAPOD(db, APODRecord{
		Date:        "2024-09-27",
		Title:       "Stored",
		Description: "Stored item.",
		MediaType:   "image",
		URL:         "https://example.com/stored.jpg",
		FetchedAt:   time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("upsertAPOD() error: %v", err)
	}

	records, err := listRecentAPODs(db, 30)
	if err != nil {
		t.Fatalf("listRecentAPODs() error: %v", err)
	}
	model := newTUIModel(records, nil, "KEY")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = updated.(tuiModel)
	if got := model.selectedRecord().Title; got != "Stored" {
		t.Fatalf("selected title = %q, want Stored", got)
	}
}

func TestNewTUIModelFromLibraryStartsWithCachedRecords(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}
	db, err := openLibrary(paths.DBPath)
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()
	if err := upsertAPOD(db, APODRecord{
		Date:        "2024-09-26",
		Title:       "Cached",
		Description: "Available before sync.",
		MediaType:   "image",
		URL:         "https://example.com/cached.jpg",
		FetchedAt:   time.Date(2024, 9, 26, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("upsertAPOD() error: %v", err)
	}

	model, err := newTUIModelFromLibrary(db, paths, "KEY", imageProtocolANSI, time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("newTUIModelFromLibrary() error: %v", err)
	}
	if got := model.selectedRecord().Title; got != "Cached" {
		t.Fatalf("selected title = %q, want Cached", got)
	}
	if !model.syncing {
		t.Fatal("syncing = false, want background sync pending")
	}
	if model.Init() == nil {
		t.Fatal("Init() command = nil, want background sync command")
	}
}

func TestTUIHeaderCountsEntireLibraryAndUpdatesDuringSync(t *testing.T) {
	db, err := openLibrary(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()
	for i := range 32 {
		record := APODRecord{Date: fmt.Sprintf("2024-09-%02d", i+1), Title: "APOD", FetchedAt: time.Now()}
		if i == 0 {
			record.Favorite = true
		}
		if err := upsertAPOD(db, record); err != nil {
			t.Fatalf("upsertAPOD() error: %v", err)
		}
	}
	m, err := newTUIModelFromLibrary(db, AppPaths{}, "KEY", imageProtocolANSI, time.Now())
	if err != nil {
		t.Fatalf("newTUIModelFromLibrary() error: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	if got := ansi.Strip(m.renderDashboardHeader(76)); !strings.Contains(got, "LIBRARY 32") || !strings.Contains(got, "★ 1") {
		t.Fatalf("header = %q, want full library and favorite counts", got)
	}

	if err := upsertAPOD(db, APODRecord{Date: "2024-10-01", Title: "New", FetchedAt: time.Now()}); err != nil {
		t.Fatalf("upsertAPOD() error: %v", err)
	}
	if err := m.reloadRecords(); err != nil {
		t.Fatalf("reloadRecords() error: %v", err)
	}
	if got := ansi.Strip(m.renderDashboardHeader(76)); !strings.Contains(got, "LIBRARY 33") {
		t.Fatalf("header after sync = %q, want 33 items", got)
	}
}

func TestTUIArchiveBrowsesOlderEntriesWithoutLosingRecentSearch(t *testing.T) {
	db, err := openLibrary(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	base := time.Date(2024, 9, 27, 0, 0, 0, 0, time.UTC)
	for i := range 36 {
		date := base.AddDate(0, 0, -i).Format("2006-01-02")
		title := "Regular"
		if i == 35 {
			title = "Ancient Nebula"
		}
		if err := upsertAPOD(db, APODRecord{Date: date, Title: title, Description: "A distant nebula", PreviewPath: "/preview.jpg", FetchedAt: base}); err != nil {
			t.Fatal(err)
		}
	}
	m, err := newTUIModelFromLibrary(db, AppPaths{}, "KEY", imageProtocolWezTerm, base)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(tuiModel)
	if len(m.recentRecords) != 30 || m.archiveLoaded {
		t.Fatal("archive loaded eagerly or Recent did not retain 30 records")
	}
	m = typeSearchQuery(t, m, "Ancient")
	if m.selectedRecord().Date != "" {
		t.Fatal("older record should not appear in Recent search")
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "b", Code: 'b'})
	m = updated.(tuiModel)
	if cmd == nil || !m.archiveLoading || m.archiveMode {
		t.Fatal("archive must load on demand without blocking the current pane")
	}
	records, err := listAllAPODs(db)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ = m.Update(archiveLoadedMsg{records: records})
	m = updated.(tuiModel)
	if !m.archiveMode || m.archiveLoading || len(m.archiveRecords) != 36 || m.archiveList.Title != "Archive • active" {
		t.Fatalf("archive state: loaded=%t count=%d title=%q", m.archiveLoaded, len(m.archiveRecords), m.archiveList.Title)
	}
	m = typeSearchQuery(t, m, "Ancient")
	wantDate := base.AddDate(0, 0, -35).Format("2006-01-02")
	if m.selectedRecord().Date != wantDate || m.nativeRequest == "" {
		t.Fatalf("older record not selected/previewed: date=%q native=%q", m.selectedRecord().Date, m.nativeRequest)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Text: "b", Code: 'b'})
	m = updated.(tuiModel)
	if m.archiveMode || m.recentList.FilterValue() != "Ancient" || m.selectedRecord().Date != "" {
		t.Fatal("Recent search not restored on toggle")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Text: "b", Code: 'b'})
	m = updated.(tuiModel)
	if m.selectedRecord().Date != wantDate || m.archiveList.FilterValue() != "Ancient" {
		t.Fatal("Archive search and selection not restored on toggle")
	}
}

func TestTUIArchiveKeepsOlderFavoritesAndSelectionAcrossSync(t *testing.T) {
	db, err := openLibrary(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	base := time.Date(2024, 9, 27, 0, 0, 0, 0, time.UTC)
	for i := range 34 {
		if err := upsertAPOD(db, APODRecord{Date: base.AddDate(0, 0, -i).Format("2006-01-02"), Title: "APOD", FetchedAt: base}); err != nil {
			t.Fatal(err)
		}
	}
	m, err := newTUIModelFromLibrary(db, AppPaths{}, "KEY", imageProtocolANSI, base)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(archiveLoadedMsg{records: mustListAllAPODs(t, db)})
	m = updated.(tuiModel)
	oldDate := base.AddDate(0, 0, -33).Format("2006-01-02")
	selectListDate(&m.archiveList, oldDate)
	if m.selectedRecord().Date != oldDate {
		t.Fatalf("selected %q instead of older record", m.selectedRecord().Date)
	}
	msg := toggleFavoriteCmd(db, oldDate)().(favoriteToggledMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	updated, _ = m.Update(msg)
	m = updated.(tuiModel)
	if !m.selectedRecord().Favorite || len(m.favoriteRecords) != 1 || m.favoriteRecords[0].Date != oldDate {
		t.Fatalf("older favorite not reflected in archive and Favorites: selected=%#v favorites=%#v", m.selectedRecord(), m.favoriteRecords)
	}
	newDate := base.AddDate(0, 0, 1).Format("2006-01-02")
	if err := upsertAPOD(db, APODRecord{Date: newDate, Title: "New", FetchedAt: base}); err != nil {
		t.Fatal(err)
	}
	if err := m.reloadRecords(); err != nil {
		t.Fatal(err)
	}
	if len(m.archiveRecords) != 35 || m.archiveRecords[0].Date != newDate || m.selectedRecord().Date != oldDate || !m.selectedRecord().Favorite {
		t.Fatalf("sync lost archive selection or favorite: count=%d selected=%#v", len(m.archiveRecords), m.selectedRecord())
	}
}

func TestTUIArchiveSearchAndSelectionSurviveFavoriteRefresh(t *testing.T) {
	db, err := openLibrary(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, record := range []APODRecord{
		{Date: "2024-09-27", Title: "Nebula One", FetchedAt: time.Now()},
		{Date: "2024-09-26", Title: "Nebula Two", FetchedAt: time.Now()},
		{Date: "2024-09-25", Title: "Moon", FetchedAt: time.Now()},
	} {
		if err := upsertAPOD(db, record); err != nil {
			t.Fatal(err)
		}
	}
	m, err := newTUIModelFromLibrary(db, AppPaths{}, "KEY", imageProtocolANSI, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	updated, _ = m.Update(archiveLoadedMsg{records: mustListAllAPODs(t, db)})
	m = updated.(tuiModel)
	m = typeSearchQuery(t, m, "Nebula")
	selectListDate(&m.archiveList, "2024-09-26")
	msg := toggleFavoriteCmd(db, "2024-09-26")().(favoriteToggledMsg)
	updated, _ = m.Update(msg)
	m = updated.(tuiModel)
	if m.selectedRecord().Date != "2024-09-26" || m.archiveList.FilterValue() != "Nebula" || len(m.archiveList.VisibleItems()) != 2 {
		t.Fatalf("favorite refresh lost archive state: date=%q query=%q visible=%d", m.selectedRecord().Date, m.archiveList.FilterValue(), len(m.archiveList.VisibleItems()))
	}
}

func mustListAllAPODs(t *testing.T, db *sql.DB) []APODRecord {
	t.Helper()
	records, err := listAllAPODs(db)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func TestTUIKeepsActionFeedbackVisibleWhileSyncProgresses(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", Title: "Recent"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	m.syncing = true
	m.status = "Wallpaper set to Recent"
	updated, _ = m.Update(archiveSyncPreparedMsg{plan: archiveSyncPlan{Items: []APODResponse{{Date: "2024-09-28"}}}})
	m = updated.(tuiModel)
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Wallpaper set to Recent") || !strings.Contains(view, "Syncing APOD 1/1") {
		t.Fatalf("action feedback and sync progress must coexist: %q", view)
	}
	if m.status != "Wallpaper set to Recent" {
		t.Fatalf("action status overwritten by sync: %q", m.status)
	}
}

func TestTUIModelAddsBackgroundSyncItemsIncrementally(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}
	db, err := openLibrary(paths.DBPath)
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()

	now := time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC)
	model, err := newTUIModelFromLibrary(db, paths, "KEY", imageProtocolANSI, now)
	if err != nil {
		t.Fatalf("newTUIModelFromLibrary() error: %v", err)
	}
	items := []APODResponse{
		{Date: "2024-09-26", Title: "First", MediaType: "video"},
		{Date: "2024-09-27", Title: "Second", MediaType: "video"},
	}

	updated, cmd := model.Update(archiveSyncPreparedMsg{plan: archiveSyncPlan{Items: items}})
	model = updated.(tuiModel)
	if cmd == nil {
		t.Fatal("first item command = nil")
	}
	updated, cmd = model.Update(cmd())
	model = updated.(tuiModel)
	if len(model.recentRecords) != 1 || model.recentRecords[0].Title != "First" {
		t.Fatalf("recent records after first item = %#v", model.recentRecords)
	}
	if cmd == nil {
		t.Fatal("second item command = nil")
	}
	updated, cmd = model.Update(cmd())
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("final item command is not nil")
	}
	if model.syncing {
		t.Fatal("syncing = true after final item")
	}
	if len(model.recentRecords) != 2 || model.recentRecords[0].Title != "Second" {
		t.Fatalf("recent records after final item = %#v", model.recentRecords)
	}
	if !strings.Contains(model.syncStatus, "Synced 2 APODs") {
		t.Fatalf("sync status = %q", model.syncStatus)
	}
}

func TestTUIModelContinuesBackgroundSyncAfterPreviewFailure(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}
	db, err := openLibrary(paths.DBPath)
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()

	m := newTUIModel(nil, nil, "KEY")
	m.db = db
	m.paths = paths
	m.syncContext, m.cancelSync = context.WithCancel(context.Background())
	m.commands = &commandTracker{}
	m.syncing = true
	m.syncItems = []APODResponse{{Date: "2024-09-26"}, {Date: "2024-09-27"}}
	m.syncTotal = len(m.syncItems)

	if err := upsertAPOD(db, APODRecord{Date: "2024-09-26", Title: "Failed preview", PreviewError: "permission denied", FetchedAt: time.Now()}); err != nil {
		t.Fatalf("upsertAPOD() error: %v", err)
	}
	updated, cmd := m.Update(archiveItemSyncedMsg{date: "2024-09-26", previewError: "permission denied"})
	m = updated.(tuiModel)
	if cmd == nil {
		t.Fatal("next item command is nil after preview failure")
	}
	if !m.syncing {
		t.Fatal("syncing = false after recoverable preview failure")
	}
	if len(m.recentRecords) != 1 || m.recentRecords[0].PreviewError == "" {
		t.Fatalf("recent records = %#v", m.recentRecords)
	}
}

func TestTUIModelStopsBackgroundSyncAfterFatalItemFailure(t *testing.T) {
	m := newTUIModel(nil, nil, "KEY")
	m.syncing = true
	m.syncItems = []APODResponse{{Date: "2024-09-26"}, {Date: "2024-09-27"}}
	m.syncTotal = len(m.syncItems)

	updated, cmd := m.Update(archiveItemSyncedMsg{date: "2024-09-26", err: os.ErrPermission})
	m = updated.(tuiModel)
	if cmd != nil || m.syncing || !strings.Contains(m.syncStatus, "Library sync failed for 2024-09-26") {
		t.Fatalf("fatal failure state: syncing=%v syncStatus=%q cmd=%v", m.syncing, m.syncStatus, cmd)
	}
}

func TestTUIModelReportsEmptyBackgroundSync(t *testing.T) {
	m := newTUIModel(nil, nil, "KEY")
	m.syncing = true

	updated, cmd := m.Update(archiveSyncPreparedMsg{plan: archiveSyncPlan{StartDate: "2024-09-27", EndDate: "2024-09-27"}})
	m = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("command is not nil for empty sync response")
	}
	if m.syncing {
		t.Fatal("syncing = true after empty sync response")
	}
	if m.syncStatus != "Library sync returned no APODs" {
		t.Fatalf("sync status = %q", m.syncStatus)
	}
}

func TestTUIManualSyncQueuesRefreshDuringCurrentRun(t *testing.T) {
	db, err := openLibrary(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := newTUIModel(nil, nil, "KEY")
	m.db = db
	m.syncContext, m.cancelSync = context.WithCancel(t.Context())
	m.commands = &commandTracker{}
	m.syncStarted = true
	m.syncTotal, m.syncCompleted, m.syncPreviewed, m.syncFailed = 7, 7, 3, 2
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m = updated.(tuiModel)
	if cmd == nil || !m.syncing || m.syncTotal != 0 || m.syncCompleted != 0 || m.syncPreviewed != 0 || m.syncFailed != 0 {
		t.Fatalf("manual sync did not start cleanly: syncing=%t counts=%d/%d/%d/%d", m.syncing, m.syncTotal, m.syncCompleted, m.syncPreviewed, m.syncFailed)
	}
	updated, cmd = m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m = updated.(tuiModel)
	if cmd != nil || !m.syncQueued || !strings.Contains(m.status, "queued") {
		t.Fatalf("overlapping refresh scheduled: cmd=%v status=%q", cmd, m.status)
	}
	updated, cmd = m.Update(archiveSyncPreparedMsg{plan: archiveSyncPlan{AlreadyUpToDate: true}})
	m = updated.(tuiModel)
	if !m.syncing || cmd == nil || m.syncQueued || !strings.Contains(m.syncStatus, "Checking") {
		t.Fatalf("queued sync did not start: syncing=%t status=%q cmd=%v", m.syncing, m.syncStatus, cmd)
	}
	updated, cmd = m.Update(archiveSyncPreparedMsg{plan: archiveSyncPlan{AlreadyUpToDate: true}})
	m = updated.(tuiModel)
	if cmd != nil || m.syncing || !strings.Contains(m.syncStatus, "up to date") {
		t.Fatalf("final no-op sync: syncing=%t status=%q", m.syncing, m.syncStatus)
	}
	updated, cmd = m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m = updated.(tuiModel)
	if cmd == nil || !m.syncing {
		t.Fatal("could not refresh again after completed runs")
	}
}

func TestTUIRetrySelectedOlderPreviewReusesExistingSyncAndKeepsSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("preview image"))
	}))
	defer server.Close()
	root := t.TempDir()
	previewDir := filepath.Join(root, "previews")
	if err := os.Mkdir(previewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := openLibrary(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	base := time.Date(2024, 9, 27, 0, 0, 0, 0, time.UTC)
	for i := range 32 {
		record := APODRecord{Date: base.AddDate(0, 0, -i).Format("2006-01-02"), Title: "APOD", FetchedAt: base}
		if i == 31 {
			record.PreviewError = "temporary failure"
			record.URL = server.URL + "/preview.jpg"
		}
		if err := upsertAPOD(db, record); err != nil {
			t.Fatal(err)
		}
	}
	m, err := newTUIModelFromLibrary(db, AppPaths{PreviewDir: previewDir}, "KEY", imageProtocolANSI, base)
	if err != nil {
		t.Fatal(err)
	}
	m.syncing = false
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	updated, _ = m.Update(archiveLoadedMsg{records: mustListAllAPODs(t, db)})
	m = updated.(tuiModel)
	oldDate := base.AddDate(0, 0, -31).Format("2006-01-02")
	selectListDate(&m.archiveList, oldDate)
	m.refreshDetail(true)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "p", Code: 'p'})
	m = updated.(tuiModel)
	if cmd == nil || !m.retryingPreview {
		t.Fatal("selected preview retry did not start")
	}
	updated, blocked := m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m = updated.(tuiModel)
	if blocked != nil || m.syncing || !m.syncQueued {
		t.Fatal("overlapping library sync started during preview retry")
	}
	var retry previewRetriedMsg
	for _, run := range cmd().(tea.BatchMsg) {
		if msg, ok := run().(previewRetriedMsg); ok {
			retry = msg
		}
	}
	if retry.err != nil || retry.date != oldDate {
		t.Fatalf("retry command result: %#v", retry)
	}
	updated, queued := m.Update(retry)
	m = updated.(tuiModel)
	if queued == nil || !m.syncing || m.syncQueued {
		t.Fatal("queued refresh did not start after preview retry")
	}
	if m.retryingPreview || m.selectedRecord().Date != oldDate || m.selectedRecord().PreviewError != "" || m.selectedRecord().PreviewPath == "" {
		t.Fatalf("retry did not update older selection: %#v", m.selectedRecord())
	}
	if _, err := os.Stat(m.selectedRecord().PreviewPath); err != nil {
		t.Fatalf("preview not cached: %v", err)
	}
	if !strings.Contains(m.status, "Preview refreshed") {
		t.Fatalf("success status: %q", m.status)
	}
}

func TestTUIRetryPreviewKeepsFailureAndRejectsStaleResult(t *testing.T) {
	db, err := openLibrary(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	record := APODRecord{Date: "2024-09-27", Title: "Broken", MediaType: "image", URL: "http://127.0.0.1:1/missing.jpg", PreviewError: "download failed", FetchedAt: time.Now()}
	if err := upsertAPOD(db, record); err != nil {
		t.Fatal(err)
	}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	m.db = db
	m.paths.PreviewDir = t.TempDir()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	updated, _ = m.Update(previewRetriedMsg{date: record.Date})
	m = updated.(tuiModel)
	if m.status == "Preview ready for 2024-09-27" {
		t.Fatal("stale retry changed status")
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "p", Code: 'p'})
	m = updated.(tuiModel)
	var retry previewRetriedMsg
	for _, run := range cmd().(tea.BatchMsg) {
		if msg, ok := run().(previewRetriedMsg); ok {
			retry = msg
		}
	}
	if retry.err != nil || retry.result.PreviewError == "" {
		t.Fatalf("retry command result: %#v", retry)
	}
	updated, _ = m.Update(retry)
	m = updated.(tuiModel)
	if m.retryingPreview || m.selectedRecord().PreviewError == "" || !strings.Contains(m.status, "still unavailable") {
		t.Fatalf("failed retry state: record=%#v status=%q", m.selectedRecord(), m.status)
	}
}

func TestTUIRefreshPreviewReplacesCachedImageWithoutStoredError(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = fmt.Fprintf(w, "preview %d", requests)
	}))
	defer server.Close()
	root := t.TempDir()
	previewDir := filepath.Join(root, "previews")
	if err := os.Mkdir(previewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := openLibrary(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	date := "2024-09-27"
	path := filepath.Join(previewDir, date+".jpg")
	if err := os.WriteFile(path, []byte("old preview"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := APODRecord{Date: date, Title: "Cached", MediaType: "image", URL: server.URL + "/preview.jpg", PreviewPath: path, FetchedAt: time.Now()}
	if err := upsertAPOD(db, record); err != nil {
		t.Fatal(err)
	}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	m.db = db
	m.paths.PreviewDir = previewDir
	m.imageProtocol = imageProtocolWezTerm
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	m.cacheANSIPreview(m.ansiPreviewKey(), ansiPreviewResult{preview: "old cached ANSI"})
	m.nativeImage = nativeImage{id: 42, path: path, protocol: imageProtocolWezTerm, width: m.previewArea.width, height: m.previewArea.height}
	m.nativeTarget = m.nativeImageKey()
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "p", Code: 'p'})
	m = updated.(tuiModel)
	if cmd == nil || !m.retryingPreview {
		t.Fatalf("p did not start refresh: status=%q", m.status)
	}
	var retry previewRetriedMsg
	for _, run := range cmd().(tea.BatchMsg) {
		if msg, ok := run().(previewRetriedMsg); ok {
			retry = msg
		}
	}
	if retry.err != nil || !retry.result.Previewed || requests != 1 {
		t.Fatalf("cached image not downloaded again: result=%#v requests=%d", retry, requests)
	}
	updated, _ = m.Update(retry)
	m = updated.(tuiModel)
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "preview 1" {
		t.Fatalf("preview contents=%q err=%v", contents, err)
	}
	if m.nativeImage.id != 0 || m.nativeRequest == "" || len(m.ansiCache) != 0 || !strings.Contains(m.status, "Preview refreshed") {
		t.Fatalf("stale preview still active: native=%d request=%q cache=%d status=%q", m.nativeImage.id, m.nativeRequest, len(m.ansiCache), m.status)
	}
}

func TestTUIRefreshPreviewReportsUnavailableURLAndTinyLayoutFeedback(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", Title: "Video", MediaType: "video"}}, nil, "KEY")
	m.db = &sql.DB{}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 35, Height: 12})
	m = updated.(tuiModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "p", Code: 'p'})
	m = updated.(tuiModel)
	if cmd != nil || !strings.Contains(ansi.Strip(m.View().Content), "No preview URL") {
		t.Fatalf("missing URL feedback invisible: status=%q view=%q", m.status, m.View().Content)
	}
}

func TestTUIRefreshPreviewFailureKeepsExistingCachedImage(t *testing.T) {
	root := t.TempDir()
	previewDir := filepath.Join(root, "previews")
	if err := os.Mkdir(previewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := openLibrary(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := filepath.Join(previewDir, "2024-09-27.jpg")
	if err := os.WriteFile(path, []byte("working preview"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := APODRecord{Date: "2024-09-27", Title: "Cached", MediaType: "image", URL: "http://127.0.0.1:1/unavailable.jpg", PreviewPath: path, FetchedAt: time.Now()}
	if err := upsertAPOD(db, record); err != nil {
		t.Fatal(err)
	}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	m.db = db
	m.paths.PreviewDir = previewDir
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "p", Code: 'p'})
	m = updated.(tuiModel)
	var retry previewRetriedMsg
	for _, run := range cmd().(tea.BatchMsg) {
		if msg, ok := run().(previewRetriedMsg); ok {
			retry = msg
		}
	}
	updated, _ = m.Update(retry)
	m = updated.(tuiModel)
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "working preview" {
		t.Fatalf("failed refresh damaged cache: contents=%q err=%v", contents, err)
	}
	if m.selectedRecord().PreviewPath != path || !strings.Contains(m.status, "still unavailable") {
		t.Fatalf("cached image or feedback lost: record=%#v status=%q", m.selectedRecord(), m.status)
	}
}

func TestTUIManualSyncRetriesPreviouslyFailedPreview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("recovered preview"))
	}))
	defer server.Close()
	root := t.TempDir()
	previewDir := filepath.Join(root, "previews")
	if err := os.Mkdir(previewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := openLibrary(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2024, 9, 27, 12, 0, 0, 0, time.UTC)
	record := APODRecord{Date: now.Format("2006-01-02"), Title: "Recover", MediaType: "image", URL: server.URL + "/preview.jpg", PreviewError: "old failure", FetchedAt: now}
	if err := upsertAPOD(db, record); err != nil {
		t.Fatal(err)
	}
	m, err := newTUIModelFromLibrary(db, AppPaths{PreviewDir: previewDir}, "KEY", imageProtocolANSI, now)
	if err != nil {
		t.Fatal(err)
	}
	m.syncing = false
	m.syncContext, m.cancelSync = context.WithCancel(t.Context())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "s", Code: 's'})
	m = updated.(tuiModel)
	plan, err := prepareArchiveSyncContext(m.syncContext, db, "KEY", now)
	if err != nil || len(plan.Items) != 1 || plan.Items[0].Date != record.Date {
		t.Fatalf("retry plan = %#v, err=%v", plan, err)
	}
	updated, cmd := m.Update(archiveSyncPreparedMsg{plan: plan})
	m = updated.(tuiModel)
	if cmd == nil || !m.syncing {
		t.Fatal("manual refresh did not start failed preview retry")
	}
	updated, _ = m.Update(cmd())
	m = updated.(tuiModel)
	if m.syncing || m.selectedRecord().PreviewError != "" || m.selectedRecord().PreviewPath == "" || m.syncPreviewed != 1 {
		t.Fatalf("manual retry did not recover: selected=%#v status=%q", m.selectedRecord(), m.syncStatus)
	}
}

func TestTUIRecoveredPreviewRefreshesWezTermPlacement(t *testing.T) {
	db, err := openLibrary(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	record := APODRecord{Date: "2024-09-27", Title: "Recovered", PreviewPath: "/preview.jpg", PreviewError: "old error", FetchedAt: time.Now()}
	if err := upsertAPOD(db, record); err != nil {
		t.Fatal(err)
	}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	m.db = db
	m.imageProtocol = imageProtocolWezTerm
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	m.nativeImage = nativeImage{id: 42, path: record.PreviewPath, protocol: imageProtocolWezTerm, width: m.previewArea.width, height: m.previewArea.height}
	m.nativeTarget = m.nativeImageKey()
	m.retryingPreview = true
	m.retryingDate = record.Date
	if _, err := db.Exec(`UPDATE apods SET preview_error = '' WHERE date = ?`, record.Date); err != nil {
		t.Fatal(err)
	}
	updated, cmd := m.Update(previewRetriedMsg{date: record.Date, result: itemSyncResult{Previewed: true}})
	m = updated.(tuiModel)
	if cmd == nil || m.nativeImage.id != 0 || m.nativeRequest == "" || m.selectedRecord().PreviewError != "" {
		t.Fatalf("WezTerm image was not refreshed: id=%d request=%q error=%q", m.nativeImage.id, m.nativeRequest, m.selectedRecord().PreviewError)
	}
}

func TestAPODListItemShowsPreviewError(t *testing.T) {
	item := apodListItem{record: APODRecord{Date: "2024-09-27", PreviewError: "download timed out"}}
	if got := item.Description(); got != "2024-09-27 • preview error" {
		t.Fatalf("Description() = %q", got)
	}
	m := newTUIModel([]APODRecord{item.record}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	if got := ansi.Strip(m.detail.View()); !strings.Contains(got, "Preview error: download timed out") {
		t.Fatalf("detail description = %q", got)
	}
}

func TestTUIModelTracksSideEffectCommands(t *testing.T) {
	m := newTUIModel(nil, nil, "KEY")
	m.commands = &commandTracker{}
	finished := false

	cmd := m.trackCmd(func() tea.Msg {
		finished = true
		return nil
	})
	cmd()
	m.commands.closeAndWait()
	if !finished {
		t.Fatal("tracked command did not run")
	}
}

func TestCommandTrackerRejectsCommandsAfterShutdown(t *testing.T) {
	tracker := &commandTracker{}
	tracker.closeAndWait()
	runs := 0
	msg := tracker.run(func() tea.Msg {
		runs++
		return nil
	})
	if msg != nil || runs != 0 {
		t.Fatalf("late command returned %v and ran %d times", msg, runs)
	}
}

func TestEnsureHDImageCached_ReusesExistingFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}
	db, err := openLibrary(paths.DBPath)
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()

	hdPath := filepath.Join(paths.FullDir, "2024-09-27.jpg")
	if err := os.WriteFile(hdPath, []byte("cached-full"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	record := APODRecord{Date: "2024-09-27", Title: "Cached", HDPath: hdPath}
	got, err := ensureHDImageCached(db, paths, record, "KEY")
	if err != nil {
		t.Fatalf("ensureHDImageCached() error: %v", err)
	}
	if got != hdPath {
		t.Fatalf("ensureHDImageCached() = %q, want %q", got, hdPath)
	}
}

func TestEnsureHDImageCached_UsesStoredHDPathFromDatabase(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}
	db, err := openLibrary(paths.DBPath)
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()

	hdPath := filepath.Join(paths.FullDir, "2024-09-27.jpg")
	if err := os.WriteFile(hdPath, []byte("cached-full"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if err := upsertAPOD(db, APODRecord{
		Date:        "2024-09-27",
		Title:       "Stored",
		Description: "Stored item.",
		MediaType:   "image",
		URL:         "https://example.com/stored.jpg",
		HDPath:      hdPath,
		FetchedAt:   time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("upsertAPOD() error: %v", err)
	}

	originalBaseURL := apodAPIBaseURL
	apodAPIBaseURL = "http://127.0.0.1:1/planetary/apod"
	defer func() {
		apodAPIBaseURL = originalBaseURL
	}()

	staleRecord := APODRecord{Date: "2024-09-27", Title: "Stored"}
	got, err := ensureHDImageCached(db, paths, staleRecord, "KEY")
	if err != nil {
		t.Fatalf("ensureHDImageCached() error: %v", err)
	}
	if got != hdPath {
		t.Fatalf("ensureHDImageCached() = %q, want %q", got, hdPath)
	}
}

func TestToggleFavoriteCmdUpdatesState(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}
	db, err := openLibrary(paths.DBPath)
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()

	if err := upsertAPOD(db, APODRecord{
		Date:        "2024-09-27",
		Title:       "Favorite me",
		Description: "Stored item.",
		MediaType:   "image",
		URL:         "https://example.com/stored.jpg",
		FetchedAt:   time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("upsertAPOD() error: %v", err)
	}

	records, err := listRecentAPODs(db, 30)
	if err != nil {
		t.Fatalf("listRecentAPODs() error: %v", err)
	}
	model := newTUIModel(records, nil, "KEY")
	model.db = db
	model.paths = paths

	msg := toggleFavoriteCmd(db, "2024-09-27")().(favoriteToggledMsg)
	if msg.err != nil {
		t.Fatalf("toggleFavoriteCmd() error: %v", msg.err)
	}
	if !msg.favorite {
		t.Fatal("msg.favorite = false, want true")
	}

	updatedModel, _ := model.Update(msg)
	model = updatedModel.(tuiModel)
	if !model.selectedRecord().Favorite {
		t.Fatal("selectedRecord().Favorite = false, want true")
	}
}

func TestTUIModelTabSwitchesToFavoritesPane(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent", Description: "Recent item.", MediaType: "image", URL: "https://example.com/recent.jpg"}}
	favorites := []APODRecord{{Date: "2024-08-01", Title: "Favorite", Description: "Favorite item.", MediaType: "image", URL: "https://example.com/favorite.jpg", Favorite: true}}

	m := newTUIModel(recent, favorites, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{})
	_ = updated

	updated, _ = m.Update(tea.KeyPressMsg{Text: "tab"})
	m = updated.(tuiModel)
	if m.activePane != favoritesPane {
		t.Fatalf("activePane = %v, want favoritesPane", m.activePane)
	}
	if m.favoriteList.Title != "Favorites • active" {
		t.Fatalf("favoriteList.Title = %q, want Favorites • active", m.favoriteList.Title)
	}
	if m.recentList.Title != "Recent APODs" {
		t.Fatalf("recentList.Title = %q, want Recent APODs", m.recentList.Title)
	}
	if got := m.selectedRecord().Title; got != "Favorite" {
		t.Fatalf("selected title = %q, want Favorite", got)
	}
}

func TestTUIModelTabSwitchesPaneByKeyCode(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent", Description: "Recent item.", MediaType: "image", URL: "https://example.com/recent.jpg"}}
	favorites := []APODRecord{{Date: "2024-08-01", Title: "Favorite", Description: "Favorite item.", MediaType: "image", URL: "https://example.com/favorite.jpg", Favorite: true}}

	m := newTUIModel(recent, favorites, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(tuiModel)
	if m.activePane != favoritesPane {
		t.Fatalf("activePane after KeyTab = %v, want favoritesPane", m.activePane)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = updated.(tuiModel)
	if m.activePane != recentPane {
		t.Fatalf("activePane after Shift+Tab = %v, want recentPane", m.activePane)
	}
	if m.recentList.Title != "Recent APODs • active" {
		t.Fatalf("recentList.Title = %q, want Recent APODs • active", m.recentList.Title)
	}
}

func TestTUIModelShowsSpinnerWhileLoading(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent", Description: "Recent item.", MediaType: "image", URL: "https://example.com/recent.jpg"}}

	m := newTUIModel(recent, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	m.loading = true
	m.status = "Setting wallpaper for Recent…"

	updated, cmd := m.Update(m.spinner.Tick())
	m = updated.(tuiModel)
	if cmd == nil {
		t.Fatal("spinner tick should schedule the next frame")
	}
	if !strings.Contains(m.View().Content, "Setting wallpaper for Recent…") {
		t.Fatalf("view = %q, want loading status", m.View().Content)
	}
	if !strings.Contains(m.View().Content, m.spinner.View()) {
		t.Fatalf("view = %q, want spinner frame", m.View().Content)
	}

	updated, _ = m.Update(wallpaperAppliedMsg{date: "2024-09-27", title: "Recent", path: "/tmp/recent.jpg"})
	m = updated.(tuiModel)
	if m.loading {
		t.Fatal("loading = true, want false after wallpaperAppliedMsg")
	}
}

func TestTUIModelOpensApodPageURL(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent", Description: "Recent item.", MediaType: "image", URL: "https://example.com/recent.jpg"}}
	m := newTUIModel(recent, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)

	var opened string
	oldOpen := openURLFunc
	defer func() { openURLFunc = oldOpen }()
	openURLFunc = func(url string) error {
		opened = url
		return nil
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "o", Code: 'o'})
	m = updated.(tuiModel)
	if cmd == nil {
		t.Fatal("open page should return a command")
	}
	msg := cmd().(urlOpenedMsg)
	updated, _ = m.Update(msg)
	m = updated.(tuiModel)

	if opened != apodPageURL("2024-09-27") {
		t.Fatalf("opened URL = %q", opened)
	}
	if !strings.Contains(m.status, "Opened APOD page") {
		t.Fatalf("status = %q", m.status)
	}
}

func TestTUIModelOpensMediaURL(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent", Description: "Recent item.", MediaType: "image", URL: "https://example.com/recent.jpg", HDURL: "https://example.com/hd.jpg"}}
	m := newTUIModel(recent, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)

	var opened string
	oldOpen := openURLFunc
	defer func() { openURLFunc = oldOpen }()
	openURLFunc = func(url string) error {
		opened = url
		return nil
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "u", Code: 'u'})
	m = updated.(tuiModel)
	if cmd == nil {
		t.Fatal("open media should return a command")
	}
	msg := cmd().(urlOpenedMsg)
	updated, _ = m.Update(msg)
	m = updated.(tuiModel)

	if opened != "https://example.com/hd.jpg" {
		t.Fatalf("opened URL = %q, want HD URL", opened)
	}
	if !strings.Contains(m.status, "Opened media URL") {
		t.Fatalf("status = %q", m.status)
	}
}

func TestTUIModelTogglesHelpView(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent", Description: "Recent item.", MediaType: "image", URL: "https://example.com/recent.jpg"}}

	m := newTUIModel(recent, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)

	updated, _ = m.Update(tea.KeyPressMsg{Text: "?"})
	m = updated.(tuiModel)
	if !m.showHelp {
		t.Fatal("showHelp = false, want true")
	}
	view := m.View().Content
	if !strings.Contains(view, "Keybindings") {
		t.Fatalf("view = %q, want help title", view)
	}
	if !strings.Contains(view, "Tab                Switch to the next pane") {
		t.Fatalf("view = %q, want tab binding text", view)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Text: "?"})
	m = updated.(tuiModel)
	if m.showHelp {
		t.Fatal("showHelp = true, want false after toggle")
	}
}

func TestTUIModelEscClosesHelpView(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent", Description: "Recent item.", MediaType: "image", URL: "https://example.com/recent.jpg"}}

	m := newTUIModel(recent, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	m.showHelp = true

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(tuiModel)
	if m.showHelp {
		t.Fatal("showHelp = true, want false after Esc")
	}
}

func TestTUIModelEscClosesHelpBeforeClearingSearch(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}, {Title: "Galaxy"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	m = typeSearchQuery(t, m, "Galaxy")
	m.showHelp = true

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(tuiModel)
	if m.showHelp || !m.recentList.IsFiltered() {
		t.Fatalf("showHelp = %t, filtered = %t", m.showHelp, m.recentList.IsFiltered())
	}
}

func TestTUIModelSearchesTitleDateAndDescription(t *testing.T) {
	records := []APODRecord{
		{Date: "2024-09-27", Title: "Aurora", Description: "Lights over Iceland."},
		{Date: "2024-09-26", Title: "Galaxy", Description: "A distant spiral nebula."},
	}

	for _, query := range []string{"Galaxy", "2024-09-26", "nebula"} {
		m := newTUIModel(records, nil, "KEY")
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		m = updated.(tuiModel)
		m = typeSearchQuery(t, m, query)
		if got := m.selectedRecord().Title; got != "Galaxy" {
			t.Fatalf("query %q selected %q, want Galaxy", query, got)
		}
		if len(m.recentList.VisibleItems()) != 1 {
			t.Fatalf("query %q has %d visible items", query, len(m.recentList.VisibleItems()))
		}
	}
}

func TestTUIModelSearchIsIndependentPerPane(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent Aurora"}, {Date: "2024-09-26", Title: "Recent Galaxy"}}
	favorites := []APODRecord{{Date: "2024-09-25", Title: "Favorite Moon", Favorite: true}, {Date: "2024-09-24", Title: "Favorite Nebula", Favorite: true}}
	m := newTUIModel(recent, favorites, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	m = typeSearchQuery(t, m, "Galaxy")

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(tuiModel)
	m = typeSearchQuery(t, m, "Moon")
	if m.recentList.FilterValue() != "Galaxy" || m.favoriteList.FilterValue() != "Moon" {
		t.Fatalf("recent query = %q, favorite query = %q", m.recentList.FilterValue(), m.favoriteList.FilterValue())
	}
}

func TestTUIModelEscapeClearsAppliedSearch(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}, {Title: "Galaxy"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	m = typeSearchQuery(t, m, "Galaxy")

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(tuiModel)
	if m.recentList.IsFiltered() || len(m.recentList.VisibleItems()) != 2 {
		t.Fatalf("filtered = %t, visible = %d", m.recentList.IsFiltered(), len(m.recentList.VisibleItems()))
	}
}

func TestTUIModelSearchTitleShowsQueryAndMatchCount(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}, {Title: "Galaxy"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	m = typeSearchQuery(t, m, "Galaxy")
	if !strings.Contains(m.recentList.Title, `"Galaxy" (1)`) {
		t.Fatalf("recent title = %q", m.recentList.Title)
	}
}

func TestTUIModelSearchAcceptsOnlyEnter(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}, {Title: "Galaxy"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "Galaxy"})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(tuiModel)
	if !m.recentList.SettingFilter() || m.activePane != recentPane {
		t.Fatalf("filtering = %t, pane = %v", m.recentList.SettingFilter(), m.activePane)
	}
}

func TestTUIModelCanApplySearchWithNoMatches(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}, {Title: "Galaxy"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	m = typeSearchQuery(t, m, "Saturn")
	if !m.recentList.IsFiltered() || len(m.recentList.VisibleItems()) != 0 {
		t.Fatalf("filtered = %t, visible = %d", m.recentList.IsFiltered(), len(m.recentList.VisibleItems()))
	}
}

func TestTUIModelEmptySearchResetsFilter(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}, {Title: "Galaxy"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(tuiModel)
	if m.recentList.IsFiltered() || m.recentList.FilterValue() != "" {
		t.Fatalf("filtered = %t, query = %q", m.recentList.IsFiltered(), m.recentList.FilterValue())
	}
}

func TestTUIModelStartingEmptySearchKeepsAllItemsVisible(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}, {Title: "Galaxy"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = updated.(tuiModel)
	if len(m.recentList.VisibleItems()) != 2 {
		t.Fatalf("visible items = %d, want 2", len(m.recentList.VisibleItems()))
	}
}

func TestTUIModelBackspaceToEmptySearchKeepsAllItemsVisible(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}, {Title: "Galaxy"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "A", Code: 'A'})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = updated.(tuiModel)
	if m.recentList.FilterValue() != "" || len(m.recentList.VisibleItems()) != 2 {
		t.Fatalf("query = %q, visible = %d", m.recentList.FilterValue(), len(m.recentList.VisibleItems()))
	}
}

func TestTUIModelPasteMessageUpdatesSearchSynchronously(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}, {Title: "Galaxy"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.PasteMsg{Content: "Galaxy"})
	m = updated.(tuiModel)
	if m.recentList.FilterValue() != "Galaxy" || len(m.recentList.VisibleItems()) != 1 {
		t.Fatalf("query = %q, visible = %d", m.recentList.FilterValue(), len(m.recentList.VisibleItems()))
	}
}

func TestTUIModelRebuildDuringEmptySearchKeepsAllItemsVisible(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = updated.(tuiModel)
	m.recentRecords = append(m.recentRecords, APODRecord{Title: "Galaxy"})
	m.syncListItems()
	if len(m.recentList.VisibleItems()) != 2 {
		t.Fatalf("visible items = %d, want 2", len(m.recentList.VisibleItems()))
	}
}

func TestTUIModelCtrlCQuitsWhileSearching(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}}, nil, "KEY")
	updated, _ := m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = updated.(tuiModel)
	_, cmd := m.Update(tea.KeyPressMsg{Text: "ctrl+c", Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+C command = nil while searching")
	}
}

func TestTUIModelFilteredRebuildClampsVisibleSelection(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "1", Title: "Galaxy One"}, {Date: "2", Title: "Galaxy Two"}, {Date: "3", Title: "Aurora"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	m = typeSearchQuery(t, m, "Galaxy")
	m.recentList.Select(1)
	m.recentRecords = []APODRecord{{Date: "1", Title: "Galaxy One"}, {Date: "3", Title: "Aurora"}, {Date: "4", Title: "Moon"}}
	m.syncListItems()
	if got := m.selectedRecord().Date; got != "1" {
		t.Fatalf("selected date = %q after filtered rebuild", got)
	}
}

func TestTUIModelSearchUpdatesOneCharacterAtATime(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}, {Title: "Galaxy"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = updated.(tuiModel)
	for _, character := range "Galaxy" {
		updated, _ = m.Update(tea.KeyPressMsg{Text: string(character), Code: character})
		m = updated.(tuiModel)
	}
	if m.recentList.FilterValue() != "Galaxy" || len(m.recentList.VisibleItems()) != 1 {
		t.Fatalf("query = %q, visible = %d", m.recentList.FilterValue(), len(m.recentList.VisibleItems()))
	}
}

func TestTUIModelNativePreviewStaysDisabledWhileSearching(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora", PreviewPath: "/preview.jpg"}}, nil, "KEY")
	m.imageProtocol = imageProtocolWezTerm
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = updated.(tuiModel)
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "A", Code: 'A'})
	m = updated.(tuiModel)
	if m.nativeImageWanted() || m.nativeRequest != "" || cmd == nil {
		t.Fatalf("wanted = %t, request = %q, command = %v", m.nativeImageWanted(), m.nativeRequest, cmd)
	}
}

func TestTUIModelCancelSearchRestoresPreviousQuery(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}, {Title: "Galaxy"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	m = typeSearchQuery(t, m, "Galaxy")
	updated, _ = m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Text: "X", Code: 'X'})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(tuiModel)
	if m.recentList.FilterValue() != "Galaxy" || !m.recentList.IsFiltered() {
		t.Fatalf("query = %q, filtered = %t", m.recentList.FilterValue(), m.recentList.IsFiltered())
	}
}

func TestTUIModelSyncListItemsPreservesAppliedSearch(t *testing.T) {
	m := newTUIModel([]APODRecord{{Title: "Aurora"}, {Title: "Galaxy"}}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	m = typeSearchQuery(t, m, "Galaxy")
	m.recentRecords = append(m.recentRecords, APODRecord{Title: "Another Galaxy"})
	m.syncListItems()

	if !m.recentList.IsFiltered() || len(m.recentList.VisibleItems()) != 2 {
		t.Fatalf("filtered = %t, visible = %d", m.recentList.IsFiltered(), len(m.recentList.VisibleItems()))
	}
}

func typeSearchQuery(t *testing.T, m tuiModel, query string) tuiModel {
	t.Helper()
	updated, _ := m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = updated.(tuiModel)
	if !m.activeList().SettingFilter() {
		t.Fatal("active list is not editing a search")
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Text: query})
	m = updated.(tuiModel)
	_ = cmd
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return updated.(tuiModel)
}

func TestListFavoriteAPODsReturnsPersistentFavorites(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}
	db, err := openLibrary(paths.DBPath)
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()

	for _, record := range []APODRecord{
		{Date: "2024-09-27", Title: "Recent Favorite", Description: "Recent favorite.", MediaType: "image", URL: "https://example.com/recent.jpg", Favorite: true, FetchedAt: time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC)},
		{Date: "2024-07-01", Title: "Old Favorite", Description: "Older favorite.", MediaType: "image", URL: "https://example.com/old.jpg", Favorite: true, FetchedAt: time.Date(2024, 7, 1, 9, 0, 0, 0, time.UTC)},
		{Date: "2024-09-26", Title: "Not Favorite", Description: "Normal item.", MediaType: "image", URL: "https://example.com/normal.jpg", Favorite: false, FetchedAt: time.Date(2024, 9, 26, 9, 0, 0, 0, time.UTC)},
	} {
		if err := upsertAPOD(db, record); err != nil {
			t.Fatalf("upsertAPOD(%s) error: %v", record.Date, err)
		}
	}

	favorites, err := listFavoriteAPODs(db)
	if err != nil {
		t.Fatalf("listFavoriteAPODs() error: %v", err)
	}
	if len(favorites) != 2 {
		t.Fatalf("len(favorites) = %d, want 2", len(favorites))
	}
	if favorites[0].Date != "2024-09-27" || favorites[1].Date != "2024-07-01" {
		t.Fatalf("favorite dates = [%s, %s], want [2024-09-27, 2024-07-01]", favorites[0].Date, favorites[1].Date)
	}
}
