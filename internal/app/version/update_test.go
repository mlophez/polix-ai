package version

import "testing"

func TestUpdateValid(t *testing.T) {
	u := Update{Running: "0.1.0", Exe: "/bin/polix"}
	if err := u.Valid(); err != nil {
		t.Fatalf("a valid update: %v", err)
	}
	for name, bad := range map[string]Update{
		"no version": {Exe: "/bin/polix"},
		"no binary":  {Running: "0.1.0"},
	} {
		if bad.Valid() == nil {
			t.Errorf("%s is valid", name)
		}
	}
}

func TestUpdateIntent(t *testing.T) {
	cases := []struct {
		name                              string
		u                                 Update
		latest, silent, records, installs bool
	}{
		{name: "person", u: Update{}, latest: true, installs: true},
		{name: "version asked", u: Update{Want: "0.2.0"}, installs: true},
		{name: "check", u: Update{Check: true}, latest: true},
		{name: "hook", u: Update{Hook: true}, latest: true, records: true, installs: true},
		{name: "disabled", u: Update{Disabled: true}, latest: true},
		{name: "disabled hook", u: Update{Hook: true, Disabled: true}, latest: true, silent: true},
	}
	for _, c := range cases {
		if c.u.Latest() != c.latest || c.u.Silent() != c.silent || c.u.Records() != c.records ||
			c.u.Installs() != c.installs {
			t.Errorf("%s: latest %v, silent %v, records %v, installs %v", c.name, c.u.Latest(), c.u.Silent(),
				c.u.Records(), c.u.Installs())
		}
	}
}
