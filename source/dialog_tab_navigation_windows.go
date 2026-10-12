//go:build windows

package main

// moveDialogTabFocus supplies the dialog-manager Tab behaviour to dotPaint's
// custom modal windows. They are ordinary top-level windows rather than
// resources created by CreateDialog, so WS_TABSTOP alone does not move focus.
// GetNextDlgTabItem follows child creation order and wraps at both ends.
func moveDialogTabFocus(dialog uintptr, reverse bool) bool {
	if dialog == 0 {
		return false
	}
	current, _, _ := procGetFocus.Call()
	previous := uintptr(0)
	if reverse {
		previous = 1
	}
	next, _, _ := procGetNextDlgTabItem.Call(dialog, current, previous)
	if next == 0 || next == current {
		return false
	}
	procSetFocus.Call(next)
	if class := getWindowClassNameString(next); class == "Edit" || class == "EDIT" {
		// Native dialogs select the complete numeric value when focus arrives by
		// Tab. Preserve that convenient overwrite behaviour in these editors.
		sendMessage(next, EM_SETSEL, 0, ^uintptr(0))
	}
	return true
}
