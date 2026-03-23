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
			want:    "#(tmux-agents status -d \" • \")%m/%d %H:%M%z",
		},
		{
			name:    "trim surrounding whitespace",
			current: "   %H:%M   ",
			want:    "#(tmux-agents status -d \" • \")%H:%M",
		},
		{
			name:    "already configured on left",
			current: "#(tmux-agents status -d \" • \")%H:%M",
			want:    "#(tmux-agents status -d \" • \")%H:%M",
		},
		{
			name:    "migrate legacy interpolation to left",
			current: "%H:%M #(tmux-agents status)",
			want:    "#(tmux-agents status -d \" • \")%H:%M",
		},
		{
			name:    "drop duplicate interpolations",
			current: "#(tmux-agents status -d \" • \")%H:%M #(tmux-agents status)",
			want:    "#(tmux-agents status -d \" • \")%H:%M",
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
