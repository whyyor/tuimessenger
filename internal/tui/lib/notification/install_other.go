//go:build !darwin

package notification

// NotifiersUpToDate is always true off macOS: there are no notifier apps to build.
func NotifiersUpToDate() bool { return true }

// InstallNotifiers does nothing off macOS.
func InstallNotifiers() error { return nil }
