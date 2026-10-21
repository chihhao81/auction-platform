package database

import "testing"

func TestDirectURL(t *testing.T) {
	cases := map[string]string{
		"postgresql://u:p@ep-cool-1234-pooler.ap-southeast-1.aws.neon.tech/db?sslmode=require": "postgresql://u:p@ep-cool-1234.ap-southeast-1.aws.neon.tech/db?sslmode=require",
		"postgres://u:p@ep-x-pooler.neon.tech:5432/db":                                         "postgres://u:p@ep-x.neon.tech:5432/db",
		"postgres://u:p@ep-cool-1234.ap-southeast-1.aws.neon.tech/db":                          "postgres://u:p@ep-cool-1234.ap-southeast-1.aws.neon.tech/db",
		"postgres://localhost/db":                                                              "postgres://localhost/db",
	}
	for in, want := range cases {
		if got := DirectURL(in); got != want {
			t.Errorf("DirectURL(%q) = %q, want %q", in, got, want)
		}
	}
}
