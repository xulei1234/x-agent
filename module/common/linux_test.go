package common

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// isolate viper global state for tests
func setupViperForLinuxTests() func() {
	old := viper.New()
	*viper.GetViper() = *old

	// defaults used by linux.go (keep minimal)
	viper.Set("UUID", "")
	viper.Set("IP", "")
	viper.Set("HostName", "")
	viper.Set("IDC.Zone", "")

	return func() {
		*viper.GetViper() = *viper.New()
	}
}

func TestParseUUIDFromDMIDecode_ExtractsUUID(t *testing.T) {
	// typical dmidecode output fragment
	in := `
System Information
	Manufacturer: Example
	Product Name: Example
	UUID: 12345678-1234-1234-1234-1234567890ab
	SKU Number: To be filled by O.E.M.
`
	got := parseUUIDFromDMIDecode(in)
	if got != "12345678-1234-1234-1234-1234567890ab" {
		t.Fatalf("expected uuid extracted, got=%q", got)
	}
}

func TestParseUUIDFromDMIDecode_EmptyWhenMissing(t *testing.T) {
	in := `System Information
	Product Name: Example
	Serial Number: ABC
`
	got := parseUUIDFromDMIDecode(in)
	if got != "" {
		t.Fatalf("expected empty, got=%q", got)
	}
}

func TestParseUUIDFromDMIDecode_TrimSpaces(t *testing.T) {
	in := "UUID:\t  ABCD-EF  \n"
	got := parseUUIDFromDMIDecode(in)
	if got != "ABCD-EF" {
		t.Fatalf("expected trimmed uuid, got=%q", got)
	}
}

func TestGetDeviceUUID_ConfigOverridesEverything(t *testing.T) {
	defer setupViperForLinuxTests()()

	viper.Set("UUID", "my-fixed-uuid")
	got := GetDeviceUUID()
	if got != "my-fixed-uuid" {
		t.Fatalf("expected config uuid, got=%q", got)
	}
}

func TestGetDeviceZone_Config(t *testing.T) {
	defer setupViperForLinuxTests()()

	viper.Set("IDC.Zone", "zone-a")
	got := GetDeviceZone()
	if got != "zone-a" {
		t.Fatalf("expected zone from config, got=%q", got)
	}
}

func TestGetDeviceHostname_Config(t *testing.T) {
	defer setupViperForLinuxTests()()

	viper.Set("HostName", "my-host")
	got := GetDeviceHostname()
	if got != "my-host" {
		t.Fatalf("expected hostname from config, got=%q", got)
	}
}

func TestGetConfigIP_Config(t *testing.T) {
	defer setupViperForLinuxTests()()

	viper.Set("IP", "192.0.2.10")
	got := GetConfigIP()
	if got != "192.0.2.10" {
		t.Fatalf("expected ip from config, got=%q", got)
	}
}

func TestGetConfigIP_NoConfig_NoPanic(t *testing.T) {
	defer setupViperForLinuxTests()()

	viper.Set("IP", "")
	// should not panic even if no usable interfaces
	got := GetConfigIP()
	_ = got // allow empty result depending on host env
}

func TestGetDeviceIPList_Smoke_NoPanic(t *testing.T) {
	// env-dependent, only assert it does not panic and returns sane values if any
	ips := GetDeviceIPList()
	for _, ip := range ips {
		if strings.TrimSpace(ip) == "" {
			t.Fatalf("expected non-empty ip string")
		}
	}
}

func TestGetDeviceOsInfo_Smoke_NotNil(t *testing.T) {
	// env-dependent, only ensure it does not return nil / panic
	got := GetDeviceOsInfo()
	if got == nil {
		t.Fatalf("expected non-nil osinfo")
	}
}
