package battery

import "testing"

const ioregSample = `+-o AppleBluetoothHIDKeyboard  <class AppleBluetoothHIDKeyboard>
    {
      "BatteryPercent" = 64
      "Product" = "Magic Mouse"
      "VendorID" = 76
    }
+-o AppleDeviceManagementHIDEventService  <class AppleDeviceManagementHIDEventService>
    {
      "BatteryStatusFlags" = 3
      "Product" = "Rainy75"
      "BatteryPercent" = 87
      "VendorID" = 12815
    }
`

func TestParseIORegistryPicksRainy(t *testing.T) {
	s, ok := parseIORegistry(ioregSample)
	if !ok || s.Percent != 87 || !s.Charging || !s.Available {
		t.Fatalf("ok=%v status=%+v", ok, s)
	}
}

func TestParseIORegistryByVendorID(t *testing.T) {
	out := `+-o X
      "BatteryPercent" = 40
      "Product" = "BT Keyboard"
      "VendorID" = 12815
`
	s, ok := parseIORegistry(out)
	if !ok || s.Percent != 40 || s.Charging {
		t.Fatalf("ok=%v status=%+v", ok, s)
	}
}

func TestParseIORegistryNoMatch(t *testing.T) {
	if _, ok := parseIORegistry(`+-o Y "BatteryPercent" = 10 "Product" = "Mouse"`); ok {
		t.Fatal("чужое устройство не должно совпадать")
	}
}
