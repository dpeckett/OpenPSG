package openpsg

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TransducerType defines types of transducers
type TransducerType string

const (
	MEMSPressureTransducer TransducerType = "MEMS Pressure Transducer"
)

// Unit defines measurement units
type Unit string

const (
	Microvolts Unit = "uV"
	Millivolts Unit = "mV"
	Volts      Unit = "V"
	Hertz      Unit = "Hz"
	Kilohertz  Unit = "kHz"
	Pascal     Unit = "Pa"
)

// FilterKind defines types of filters
type FilterKind string

const (
	HighPass FilterKind = "HP"
	LowPass  FilterKind = "LP"
	Notch    FilterKind = "N"
)

// Filter represents a filter configuration
type Filter struct {
	Kind      FilterKind
	Unit      Unit
	Frequency float32
}

// FilterList holds a list of Filters
type FilterList struct {
	Filters []Filter
}

// UnmarshalJSON custom unmarshaller for FilterList
func (fl *FilterList) UnmarshalJSON(data []byte) error {
	var filtersStr string
	if err := json.Unmarshal(data, &filtersStr); err != nil {
		return err
	}

	parts := strings.Split(filtersStr, " ")
	for _, part := range parts {
		details := strings.Split(part, ":")
		if len(details) != 2 {
			return fmt.Errorf("invalid filter format")
		}

		r := regexp.MustCompile(`(\d+\.\d+|\d+)(Hz|kHz)`)
		matches := r.FindStringSubmatch(details[1])
		if matches == nil || len(matches) < 3 {
			return fmt.Errorf("invalid frequency format")
		}

		frequency, err := strconv.ParseFloat(matches[1], 32)
		if err != nil {
			return fmt.Errorf("error parsing frequency: %v", err)
		}

		var unit Unit
		switch matches[2] {
		case "Hz":
			unit = Hertz
		case "kHz":
			unit = Kilohertz
		default:
			return fmt.Errorf("unsupported unit: %s", matches[2])
		}

		fl.Filters = append(fl.Filters, Filter{
			Kind:      FilterKind(details[0]),
			Unit:      unit,
			Frequency: float32(frequency),
		})
	}

	return nil
}

// MarshalJSON custom marshaller for FilterList
func (fl *FilterList) MarshalJSON() ([]byte, error) {
	var builder strings.Builder
	for i, filter := range fl.Filters {
		if i > 0 {
			builder.WriteString(" ")
		}
		builder.WriteString(fmt.Sprintf("%s:%f%s", filter.Kind, filter.Frequency, filter.Unit))
	}
	return json.Marshal(builder.String())
}

// Signal represents a signal configuration
type Signal struct {
	// The unique identifier of the signal.
	ID uint32 `json:"id"`
	// The human-readable name of the signal.
	Name string `json:"name"`
	// The type of transducer used to measure the signal.
	TransducerType TransducerType `json:"transducerType"`
	// The unit of the signal (eg. microvolts).
	Unit Unit `json:"unit"`
	// The minimum value of the signal (in the unit of the signal).
	Min float32 `json:"min"`
	// The maximum value of the signal (in the unit of the signal).
	Max float32 `json:"max"`
	// The list of filters applied to the signal.
	Prefiltering FilterList `json:"prefiltering"`
	// The sample rate of the signal (in Hertz).
	SampleRate uint32 `json:"sampleRate"`
}

type SignalValues struct {
	// The unique identifier of the signal these values belong to.
	ID uint32
	// The start timestamp of the values.
	Timestamp time.Time
	// The list of values.
	Values []int16
}
