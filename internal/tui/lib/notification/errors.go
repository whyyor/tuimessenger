package notification

// NotAuthorizedError means the system doesn't let this notifier app show notifications (on macOS: the permission
// prompt was declined or closed, which macOS records as "Don't Allow").
type NotAuthorizedError struct {
	// App is the notifier app's name as System Settings → Notifications lists it.
	App string
}

func (e *NotAuthorizedError) Error() string {
	return "notifications aren't allowed for " + e.App
}
