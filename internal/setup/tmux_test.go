package setup

import "testing"

func TestDesiredStatusRight(t *testing.T) {
	tests := []struct {
		name    string
		current string
		want    string
	}{
		{
			name:    "empty",
			current: "",
			want:    StatusInterpolation,
		},
		{
			name:    "prepend existing status",
			current: "%m/%d %H:%M%z",
			want:    "#(tmux-agents status)%m/%d %H:%M%z",
		},
		{
			name:    "trim surrounding whitespace",
			current: "   %H:%M   ",
			want:    "#(tmux-agents status)%H:%M",
		},
		{
			name:    "already configured on left",
			current: "#(tmux-agents status)%H:%M",
			want:    "#(tmux-agents status)%H:%M",
		},
		{
			name:    "migrate legacy interpolation",
			current: "%H:%M #(tmux-agents status -d \" • \")",
			want:    "#(tmux-agents status)%H:%M",
		},
		{
			name:    "drop duplicate interpolations",
			current: "#(tmux-agents status)%H:%M #(tmux-agents status -d \" • \")",
			want:    "#(tmux-agents status)%H:%M",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := desiredStatusRight(test.current); got != test.want {
				t.Fatalf("desiredStatusRight(%q) = %q, want %q", test.current, got, test.want)
			}
		})
	}
}
