package process

import (
	"math"
	"testing"
)

func TestTransportGetBarsBeats(t *testing.T) {
	tests := []struct {
		name      string
		transport TransportInfo
		wantBars  int
		wantBeats float64
	}{
		{
			name:      "no musical time",
			transport: TransportInfo{HasTimeSignature: true, TimeSigNumerator: 4, TimeSigDenominator: 4},
		},
		{
			name:      "no time signature",
			transport: TransportInfo{HasMusicalTime: true, ProjectTimeMusic: 5},
		},
		{
			name: "zero denominator is rejected",
			transport: TransportInfo{
				HasMusicalTime: true, HasTimeSignature: true,
				TimeSigNumerator: 4, TimeSigDenominator: 0, ProjectTimeMusic: 5,
			},
		},
		{
			name: "4/4 bar two beat one",
			transport: TransportInfo{
				HasMusicalTime: true, HasTimeSignature: true,
				TimeSigNumerator: 4, TimeSigDenominator: 4, ProjectTimeMusic: 5,
			},
			wantBars:  1,
			wantBeats: 1,
		},
		{
			name: "6/8 counts eighth notes",
			transport: TransportInfo{
				HasMusicalTime: true, HasTimeSignature: true,
				TimeSigNumerator: 6, TimeSigDenominator: 8, ProjectTimeMusic: 3,
			},
			wantBars:  1,
			wantBeats: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bars, beats := tt.transport.GetBarsBeats()
			if bars != tt.wantBars || math.Abs(beats-tt.wantBeats) > 1e-9 {
				t.Errorf("GetBarsBeats() = (%d, %v), want (%d, %v)", bars, beats, tt.wantBars, tt.wantBeats)
			}
		})
	}
}

func TestTransportGetSamplesPerBeat(t *testing.T) {
	tests := []struct {
		name       string
		transport  TransportInfo
		sampleRate float64
		want       float64
	}{
		{name: "no tempo", transport: TransportInfo{}, sampleRate: 48000},
		{name: "invalid tempo", transport: TransportInfo{HasTempo: true, Tempo: 0}, sampleRate: 48000},
		{name: "120 bpm at 48k", transport: TransportInfo{HasTempo: true, Tempo: 120}, sampleRate: 48000, want: 24000},
		{name: "60 bpm at 44.1k", transport: TransportInfo{HasTempo: true, Tempo: 60}, sampleRate: 44100, want: 44100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.transport.GetSamplesPerBeat(tt.sampleRate); math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("GetSamplesPerBeat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransportBeatPositionAndOnBeat(t *testing.T) {
	tests := []struct {
		name        string
		transport   TransportInfo
		wantBeatPos float64
		wantOnBeat  bool
	}{
		{
			name:       "missing musical time",
			transport:  TransportInfo{HasBarPosition: true},
			wantOnBeat: true, // position 0 counts as on-beat
		},
		{
			name: "quarter note default",
			transport: TransportInfo{
				HasMusicalTime: true, HasBarPosition: true,
				ProjectTimeMusic: 2.5, BarPositionMusic: 0,
			},
			wantBeatPos: 0.5,
		},
		{
			name: "eighth note grid",
			transport: TransportInfo{
				HasMusicalTime: true, HasBarPosition: true, HasTimeSignature: true,
				TimeSigNumerator: 6, TimeSigDenominator: 8,
				ProjectTimeMusic: 1.25, BarPositionMusic: 0,
			},
			wantBeatPos: 0.5,
		},
		{
			name: "just past the beat",
			transport: TransportInfo{
				HasMusicalTime: true, HasBarPosition: true,
				ProjectTimeMusic: 2.001, BarPositionMusic: 0,
			},
			wantBeatPos: 0.001,
			wantOnBeat:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.transport.GetBeatPosition(); math.Abs(got-tt.wantBeatPos) > 1e-9 {
				t.Errorf("GetBeatPosition() = %v, want %v", got, tt.wantBeatPos)
			}
			if got := tt.transport.IsOnBeat(0.01); got != tt.wantOnBeat {
				t.Errorf("IsOnBeat(0.01) = %v, want %v", got, tt.wantOnBeat)
			}
		})
	}
}
