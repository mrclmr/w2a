package audio

import (
	"testing"
)

func TestNewTextTmpl(t *testing.T) {
	tests := []struct {
		name    string
		str     string
		values  TextTmplValues
		want    string
		wantErr bool
	}{
		{
			name: "valid template",
			str:  "{{ .WorkoutExercisesCount }} {{ .WorkoutDuration }} {{ .WorkoutDurationWithoutPauses }} {{ .ExerciseDuration }} {{ .ExerciseName }}",
			values: TextTmplValues{
				WorkoutExercisesCount:        99,
				WorkoutDuration:              "2 minutes",
				WorkoutDurationWithoutPauses: "1 minute",
				ExerciseDuration:             "30 seconds",
				ExerciseName:                 "my exercise",
			},
			want:    "99 2 minutes 1 minute 30 seconds my exercise",
			wantErr: false,
		},
		{
			name:    "text without template values",
			str:     "some text",
			want:    "some text",
			wantErr: false,
		},
		{
			name:    "unknown template value",
			str:     "{{ .Unknown }}",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			textTmpl, err := NewTextTmpl(tt.str)
			if tt.wantErr && err != nil {
				return
			}
			if err != nil {
				t.Fatalf("NewTextTmpl() error = %v, wantErr %v", err, tt.wantErr)
			}
			got := textTmpl.Replace(tt.values)
			if got != tt.want {
				t.Fatalf("Replace() got = %v, want %v", got, tt.want)
			}
		})
	}
}
