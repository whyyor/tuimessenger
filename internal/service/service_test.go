package service

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestLaunchdPlist(t *testing.T) {
	plist := launchdPlist(Spec{Executable: "/Users/a b/.local/opt/tuimessenger/tuimessenger", DataDir: "/data & more", LogDir: "/logs"})
	// It must be valid XML, with paths escaped.
	if err := xml.Unmarshal([]byte(plist), new(struct{})); err != nil {
		t.Fatalf("not valid XML: %v\n%s", err, plist)
	}
	for _, want := range []string{
		"<string>" + Label + "</string>",
		"<string>/Users/a b/.local/opt/tuimessenger/tuimessenger</string>",
		"<string>-data</string>", "<string>/data &amp; more</string>",
		"<key>SuccessfulExit</key>\n\t\t<false/>", "<string>/logs/tuimessenger.log</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist lacks %q:\n%s", want, plist)
		}
	}
	if strings.Contains(plist, "-network") {
		t.Error("networks come from config.yaml, not the command line")
	}
}

func TestSystemdUnit(t *testing.T) {
	unit := systemdUnit(Spec{Executable: "/home/pi/bin/tuimessenger", DataDir: `/home/pi/my "data" 100%`})
	for _, want := range []string{
		`ExecStart="/home/pi/bin/tuimessenger" -data "/home/pi/my \"data\" 100%%"`,
		"Restart=on-failure", "WantedBy=default.target",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit lacks %q:\n%s", want, unit)
		}
	}
}

func TestRunsDataDir(t *testing.T) {
	def := "/Users/me/Library/Application Support/tuimessenger"
	handMade := "<array><string>/bin/tuimessenger</string><string>-network</string><string>whatsapp</string></array>"
	if !runsDataDir(handMade, def+"/", def) || runsDataDir(handMade, "/tmp/test", def) {
		t.Error("a service without -data runs the default folder, only that")
	}
	plist := launchdPlist(Spec{Executable: "/bin/tuimessenger", DataDir: def, LogDir: "/logs"})
	if !runsDataDir(plist, def, def) || runsDataDir(plist, "/tmp/test", def) || runsDataDir(plist, "/Users/me/Library/Application", def) {
		t.Error("plist with -data")
	}
	unit := systemdUnit(Spec{Executable: "/bin/tuimessenger", DataDir: "/home/pi/data"})
	if !runsDataDir(unit, "/home/pi/data", def) || runsDataDir(unit, "/home/pi/dat", def) {
		t.Error("systemd unit with -data")
	}
}
