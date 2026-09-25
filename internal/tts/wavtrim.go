package tts

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"time"
)

// trimTrailingSilence removes up to maxTrim of quiet PCM16 audio from a WAV.
// Unsupported WAV encodings are left intact.
func trimTrailingSilence(path string, maxTrim time.Duration, thresholdDB float64) error {
	if maxTrim <= 0 {
		return nil
	}
	wav, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read synthesized WAV: %w", err)
	}
	if len(wav) < 12 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return nil
	}

	var dataOffset, dataSize int
	var channels, sampleRate, bitsPerSample uint32
	for offset := 12; offset+8 <= len(wav); {
		chunkSize := uint64(binary.LittleEndian.Uint32(wav[offset+4 : offset+8]))
		chunkStart := offset + 8
		chunkEnd64 := uint64(chunkStart) + chunkSize
		if chunkEnd64 > uint64(len(wav)) {
			return nil
		}
		chunkEnd := int(chunkEnd64)
		switch string(wav[offset : offset+4]) {
		case "fmt ":
			if chunkSize < 16 {
				return nil
			}
			format := binary.LittleEndian.Uint16(wav[chunkStart : chunkStart+2])
			if format != 1 {
				return nil
			}
			channels = uint32(binary.LittleEndian.Uint16(wav[chunkStart+2 : chunkStart+4]))
			sampleRate = binary.LittleEndian.Uint32(wav[chunkStart+4 : chunkStart+8])
			bitsPerSample = uint32(binary.LittleEndian.Uint16(wav[chunkStart+14 : chunkStart+16]))
		case "data":
			dataOffset = chunkStart
			dataSize = int(chunkSize)
		}
		offset = chunkEnd + int(chunkSize&1)
	}
	if dataOffset == 0 || channels == 0 || sampleRate == 0 || bitsPerSample != 16 {
		return nil
	}
	frameBytes := int(channels) * 2
	frameCount := dataSize / frameBytes
	maxFrames := int(maxTrim * time.Duration(sampleRate) / time.Second)
	if maxFrames > frameCount {
		maxFrames = frameCount
	}
	threshold := int32(math.Round(math.Pow(10, thresholdDB/20) * math.MaxInt16))
	trimFrames := 0
	for trimFrames < maxFrames {
		frameStart := dataOffset + dataSize - (trimFrames+1)*frameBytes
		quiet := true
		for channel := uint32(0); channel < channels; channel++ {
			sampleOffset := frameStart + int(channel)*2
			sample := int32(int16(binary.LittleEndian.Uint16(wav[sampleOffset : sampleOffset+2])))
			if sample < 0 {
				sample = -sample
			}
			if sample > threshold {
				quiet = false
				break
			}
		}
		if !quiet {
			break
		}
		trimFrames++
	}
	if trimFrames == 0 {
		return nil
	}

	trimBytes := trimFrames * frameBytes
	newDataSize := dataSize - trimBytes
	newData := make([]byte, 0, len(wav)-trimBytes)
	newData = append(newData, wav[:dataOffset+newDataSize]...)
	newData = append(newData, wav[dataOffset+dataSize:]...)
	binary.LittleEndian.PutUint32(newData[dataOffset-4:dataOffset], uint32(newDataSize))
	binary.LittleEndian.PutUint32(newData[4:8], uint32(len(newData)-8))
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat synthesized WAV: %w", err)
	}
	if err := os.WriteFile(path, newData, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write trimmed WAV: %w", err)
	}
	return nil
}
