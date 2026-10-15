//go:build windows

package main

import (
	"strings"
	"syscall"
	"unsafe"
)

// dotPaint 1.3.6: language is a view preference, not a document operation.
// Neither the canvas nor the undo/redo history is touched, and the process is
// not restarted. Only labels, menus and localized option lists are refreshed.
func (a *App) switchDotPaintLanguage(edition string) {
	if edition != "ko" && edition != "en" || edition == dotPaintEdition {
		return
	}
	previous := dotPaintEdition
	dotPaintEdition = edition

	// Rebuild only the menu. This path is also used for the live theme switch;
	// it does not reinitialize App, alter any document state, or close editors.
	a.rebuildMenuForTheme()
	a.updateTitle()

	// Change labels in existing modeless dialogs without touching EDIT / RichEdit
	// controls: text, expressions, paths and user-entered values must survive.
	roots := []uintptr{a.hwnd}
	if s := activeFormulaEditor; s != nil {
		roots = append(roots, s.hwnd)
	}
	if s := activeFunctionGraphEditor; s != nil {
		roots = append(roots, s.hwnd)
	}
	if s := activeMultidimShapeEditor; s != nil {
		roots = append(roots, s.hwnd)
	}
	if s := activeCircuitLibrary; s != nil {
		roots = append(roots, s.hwnd)
	}
	if s := activeCustomImageLibrary; s != nil {
		roots = append(roots, s.hwnd)
	}
	if s := activeHelpWindow; s != nil {
		roots = append(roots, s.hwnd)
	}
	if s := activeAboutWindow; s != nil {
		roots = append(roots, s.hwnd)
	}
	for _, root := range roots {
		if root == 0 {
			continue
		}
		if r, _, _ := procIsWindow.Call(root); r == 0 {
			continue
		}
		if root != a.hwnd {
			refreshLocalizedWindowCaption(root, previous, edition)
		}
		refreshLocalizedChildControls(root, previous, edition)
		procInvalidateRect.Call(root, 0, 1)
	}
	if err := saveUILanguagePreference(edition); err != nil {
		logf("LIVE_LANGUAGE preference save failed: %v", err)
	}
	logf("LIVE_LANGUAGE %s -> %s; canvas and edit history preserved", previous, edition)
	procDrawMenuBar.Call(a.hwnd)
	procInvalidateRect.Call(a.hwnd, 0, 1)
}

func translatedLabelForLanguage(s, source, target string) string {
	if s == "" || source == target {
		return s
	}
	if source == "ko" && target == "en" {
		if en, ok := uiTranslations[s]; ok {
			return en
		}
		return s // not a translatable label: do not rewrite user data
	}
	if source == "en" && target == "ko" {
		uiReverseOnce.Do(func() {
			uiReverse = make(map[string]string, len(uiTranslations))
			for ko, en := range uiTranslations {
				if _, ok := uiReverse[en]; !ok {
					uiReverse[en] = ko
				}
			}
		})
		if ko, ok := uiReverse[s]; ok {
			return ko
		}
	}
	return s
}

func rawWindowLabel(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if n == 0 || n > 8192 {
		return ""
	}
	chars := make([]uint16, int(n)+2)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&chars[0])), uintptr(len(chars)))
	return syscall.UTF16ToString(chars)
}

func refreshLocalizedWindowCaption(hwnd uintptr, source, target string) {
	text := rawWindowLabel(hwnd)
	next := translatedLabelForLanguage(text, source, target)
	if text != next {
		procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(rawUTF16Ptr(next))))
	}
}

func refreshLocalizedChildControls(root uintptr, source, target string) {
	// Do not change a window's controls from inside EnumChildWindows' callback:
	// some controls notify their owner while resetting option lists.
	var handles []uintptr
	callback := syscall.NewCallback(func(hwnd, param uintptr) uintptr {
		handles = append(handles, hwnd)
		return 1
	})
	procEnumChildWindows.Call(root, callback, 0)
	for _, hwnd := range handles {
		if r, _, _ := procIsWindow.Call(hwnd); r == 0 {
			continue
		}
		switch strings.ToUpper(getWindowClassNameString(hwnd)) {
		case "BUTTON", "STATIC":
			refreshLocalizedWindowCaption(hwnd, source, target)
		case "COMBOBOX":
			refreshLocalizedComboChoices(hwnd, source, target)
		}
	}
}

func refreshLocalizedComboChoices(hwnd uintptr, source, target string) {
	// Only pure selection combo boxes can be safely reconstructed. Editable
	// combo boxes may contain unsaved text and must not be modified.
	style, _, _ := procGetWindowLongW.Call(hwnd, uintptr(GWL_STYLE))
	if style&3 != CBS_DROPDOWNLIST || style&CBS_HASSTRINGS == 0 {
		return
	}
	const cbGetCount = 0x0146
	const cbResetContent = 0x014B
	n := int(int32(sendMessage(hwnd, cbGetCount, 0, 0)))
	if n <= 0 || n > 2000 {
		return
	}
	values := make([]string, n)
	anyChange := false
	for i := range values {
		old := comboItemText(hwnd, i)
		values[i] = translatedLabelForLanguage(old, source, target)
		if values[i] != old {
			anyChange = true
		}
	}
	if !anyChange {
		return
	}
	selected := int32(sendMessage(hwnd, CB_GETCURSEL, 0, 0))
	sendMessage(hwnd, cbResetContent, 0, 0)
	for _, value := range values {
		sendMessage(hwnd, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(rawUTF16Ptr(value))))
	}
	if selected >= 0 && int(selected) < n {
		sendMessage(hwnd, CB_SETCURSEL, uintptr(selected), 0)
	}
	procInvalidateRect.Call(hwnd, 0, 1)
}

// Open modeless editors keep their own 11/12px font handle. Rebind only controls
// that use that shared chrome font, preserving specialized preview/text fonts.
func (a *App) refreshLocalizedEditorFonts() {
	update := func(root uintptr, old *uintptr) {
		if root == 0 || old == nil || *old == 0 {
			return
		}
		next := createCompactEditorUIFont()
		if next == 0 {
			return
		}
		prev := *old
		var handles []uintptr
		callback := syscall.NewCallback(func(hwnd, param uintptr) uintptr {
			handles = append(handles, hwnd)
			return 1
		})
		procEnumChildWindows.Call(root, callback, 0)
		for _, hwnd := range handles {
			if sendMessage(hwnd, WM_GETFONT, 0, 0) == prev {
				sendMessage(hwnd, WM_SETFONT, next, 1)
			}
		}
		*old = next
		procDeleteObject.Call(prev)
	}
	if s := activeFormulaEditor; s != nil {
		update(s.hwnd, &s.uiFont)
	}
	if s := activeFunctionGraphEditor; s != nil {
		update(s.hwnd, &s.uiFont)
	}
	if s := activeMultidimShapeEditor; s != nil {
		update(s.hwnd, &s.uiFont)
	}
}
