package glob

import "testing"

func TestPath(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"src/**", "src/a/b.go", true},
		{"src/**", "srcx/a.go", false},
		{"**/*.tf", "main.tf", true},
		{"**/*.tf", "sites/a/main.tf", true},
		{"plans/PLAT-1-*.md", "plans/PLAT-1-login.md", true},
		{"plans/PLAT-1-*.md", "plans/sub/PLAT-1-login.md", false},
		{"*.md", "docs/guide.md", true}, // no slash: matches the base name anywhere
		{"docs/*.md", "docs/a/guide.md", false},
		{"polix.yaml", "polix.yaml", true},
		{"file?.txt", "file1.txt", true},
		{"a.b", "aXb", false}, // dots are literal
	}
	for _, c := range cases {
		if got := Path(c.pattern, c.path); got != c.want {
			t.Errorf("Path(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestCommand(t *testing.T) {
	cases := []struct {
		pattern, command string
		want             bool
	}{
		{"terraform apply*", "terraform apply -auto-approve", true},
		{"terraform apply*", "terraform plan", false},
		{"just test", "  just test ", true},
		{"just test", "just test -k sync", false},
		{"*git push*", "cd x && git push origin main", true},
	}
	for _, c := range cases {
		if got := Command(c.pattern, c.command); got != c.want {
			t.Errorf("Command(%q, %q) = %v, want %v", c.pattern, c.command, got, c.want)
		}
	}
}
