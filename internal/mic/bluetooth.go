package mic

import (
	"sync"

	"ubunatic.com/voxi/spec"
)

// bluetoothSpec is the spec's bluetooth_mic block, parsed once. Detection, the
// headset profile choice and the doctor read the Bluetooth identity and the HFP
// profile name through it, so the spec is the only definition (issue 213).
var bluetoothSpec = sync.OnceValue(func() spec.BluetoothMicSpec {
	es, err := spec.LoadEager()
	if err != nil {
		panic("mic: " + err.Error())
	}
	return es.BluetoothMic
})

// IsBluetoothDevice reports whether a card's device.api names a Bluetooth device,
// as the spec's bluetooth_mic.device_api does.
func IsBluetoothDevice(deviceAPI string) bool {
	return bluetoothSpec().IsBluetoothDevice(deviceAPI)
}
