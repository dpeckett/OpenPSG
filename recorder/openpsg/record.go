package openpsg

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/netip"
	"time"

	"github.com/OpenPSG/edf"
	"github.com/hedzr/go-ringbuf/v2"
	"github.com/hedzr/go-ringbuf/v2/mpmc"
	"golang.org/x/sync/errgroup"
)

// 30 second epochs are pretty standard for PSG data.
const dataRecordDuration = 30 * time.Second

// Record records PSG data from the specified devices and writes it to an EDF file.
func Record(ctx context.Context, edfFile io.WriteSeeker, patientID, recordingID string, deviceAddrs []netip.Addr) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	currentSignalIndice := 0
	signalIndices := make(map[netip.Addr]map[uint32]int)
	var signals []Signal
	var signalBuffers []mpmc.RingBuffer[float64]

	for _, deviceAddr := range deviceAddrs {
		client, err := Connect(ctx, netip.AddrPortFrom(deviceAddr, 80))
		if err != nil {
			slog.Warn("Failed to connect to device", slog.Any("error", err))
			continue
		}

		deviceSignals, err := client.Signals(ctx)
		if err != nil {
			return fmt.Errorf("failed to get signals: %w", err)
		}

		signalIndices[deviceAddr] = make(map[uint32]int)
		for _, signal := range deviceSignals {
			signalIndices[deviceAddr][signal.ID] = currentSignalIndice
			signalBuffers = append(signalBuffers, ringbuf.New[float64](2*uint32(float64(signal.SampleRate)*dataRecordDuration.Seconds())))
			currentSignalIndice++

			signals = append(signals, signal)
		}

		g.Go(func() error {
			defer client.Close()

			deviceSignalIDs := make([]uint32, len(signals))
			for i, signal := range signals {
				deviceSignalIDs[i] = signal.ID
			}

			slog.Debug("Starting recording",
				slog.Any("deviceAddr", deviceAddr),
				slog.Any("signals", deviceSignalIDs))

			if err := client.Start(ctx, deviceSignalIDs); err != nil {
				return fmt.Errorf("failed to start recording: %w", err)
			}

			deviceSignalValues := client.SignalValues()

			for {
				select {
				case <-ctx.Done():
					slog.Debug("Stopping recording", slog.Any("deviceAddr", deviceAddr))

					if err := client.Stop(context.Background(), deviceSignalIDs); err != nil {
						return fmt.Errorf("failed to stop recording: %w", err)
					}

					return nil
				case sv := <-deviceSignalValues:
					// Rewrite the signal id to it's global form.
					sv.ID = uint32(signalIndices[deviceAddr][sv.ID])

					// TODO: handle missing, and out-of-order signal values.
					// Given we are using a reliable transport (TCP), we should be okay.

					for _, value := range sv.Values {
						if err := signalBuffers[sv.ID].Enqueue(convertDigitalToPhysical(
							value, float64(signals[sv.ID].Min), float64(signals[sv.ID].Max))); err != nil {
							return fmt.Errorf("signal buffer overrun: %w", err)
						}
					}
				}
			}
		})
	}

	g.Go(func() error {
		hdr := edf.Header{
			Version:            edf.Version0,
			PatientID:          patientID,
			RecordingID:        recordingID,
			StartTime:          time.Now(),
			DataRecordDuration: dataRecordDuration,
			SignalCount:        len(signals),
		}

		for _, signal := range signals {
			hdr.Signals = append(hdr.Signals, edf.SignalHeader{
				Label:             signal.Name,
				TransducerType:    string(signal.TransducerType),
				PhysicalDimension: string(signal.Unit),
				PhysicalMin:       float64(signal.Min),
				PhysicalMax:       float64(signal.Max),
				DigitalMin:        math.MinInt16,
				DigitalMax:        math.MaxInt16,
				SamplesPerRecord:  int(float64(signal.SampleRate) * hdr.DataRecordDuration.Seconds()),
			})
		}

		slog.Info("Writing EDF file header")

		ew, err := edf.Create(edfFile, hdr)
		if err != nil {
			return fmt.Errorf("failed to create EDF writer: %w", err)
		}
		defer ew.Close()

		// Give some time for the signal values to start coming in.
		select {
		case <-time.After(hdr.DataRecordDuration / 2):
		case <-ctx.Done():
			return nil
		}

		ticker := time.NewTicker(hdr.DataRecordDuration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}

			// Prepare a record to write to the EDF file.
			record := make([][]float64, len(signals))
			for i := range record {
				record[i] = make([]float64, hdr.Signals[i].SamplesPerRecord)
			}

			for i, buf := range signalBuffers {
				for j := 0; j < int(hdr.Signals[i].SamplesPerRecord); j++ {
					value, err := buf.Dequeue()
					if err != nil {
						slog.Warn("Missing signal values", slog.Any("error", err))
						break
					}

					record[i][j] = value
				}
			}

			slog.Info("Writing record to EDF file",
				slog.Int("signals", len(record)),
				slog.Duration("duration", hdr.DataRecordDuration))

			// Attempt to write the record to the EDF file.
			if err := ew.WriteRecord(record); err != nil {
				return fmt.Errorf("failed to write record: %w", err)
			}
		}
	})

	return g.Wait()
}

func convertDigitalToPhysical(digital int16, pmin, pmax float64) float64 {
	return pmin + (float64(digital)-float64(math.MinInt16))*(pmax-pmin)/float64(math.MaxInt16-math.MinInt16)
}
