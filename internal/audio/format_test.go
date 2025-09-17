package audio

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

type testStruct struct {
	Format Format `yaml:"format"`
}

func TestFormat_Unmarshal(t *testing.T) {
	tests := []struct {
		input   string
		want    Format
		wantErr bool
	}{
		{"format: m4a", M4a, false},
		{"format: mp3", Mp3, false},
		{"format: wav", Wav, false},
		{"format: flac", Unknown, true},
	}

	for _, tt := range tests {
		var ts testStruct
		err := yaml.Unmarshal([]byte(tt.input), &ts)
		if err != nil {
			if !tt.wantErr {
				t.Fatalf("Unmarshal error = %v, want error: %v", err, tt.wantErr)
			}
			if tt.want != Unknown {
				t.Fatalf("Unmarshal error = %v, want %v", err, tt.want)
			}
			return
		}
		if tt.wantErr {
			t.Fatalf("Unmarshal error = %v, want error: %v", err, tt.wantErr)
		}
		if ts.Format != tt.want {
			t.Fatalf("want %v, Format %v", tt.want, ts.Format)
		}
	}
}
