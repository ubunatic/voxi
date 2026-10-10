package spec

import (
	"embed"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed doctor.yaml scripts/routing-probe.sh
var doctorFiles embed.FS

// DoctorSpec contains the checks and embedded scripts for `voxi doctor`.
type DoctorSpec struct {
	Checks  []DoctorCheck     `yaml:"checks"`
	Scripts map[string]string `yaml:"scripts"`
}

// DoctorCheck describes one read-only audio health check.
type DoctorCheck struct {
	ID        string          `yaml:"id"`
	Title     string          `yaml:"title"`
	Order     int             `yaml:"order"`
	Services  []DoctorService `yaml:"services"`
	Defaults  []DoctorDefault `yaml:"defaults"`
	Bluetooth DoctorBluetooth `yaml:"bluetooth"`
	Journal   DoctorJournal   `yaml:"journal"`
	Coredumps DoctorCoredumps `yaml:"coredumps"`
	Routing   DoctorRouting   `yaml:"routing"`
	Repair    DoctorRepair    `yaml:"repair"`
}

// DoctorService identifies a systemd unit and manager scope.
type DoctorService struct {
	Scope string `yaml:"scope"`
	Unit  string `yaml:"unit"`
}

// DoctorDefault identifies a PipeWire node class checked as a default.
type DoctorDefault struct {
	Kind  string `yaml:"kind"`
	Class string `yaml:"class"`
}

// DoctorBluetooth defines which PipeWire cards are Bluetooth cards.
type DoctorBluetooth struct {
	API        string `yaml:"api"`
	OffProfile string `yaml:"off_profile"`
}

// DoctorJournal lists journal signatures and sleep markers.
type DoctorJournal struct {
	Signals       []string `yaml:"signals"`
	ResumeMarker  string   `yaml:"resume_marker"`
	SuspendMarker string   `yaml:"suspend_marker"`
}

// DoctorCoredumps configures the core dump window and executable names.
type DoctorCoredumps struct {
	Since     string   `yaml:"since"`
	Processes []string `yaml:"processes"`
}

// DoctorRouting configures the silent playback probe.
type DoctorRouting struct {
	ProbeNode  string `yaml:"probe_node"`
	Script     string `yaml:"script"`
	SampleRate int    `yaml:"sample_rate"`
	Channels   int    `yaml:"channels"`
	DurationMS int    `yaml:"duration_ms"`
}

// DoctorRepair stores the prompt shown beside a printed repair command.
type DoctorRepair struct {
	Prompt    string            `yaml:"prompt"`
	Templates map[string]string `yaml:"templates"`
}

// LoadDoctor parses and validates the embedded doctor specification.
func LoadDoctor() (*DoctorSpec, error) {
	data, err := doctorFiles.ReadFile("doctor.yaml")
	if err != nil {
		return nil, fmt.Errorf("spec: read doctor.yaml: %w", err)
	}
	var s DoctorSpec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("spec: parse doctor.yaml: %w", err)
	}
	if err := validateDoctorSpec(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// DoctorScript reads a script embedded in the doctor specification.
func DoctorScript(name string) ([]byte, error) {
	s, err := LoadDoctor()
	if err != nil {
		return nil, err
	}
	path, ok := s.Scripts[name]
	if !ok {
		return nil, fmt.Errorf("spec: doctor.yaml does not register script %q", name)
	}
	data, err := doctorFiles.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("spec: read doctor script %q: %w", name, err)
	}
	return data, nil
}

func validateDoctorSpec(s *DoctorSpec) error {
	if len(s.Checks) == 0 {
		return fmt.Errorf("spec: doctor.yaml declares no checks")
	}
	seen := make(map[string]bool, len(s.Checks))
	orders := make([]int, 0, len(s.Checks))
	for _, c := range s.Checks {
		if c.ID == "" || c.Title == "" || c.Order < 1 || c.Repair.Prompt == "" {
			return fmt.Errorf("spec: doctor check missing id, title, order, or repair prompt")
		}
		if seen[c.ID] {
			return fmt.Errorf("spec: duplicate doctor check %q", c.ID)
		}
		seen[c.ID] = true
		orders = append(orders, c.Order)
	}
	sort.Ints(orders)
	for i, order := range orders {
		if order != i+1 {
			return fmt.Errorf("spec: doctor check order must be contiguous from 1, got %v", orders)
		}
	}
	if _, ok := s.Scripts["routing-probe.sh"]; !ok {
		return fmt.Errorf("spec: doctor.yaml does not register routing-probe.sh")
	}
	return nil
}
