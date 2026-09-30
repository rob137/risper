package meeting

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
)

const wavHeaderBytes = int64(44)

func availablePCM(paths map[string]string) (int64, error) {
	var sizes []int64
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, err
		}
		if info.Size() > wavHeaderBytes {
			sizes = append(sizes, (info.Size()-wavHeaderBytes)&^1)
		}
	}
	if len(sizes) == 0 {
		return 0, errors.New("meeting recorder has not produced PCM audio yet")
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] < sizes[j] })
	return sizes[0], nil
}

func readPCM(path string, start, end int64) ([]byte, error) {
	handle, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	if _, err := handle.Seek(wavHeaderBytes+start, 0); err != nil {
		return nil, err
	}
	length := end - start
	if length < 0 {
		return nil, errors.New("negative PCM range")
	}
	data := make([]byte, length)
	n, err := handle.Read(data)
	if err != nil && n == 0 {
		return nil, err
	}
	return data[:n&^1], nil
}

func mixedPCM(paths map[string]string, start, end int64) ([]byte, error) {
	var sources [][]byte
	for _, path := range paths {
		data, err := readPCM(path, start, end)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if len(data) > 0 {
			sources = append(sources, data)
		}
	}
	if len(sources) == 0 {
		return nil, errors.New("meeting slice has no audio sources")
	}
	length := len(sources[0])
	for _, source := range sources[1:] {
		if len(source) < length {
			length = len(source)
		}
	}
	output := make([]byte, length)
	for offset := 0; offset+1 < length; offset += 2 {
		total := 0
		for _, source := range sources {
			total += int(int16(binary.LittleEndian.Uint16(source[offset : offset+2])))
		}
		value := int16(total / len(sources))
		binary.LittleEndian.PutUint16(output[offset:offset+2], uint16(value))
	}
	return output, nil
}

func quietestBoundary(paths map[string]string, desired, available int64) (int64, error) {
	searchRadius := int64(10) * pcmBytesSecond
	start := desired - searchRadius
	if start < 0 {
		start = 0
	}
	end := desired + searchRadius
	if end > available {
		end = available
	}
	pcm, err := mixedPCM(paths, start, end)
	if err != nil {
		return 0, err
	}
	windowSamples := 16000 * 300 / 1000
	samples := len(pcm) / 2
	if samples <= windowSamples {
		return desired, nil
	}
	squares := make([]float64, samples+1)
	for i := 0; i < samples; i++ {
		value := float64(int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2])))
		squares[i+1] = squares[i] + value*value
	}
	best := 0
	bestEnergy := math.Inf(1)
	step := 160 // 10 ms
	for i := 0; i+windowSamples <= samples; i += step {
		energy := squares[i+windowSamples] - squares[i]
		if energy < bestEnergy {
			bestEnergy = energy
			best = i + windowSamples/2
		}
	}
	boundary := start + int64(best*2)
	if boundary <= 0 || boundary > available {
		return desired, nil
	}
	return boundary &^ 1, nil
}

func dbFS(pcm []byte) float64 {
	if len(pcm) < 2 {
		return math.Inf(-1)
	}
	sum := float64(0)
	samples := 0
	for offset := 0; offset+1 < len(pcm); offset += 2 {
		value := float64(int16(binary.LittleEndian.Uint16(pcm[offset : offset+2])))
		sum += value * value
		samples++
	}
	if sum == 0 {
		return math.Inf(-1)
	}
	rms := math.Sqrt(sum / float64(samples))
	return 20 * math.Log10(rms/32768)
}

func writeWAV(path string, pcm []byte) error {
	handle, err := os.Create(path)
	if err != nil {
		return err
	}
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+len(pcm)))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], 16000)
	binary.LittleEndian.PutUint32(header[28:32], uint32(pcmBytesSecond))
	binary.LittleEndian.PutUint16(header[32:34], 2)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(len(pcm)))
	if _, err := handle.Write(header); err != nil {
		handle.Close()
		return err
	}
	if _, err := handle.Write(pcm); err != nil {
		handle.Close()
		return err
	}
	if err := handle.Close(); err != nil {
		return fmt.Errorf("close WAV: %w", err)
	}
	return nil
}
