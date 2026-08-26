package httplog

import "testing"

func TestSanitizeQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rawQuery string
		want     string
	}{
		{name: "empty", rawQuery: "", want: ""},
		{name: "ordinary", rawQuery: "device_id=A1&page=2", want: "device_id=A1&page=2"},
		{name: "token", rawQuery: "token=secret-token&device_id=A1", want: "device_id=A1&token=REDACTED"},
		{name: "credential variants", rawQuery: "access_token=a&refresh-token=b&verify_code=123456", want: "access_token=REDACTED&refresh-token=REDACTED&verify_code=REDACTED"},
		{name: "invalid", rawQuery: "token=%zz", want: "INVALID_QUERY"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := SanitizeQuery(test.rawQuery); got != test.want {
				t.Fatalf("SanitizeQuery(%q) = %q, want %q", test.rawQuery, got, test.want)
			}
		})
	}
}
