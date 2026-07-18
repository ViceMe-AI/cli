package buildinfo

import "testing"

func TestValidateNPMLaunchBindsPackageAndBinaryVersions(t *testing.T) {
	t.Parallel()
	if err := ValidateNPMLaunch("npm", "0.1.0", "0.1.0"); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method         string
		packageVersion string
		binaryVersion  string
		wantError      bool
	}{
		{"npm", "", "0.1.0", true},
		{"npm", "0.1.0", "dev", true},
		{"npm", "0.1.1", "0.1.0", true},
		{"development", "0.1.1", "dev", false},
	} {
		err := ValidateNPMLaunch(test.method, test.packageVersion, test.binaryVersion)
		if (err != nil) != test.wantError {
			t.Errorf("ValidateNPMLaunch(%q, %q, %q) error=%v, wantError=%t", test.method, test.packageVersion, test.binaryVersion, err, test.wantError)
		}
	}
}
