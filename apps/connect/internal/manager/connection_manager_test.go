package manager

import "testing"

func TestSendToDeviceReturnsFalseWhenDeviceMissing(t *testing.T) {
	m := NewConnectionManagerWithBuckets(1)
	m.Register(NewClient(nil, "user-1", "dev-1"))

	if m.SendToDevice("user-1", "missing-dev", []byte("payload"), DeliveryMetadata{}) {
		t.Fatalf("expected SendToDevice to return false for missing device")
	}
}

func TestUnregisterOnlyRemovesMatchingPointer(t *testing.T) {
	m := NewConnectionManagerWithBuckets(1)
	oldClient := NewClient(nil, "user-1", "dev-1")
	newClient := NewClient(nil, "user-1", "dev-1")

	m.Register(oldClient)
	m.Register(newClient)

	removed := m.Unregister(oldClient)
	if removed {
		t.Fatalf("expected old pointer unregister to be ignored after replacement")
	}
	devices := m.GetOnlineDevices("user-1")
	if len(devices) != 1 || devices[0] != "dev-1" {
		t.Fatalf("expected replacement connection to remain online, got %#v", devices)
	}

	removed = m.Unregister(newClient)
	if !removed {
		t.Fatalf("expected matching pointer unregister to remove active connection")
	}
	if devices = m.GetOnlineDevices("user-1"); len(devices) != 0 {
		t.Fatalf("expected no devices after unregister, got %#v", devices)
	}
}
