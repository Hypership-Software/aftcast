//go:build windows

package install

import (
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ensurePathEntry adds dir to the user PATH (HKCU\Environment) if absent, preserving
// the value's REG_EXPAND_SZ type so %VAR% references survive a rewrite, then
// broadcasts WM_SETTINGCHANGE so new shells see it without a logout.
func ensurePathEntry(dir string) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()
	cur, valType, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return false, err
	}
	next, changed := addToPath(cur, dir)
	if !changed {
		return false, nil
	}
	if err := setPath(k, next, valType); err != nil {
		return false, err
	}
	broadcastEnvChange()
	return true, nil
}

func removePathEntry(dir string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	cur, valType, err := k.GetStringValue("Path")
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	if err := setPath(k, removeFromPath(cur, dir), valType); err != nil {
		return err
	}
	broadcastEnvChange()
	return nil
}

func setPath(k registry.Key, value string, valType uint32) error {
	if valType == registry.EXPAND_SZ {
		return k.SetExpandStringValue("Path", value)
	}
	return k.SetStringValue("Path", value)
}

func broadcastEnvChange() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	if err := proc.Find(); err != nil {
		return
	}
	var result uintptr
	proc.Call(uintptr(hwndBroadcast), uintptr(wmSettingChange), 0,
		uintptr(unsafe.Pointer(env)), uintptr(smtoAbortIfHung), 5000,
		uintptr(unsafe.Pointer(&result)))
}
