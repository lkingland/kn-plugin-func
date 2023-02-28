package config

/* TODO: refactor to use Local struct and be more gooder
func TestLocal_Validate(t *testing.T) {

	tests := []struct {
		name string
		cfg  Local
		errs int
	}{
		{
			"correct 'Git - only URL https",
			Local{
				GitURL: "https://myrepo/foo.git",
			},
			0,
		},
		{
			"correct 'Git - only URL scp",
			Local{
				URL: "git@myrepo:foo.git",
			},
			0,
		},
		{
			"correct 'Git - URL + revision",
			Local{
				URL:      "https://myrepo/foo.git",
				Revision: "mybranch",
			},
			0,
		},
		{
			"correct 'Git - URL + context-dir",
			Local{
				URL:        "https://myrepo/foo.git",
				ContextDir: "my-folder",
			},
			0,
		},
		{
			"correct 'Git - URL + revision & context-dir",
			Local{
				URL:        "https://myrepo/foo.git",
				Revision:   "mybranch",
				ContextDir: "my-folder",
			},
			0,
		},
		{
			"incorrect 'Git - bad URL",
			Local{
				URL: "foo",
			},
			1,
		},
		{
			"correct 'Git - not mandatory",
			Local{},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.Validate(tt.cfg); len(got) != tt.errs {
				t.Errorf("validateGit() = %v\n got %d errors but want %d", got, len(got), tt.errs)
			}
		})
	}
}
*/
