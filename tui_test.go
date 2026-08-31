package main

import (
	"context"
	"errors"
	"fmt"
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

func TestTUIModelANSIProtocolDoesNotRequestNativePreview(t *testing.T) {
	m := newTUIModel([]APODRecord{{Date: "2024-09-27", PreviewPath: "/preview.jpg"}}, nil, "KEY")
	m.imageProtocol = imageProtocolANSI
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	if cmd != nil || m.nativeRequest != "" {
		t.Fatalf("ANSI resize command = %v, native request = %q", cmd, m.nativeRequest)
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

func TestTUIModelRetriesNativePreviewAfterPreparationError(t *testing.T) {
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
	if cmd := m.requestNativeImage(); cmd == nil {
		t.Fatal("native retry command = nil")
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
	if x <= 0 || y <= 0 || favoriteY != y+1 {
		t.Fatalf("normal position = %d,%d; favorite y = %d", x, y, favoriteY)
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
	if wrappedY+m.previewArea.height > verticalOuterInset+m.detailStyle.GetBorderTopSize()+m.detail.Height() {
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

		contentHeight := size.Height - statusLineCount - verticalOuterInset*2 - verticalInterPaneGap*2
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

	if strings.TrimSpace(lines[0]) != "" {
		t.Fatalf("top inset = %q, want blank", lines[0])
	}
	if !strings.HasPrefix(lines[verticalOuterInset], strings.Repeat(" ", horizontalOuterInset)+"╔") {
		t.Fatalf("first pane line = %q, want %d-cell left inset", lines[verticalOuterInset], horizontalOuterInset)
	}
	if strings.TrimSpace(lines[len(lines)-1]) != "" {
		t.Fatalf("bottom inset = %q, want blank", lines[len(lines)-1])
	}

	detailBottom := verticalOuterInset + lipgloss.Height(m.detailStyle.Render(m.detail.View()))
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
	if !strings.Contains(model.status, "Synced 2 APODs") {
		t.Fatalf("status = %q", model.status)
	}
}

func TestTUIModelStopsBackgroundSyncAfterItemFailure(t *testing.T) {
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

	updated, cmd := m.Update(archiveItemSyncedMsg{date: "2024-09-26", err: os.ErrPermission})
	m = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("next item command is not nil after item failure")
	}
	if m.syncing {
		t.Fatal("syncing = true after item failure")
	}
	if !strings.Contains(m.status, "Library sync failed for 2024-09-26") {
		t.Fatalf("status = %q", m.status)
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
	if m.status != "Library sync returned no APODs" {
		t.Fatalf("status = %q", m.status)
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
