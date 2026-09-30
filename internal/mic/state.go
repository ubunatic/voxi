// Package mic detects and repairs a broken PipeWire microphone setup (issue 177).
//
// The work is split into pure steps so each can be tested against captured data:
//
//	pw-dump JSON --ParseDump--> State --Diagnose--> []Problem --Plan--> []Action --Apply--> wpctl/pw-metadata
//
// Doctor ties the steps together, re-probes after repairing and reports what
// changed. It never restarts services or wipes WirePlumber's saved state: it only
// clears the saved default source, switches Bluetooth headphones to their headset
// profile and, as a last resort, makes a built-in mic the default.
package mic

import (
	"encoding/json"
	"fmt"
	"strings"
)

// State is the part of the PipeWire graph that decides which mic gets recorded.
type State struct {
	// DefaultSource is the node name PipeWire records from when no target is given.
	DefaultSource string
	// ConfiguredSource is the user's saved choice; it may name an absent device.
	ConfiguredSource string
	Sources          []Source
	Devices          []Device
}

// Source is an Audio/Source node (a mic, or a Bluetooth headset input).
type Source struct {
	ID          int
	Name        string
	Description string
	DeviceID    int
	// ProfileDevice is the node's card.profile.device, matched against Route.Device.
	ProfileDevice int
	Bluetooth     bool
	// Loopback marks WirePlumber's always-present Bluetooth input: recording
	// from it switches the card to headset mode by itself, and back to A2DP
	// 2s after the last stream closes (bluetooth.autoswitch-to-headset-profile).
	Loopback bool
}

// Device is an audio card; Bluetooth devices carry the A2DP/HFP profile choice.
type Device struct {
	ID          int
	Name        string
	Description string
	Bluetooth   bool
	// FormFactor is PipeWire's device.form-factor, e.g. "headset", "headphone", "speaker".
	FormFactor string
	Active     Profile
	Profiles   []Profile
	// Routes are the active ports; a source whose profile device has no
	// available input route (e.g. an unplugged jack mic) is not usable.
	Routes []Route
}

// Route is one active card port.
type Route struct {
	Device    int
	Input     bool
	Available bool
	Priority  int
}

// Usable reports whether the source has an available input port. Sources on
// cards without route information (e.g. Bluetooth) count as usable.
func (s State) Usable(src Source) bool {
	dev, ok := s.Device(src.DeviceID)
	if !ok || len(dev.Routes) == 0 {
		return true
	}
	_, ok = dev.inputRoute(src.ProfileDevice)
	return ok
}

func (d Device) inputRoute(profileDevice int) (Route, bool) {
	for _, r := range d.Routes {
		if r.Input && r.Available && r.Device == profileDevice {
			return r, true
		}
	}
	return Route{}, false
}

// Profile is one selectable card profile.
type Profile struct {
	Index       int
	Name        string
	Description string
	Available   bool
	// Sources counts the Audio/Source nodes the profile provides; 0 means no mic.
	Sources int
}

// Source looks up a source by node name.
func (s State) Source(name string) (Source, bool) {
	for _, src := range s.Sources {
		if src.Name == name {
			return src, true
		}
	}
	return Source{}, false
}

// Device looks up a device by object id.
func (s State) Device(id int) (Device, bool) {
	for _, dev := range s.Devices {
		if dev.ID == id {
			return dev, true
		}
	}
	return Device{}, false
}

// IsHeadphone reports whether the device is worn on the head and so may be
// switched to headset mode. Speakers are never switched, even if they offer HFP.
func (d Device) IsHeadphone() bool {
	switch d.FormFactor {
	case "headset", "headphone", "hands-free", "handset":
		return true
	}
	return false
}

// HeadsetProfile returns the best available profile that provides a mic.
// The plain "headset-head-unit" (mSBC/LC3 when supported) beats the CVSD variant.
func (d Device) HeadsetProfile() (Profile, bool) {
	var best Profile
	found := false
	for _, p := range d.Profiles {
		if !p.Available || p.Sources == 0 {
			continue
		}
		if !found || (p.Name == "headset-head-unit" && best.Name != "headset-head-unit") {
			best, found = p, true
		}
	}
	return best, found
}

type dumpObject struct {
	ID       int             `json:"id"`
	Type     string          `json:"type"`
	Info     *dumpInfo       `json:"info"`
	Props    map[string]any  `json:"props"`
	Metadata []dumpMetaEntry `json:"metadata"`
}

type dumpInfo struct {
	Props  map[string]any             `json:"props"`
	Params map[string]json.RawMessage `json:"params"`
}

type dumpMetaEntry struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type dumpRoute struct {
	Direction string `json:"direction"`
	Available string `json:"available"`
	Device    int    `json:"device"`
	Priority  int    `json:"priority"`
}

type dumpProfile struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Available   string `json:"available"`
	// Classes is [count, ["Audio/Source", n, ...], ...] for ALSA and
	// [["Audio/Source", n, ...], ...] for Bluetooth; see countSources.
	Classes json.RawMessage `json:"classes"`
}

// ParseDump builds a State from `pw-dump` output.
func ParseDump(data []byte) (State, error) {
	var objs []dumpObject
	if err := json.Unmarshal(data, &objs); err != nil {
		return State{}, fmt.Errorf("mic: parse pw-dump: %w", err)
	}
	var st State
	for _, o := range objs {
		switch {
		case strings.HasSuffix(o.Type, ":Metadata"):
			if str(o.Props, "metadata.name") != "default" {
				continue
			}
			for _, e := range o.Metadata {
				switch e.Key {
				case "default.audio.source":
					st.DefaultSource = metaName(e.Value)
				case "default.configured.audio.source":
					st.ConfiguredSource = metaName(e.Value)
				}
			}
		case strings.HasSuffix(o.Type, ":Node") && o.Info != nil:
			p := o.Info.Props
			if str(p, "media.class") != "Audio/Source" {
				continue
			}
			st.Sources = append(st.Sources, Source{
				ID:            o.ID,
				Name:          str(p, "node.name"),
				Description:   str(p, "node.description"),
				DeviceID:      num(p, "device.id"),
				ProfileDevice: num(p, "card.profile.device"),
				Bluetooth:     str(p, "device.api") == "bluez5" || strings.HasPrefix(str(p, "node.name"), "bluez_"),
				Loopback:      p["bluez5.loopback"] == true,
			})
		case strings.HasSuffix(o.Type, ":Device") && o.Info != nil:
			p := o.Info.Props
			if str(p, "media.class") != "Audio/Device" {
				continue
			}
			dev := Device{
				ID:          o.ID,
				Name:        str(p, "device.name"),
				Description: str(p, "device.description"),
				Bluetooth:   str(p, "device.api") == "bluez5",
				FormFactor:  str(p, "device.form-factor"),
				Profiles:    parseProfiles(o.Info.Params["EnumProfile"]),
				Routes:      parseRoutes(o.Info.Params["Route"]),
			}
			if active := parseProfiles(o.Info.Params["Profile"]); len(active) > 0 {
				dev.Active = active[0]
				for _, p := range dev.Profiles {
					if p.Index == dev.Active.Index {
						dev.Active = p
					}
				}
			}
			st.Devices = append(st.Devices, dev)
		}
	}
	return st, nil
}

func parseRoutes(raw json.RawMessage) []Route {
	var list []dumpRoute
	if len(raw) == 0 || json.Unmarshal(raw, &list) != nil {
		return nil
	}
	out := make([]Route, 0, len(list))
	for _, r := range list {
		out = append(out, Route{Device: r.Device, Input: r.Direction == "Input", Available: r.Available != "no", Priority: r.Priority})
	}
	return out
}

func parseProfiles(raw json.RawMessage) []Profile {
	var list []dumpProfile
	if len(raw) == 0 || json.Unmarshal(raw, &list) != nil {
		return nil
	}
	out := make([]Profile, 0, len(list))
	for _, p := range list {
		out = append(out, Profile{
			Index:       p.Index,
			Name:        p.Name,
			Description: p.Description,
			Available:   p.Available != "no",
			Sources:     countSources(p.Classes),
		})
	}
	return out
}

// countSources sums the Audio/Source counts in a profile's "classes" list,
// skipping the leading integer ALSA profiles carry.
func countSources(raw json.RawMessage) int {
	var items []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &items) != nil {
		return 0
	}
	total := 0
	for _, item := range items {
		var class []any
		if json.Unmarshal(item, &class) != nil || len(class) < 2 {
			continue
		}
		if name, _ := class[0].(string); name == "Audio/Source" {
			if n, ok := class[1].(float64); ok {
				total += int(n)
			}
		}
	}
	return total
}

func metaName(raw json.RawMessage) string {
	var v struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return v.Name
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func num(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case string:
		var n int
		_, _ = fmt.Sscan(v, &n)
		return n
	}
	return 0
}
